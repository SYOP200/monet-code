// Package update upgrades a prebuilt Monet Code install in place.
//
// It is the Go counterpart of `monet update`: it finds the install tree from
// MONET_HOME or the running binary, compares the marker's release tag with the
// latest GitHub release, downloads the matching assets, verifies the published
// SHA-256 sums, and atomically replaces the TUI binary and bundled CLI. It
// keeps configuration and sessions untouched.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRepo is the GitHub repository that publishes releases.
	DefaultRepo = "SYOP200/monet-code"
	// BinaryName is the TUI executable inside <root>/bin.
	BinaryName = "monet-tui"
	// MarkerName identifies an install tree created by install.sh.
	MarkerName = ".monet-code-install"
	// CLIAsset is the archive that holds the bundled Python CLI.
	CLIAsset = "monet-cli.tar.gz"
)

// Options controls one update run. Zero values are filled from the install
// tree, the running platform and the public release URL.
type Options struct {
	Root           string
	Repo           string
	Tag            string
	Force          bool
	Check          bool
	CurrentVersion string
	ReleaseBase    string
	GOOS           string
	GOARCH         string
	Client         *http.Client
	Log            func(string)
}

// Result reports what an update run found or installed.
type Result struct {
	Current string
	Latest  string
	Updated bool
}

// NormalizeTag returns a tag with exactly one leading "v".
func NormalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	return "v" + strings.TrimPrefix(tag, "v")
}

// VersionTuple extracts up to three numeric components from a tag.
func VersionTuple(tag string) [3]int {
	base := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if i := strings.IndexAny(base, "-+"); i >= 0 {
		base = base[:i]
	}
	var out [3]int
	part := 0
	for _, field := range strings.Split(base, ".") {
		if part >= len(out) {
			break
		}
		n, err := strconv.Atoi(strings.TrimFunc(field, func(r rune) bool {
			return r < '0' || r > '9'
		}))
		if err == nil {
			out[part] = n
		}
		part++
	}
	return out
}

// IsNewer reports whether latest is a higher release than current.
func IsNewer(latest, current string) bool {
	a, b := VersionTuple(latest), VersionTuple(current)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// AssetNames returns the TUI asset for the platform and a human label.
func AssetNames(goos, goarch string) (string, string, error) {
	switch strings.ToLower(goos) {
	case "darwin", "linux":
	default:
		return "", "", fmt.Errorf("self-update is not supported on %s", goos)
	}
	switch strings.ToLower(goarch) {
	case "amd64", "x86_64":
		goarch = "amd64"
	case "arm64", "aarch64":
		goarch = "arm64"
	default:
		return "", "", fmt.Errorf("no prebuilt binary is published for %s", goarch)
	}
	return fmt.Sprintf("%s-%s-%s", BinaryName, strings.ToLower(goos), goarch),
		strings.ToLower(goos) + " " + goarch, nil
}

// ParseMarker reads the simple key=value install marker.
func ParseMarker(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return out
}

// ParseChecksums reads `sha256  name` lines, including the binary-mode `*`.
func ParseChecksums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		out[name] = strings.ToLower(fields[0])
	}
	return out
}

// InstallRoot finds the install tree that owns this binary.
func InstallRoot() (string, error) {
	if env := os.Getenv("MONET_HOME"); env != "" {
		return filepath.Abs(env)
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		if filepath.Base(dir) == "bin" {
			root := filepath.Dir(dir)
			if _, err := os.Stat(filepath.Join(root, MarkerName)); err == nil {
				return root, nil
			}
		}
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		candidate := filepath.Join(xdg, "monet-code")
		if _, err := os.Stat(filepath.Join(candidate, MarkerName)); err == nil {
			return candidate, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".local", "share", "monet-code")
		if _, err := os.Stat(filepath.Join(candidate, MarkerName)); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("could not find a Monet Code install; run the installer first")
}

// Run checks for or installs a release. It returns an error for anything the
// user can act on: a source install, an unsupported platform, a checksum
// mismatch or a network failure.
func Run(ctx context.Context, opts Options) (Result, error) {
	root := opts.Root
	if root == "" {
		var err error
		root, err = InstallRoot()
		if err != nil {
			return Result{}, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}

	marker, err := readMarker(root)
	if err != nil {
		return Result{}, err
	}
	if marker["mode"] != "release" {
		return Result{}, errors.New("this is a source install; re-run the release installer to enable updates")
	}

	repo := opts.Repo
	if repo == "" {
		repo = marker["repo"]
	}
	if repo == "" {
		repo = DefaultRepo
	}
	current := NormalizeTag(marker["tag"])
	if current == "" {
		current = NormalizeTag(opts.CurrentVersion)
	}
	if current == "" {
		current = "v0.0.0"
	}

	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string) {}
	}
	base := strings.TrimRight(opts.ReleaseBase, "/")
	if base == "" {
		base = fmt.Sprintf("https://github.com/%s/releases", repo)
	}

	latest := NormalizeTag(opts.Tag)
	if latest == "" {
		logf("checking " + repo)
		latest, err = latestTag(ctx, client, base)
		if err != nil {
			return Result{}, err
		}
	}
	result := Result{Current: current, Latest: latest}
	if opts.Check || (latest == current && !opts.Force) {
		return result, nil
	}

	binary := filepath.Join(root, "bin", BinaryName)
	if info, err := os.Lstat(binary); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return Result{}, errors.New("the TUI is linked to a custom binary (MONET_TUI_BIN); update it yourself or re-run the installer")
	}

	asset, label, err := AssetNames(goos, goarch)
	if err != nil {
		return Result{}, err
	}
	logf("downloading " + latest + " for " + label)

	temp, err := os.MkdirTemp("", "monet-update-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(temp)

	checksumsPath := filepath.Join(temp, "checksums.txt")
	if err := download(ctx, client, releaseURL(base, latest, "checksums.txt"), checksumsPath); err != nil {
		return Result{}, err
	}
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return Result{}, err
	}
	sums := ParseChecksums(string(data))

	tuiPath := filepath.Join(temp, asset)
	if err := download(ctx, client, releaseURL(base, latest, asset), tuiPath); err != nil {
		return Result{}, err
	}
	if err := verify(tuiPath, sums, asset); err != nil {
		return Result{}, err
	}

	cliPath := filepath.Join(temp, CLIAsset)
	if err := download(ctx, client, releaseURL(base, latest, CLIAsset), cliPath); err != nil {
		return Result{}, err
	}
	if err := verify(cliPath, sums, CLIAsset); err != nil {
		return Result{}, err
	}

	newCLI := filepath.Join(root, "cli.new")
	oldCLI := filepath.Join(root, "cli.old")
	removeAll(newCLI)
	removeAll(oldCLI)
	if err := extractCLI(cliPath, newCLI); err != nil {
		removeAll(newCLI)
		return Result{}, err
	}
	if _, err := os.Stat(filepath.Join(newCLI, "monet_cli", "__main__.py")); err != nil {
		removeAll(newCLI)
		return Result{}, errors.New("the CLI bundle is incomplete")
	}

	cliDir := filepath.Join(root, "cli")
	hadCLI := false
	if _, err := os.Stat(cliDir); err == nil {
		if err := os.Rename(cliDir, oldCLI); err != nil {
			removeAll(newCLI)
			return Result{}, fmt.Errorf("could not replace the CLI: %w", err)
		}
		hadCLI = true
	}
	if err := os.Rename(newCLI, cliDir); err != nil {
		if hadCLI {
			_ = os.Rename(oldCLI, cliDir)
		}
		removeAll(newCLI)
		return Result{}, fmt.Errorf("could not install the CLI: %w", err)
	}

	if err := replaceBinary(binary, tuiPath); err != nil {
		removeAll(cliDir)
		if hadCLI {
			_ = os.Rename(oldCLI, cliDir)
		}
		return Result{}, err
	}
	removeAll(oldCLI)

	if err := writeMarker(root, repo, latest); err != nil {
		return Result{}, err
	}
	result.Updated = true
	return result, nil
}

func readMarker(root string) (map[string]string, error) {
	path := filepath.Join(root, MarkerName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("this Monet Code copy was not installed from a release; re-run the installer to switch to updates")
		}
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	marker := ParseMarker(string(data))
	if _, err := os.Stat(filepath.Join(root, "bin", BinaryName)); err != nil {
		return nil, fmt.Errorf("%s is missing; re-run the installer", filepath.Join(root, "bin", BinaryName))
	}
	return marker, nil
}

func latestTag(ctx context.Context, client *http.Client, base string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/latest", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "monet-code")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("could not check for releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not check for releases: HTTP %d", response.StatusCode)
	}
	final := response.Request.URL
	if final == nil {
		return "", errors.New("could not determine the latest release")
	}
	tag := NormalizeTag(pathBase(final.Path))
	if tag == "v" || tag == "vlatest" || VersionTuple(tag) == [3]int{} {
		return "", errors.New("no published release found")
	}
	return tag, nil
}

func pathBase(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}
	return path
}

func releaseURL(base, tag, name string) string {
	return base + "/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}

func download(ctx context.Context, client *http.Client, source, dest string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "monet-code")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download %s: HTTP %d", source, response.StatusCode)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, response.Body); err != nil {
		out.Close()
		return fmt.Errorf("could not save %s: %w", dest, err)
	}
	return out.Close()
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func verify(path string, sums map[string]string, name string) error {
	expected, ok := sums[name]
	if !ok || expected == "" {
		return fmt.Errorf("the release does not publish a checksum for %s", name)
	}
	actual, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s; the download was not trusted", name)
	}
	return nil
}

// extractCLI unpacks a verified CLI bundle without allowing paths to escape.
func extractCLI(archive, dest string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("the CLI bundle is not a gzip archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read the CLI bundle: %w", err)
		}
		name := filepath.FromSlash(header.Name)
		if name == "." || name == "" {
			continue
		}
		if filepath.IsAbs(name) {
			return errors.New("the CLI bundle contains an unsafe path")
		}
		target := filepath.Join(root, name)
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("the CLI bundle contains an unsafe path")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, reader); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the CLI bundle contains an unsupported entry %q", header.Name)
		}
	}
	return nil
}

func replaceBinary(binary, source string) error {
	staged := binary + ".new"
	if err := copyFile(source, staged, 0o755); err != nil {
		return fmt.Errorf("could not stage the TUI binary: %w", err)
	}
	if err := os.Rename(staged, binary); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("could not replace the TUI binary: %w", err)
	}
	return nil
}

func copyFile(source, dest string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dest, mode)
}

func writeMarker(root, repo, tag string) error {
	path := filepath.Join(root, MarkerName)
	content := fmt.Sprintf("repo=%s\nmode=release\ntag=%s\nref=%s\ninstalled=%s\n",
		repo, tag, tag, time.Now().UTC().Format(time.RFC3339))
	staged := path + ".new"
	if err := os.WriteFile(staged, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Rename(staged, path); err != nil {
		_ = os.Remove(staged)
		return err
	}
	return nil
}

func removeAll(path string) {
	_ = os.RemoveAll(path)
}
