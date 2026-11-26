package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"0.2.0":  "v0.2.0",
		"v0.2.0": "v0.2.0",
		"":       "",
		"  v1":   "v1",
	}
	for input, want := range cases {
		if got := NormalizeTag(input); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v0.2.1", "v0.2.0") {
		t.Fatal("0.2.1 should be newer than 0.2.0")
	}
	if !IsNewer("v0.10.0", "v0.9.9") {
		t.Fatal("numeric comparison failed")
	}
	if IsNewer("v0.2.0", "v0.2.0") || IsNewer("v0.2.0", "v0.2.1") {
		t.Fatal("older or equal tags must not be newer")
	}
}

func TestAssetNames(t *testing.T) {
	asset, label, err := AssetNames("linux", "x86_64")
	if err != nil || asset != "monet-tui-linux-amd64" || label != "linux amd64" {
		t.Fatalf("got %q %q %v", asset, label, err)
	}
	if _, _, err := AssetNames("plan9", "amd64"); err == nil {
		t.Fatal("unsupported OS should fail")
	}
	if _, _, err := AssetNames("linux", "mips"); err == nil {
		t.Fatal("unsupported architecture should fail")
	}
}

func TestParseChecksums(t *testing.T) {
	sums := ParseChecksums("ABC  one\nDEF *two\nbad line\n")
	if sums["one"] != "abc" || sums["two"] != "def" {
		t.Fatalf("unexpected checksums: %#v", sums)
	}
}

func TestRunInstallsLocalRelease(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	release := t.TempDir()
	files := makeRelease(t, release, "monet-tui-linux-amd64", "new tui", "new cli")
	server := serveRelease(t, release, "v0.2.1", files)
	defer server.Close()

	result, err := Run(context.Background(), Options{
		Root:        root,
		Repo:        "acme/monet",
		Tag:         "v0.2.1",
		ReleaseBase: server.URL,
		GOOS:        "linux",
		GOARCH:      "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Current != "v0.2.0" || result.Latest != "v0.2.1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := read(t, filepath.Join(root, "bin", "monet-tui")); got != "new tui" {
		t.Fatalf("binary = %q", got)
	}
	if got := read(t, filepath.Join(root, "cli", "monet_cli", "__main__.py")); got != "new cli" {
		t.Fatalf("cli = %q", got)
	}
	marker, err := os.ReadFile(filepath.Join(root, MarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(marker), "tag=v0.2.1") {
		t.Fatalf("marker not updated: %s", marker)
	}
	if _, err := os.Stat(filepath.Join(root, "cli.old")); !os.IsNotExist(err) {
		t.Fatal("cli.old should be removed")
	}
	if _, err := os.Stat(filepath.Join(root, "cli.new")); !os.IsNotExist(err) {
		t.Fatal("cli.new should be removed")
	}
}

func TestRunChecksumMismatchKeepsInstall(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	release := t.TempDir()
	files := makeRelease(t, release, "monet-tui-linux-amd64", "new tui", "new cli")
	files["checksums.txt"] = []byte(strings.Repeat("0", 64) + "  monet-tui-linux-amd64\n")
	server := serveRelease(t, release, "v0.2.1", files)
	defer server.Close()

	_, err := Run(context.Background(), Options{
		Root:        root,
		Tag:         "v0.2.1",
		ReleaseBase: server.URL,
		GOOS:        "linux",
		GOARCH:      "amd64",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if got := read(t, filepath.Join(root, "bin", "monet-tui")); got != "old tui" {
		t.Fatalf("binary changed to %q", got)
	}
}

func TestRunCheckDoesNotChangeFiles(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	server := serveRelease(t, t.TempDir(), "v0.9.0", map[string][]byte{})
	defer server.Close()

	result, err := Run(context.Background(), Options{
		Root:        root,
		Check:       true,
		ReleaseBase: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.Latest != "v0.9.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := read(t, filepath.Join(root, "bin", "monet-tui")); got != "old tui" {
		t.Fatalf("binary changed to %q", got)
	}
}

func TestRunUpToDateIsNoop(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	result, err := Run(context.Background(), Options{Root: root, Tag: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.Current != "v0.2.0" || result.Latest != "v0.2.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunRejectsSourceInstall(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	write(t, filepath.Join(root, MarkerName), "repo=acme/monet\nmode=source\nref=main\n")
	if _, err := Run(context.Background(), Options{Root: root, Tag: "v0.2.1"}); err == nil {
		t.Fatal("source install should be rejected")
	}
}

func TestRunRejectsCustomBinarySymlink(t *testing.T) {
	root := t.TempDir()
	makeInstall(t, root, "v0.2.0")
	target := filepath.Join(t.TempDir(), "custom-tui")
	write(t, target, "custom")
	binary := filepath.Join(root, "bin", "monet-tui")
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, binary); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Options{Root: root, Tag: "v0.2.1"}); err == nil {
		t.Fatal("symlinked binary should be rejected")
	}
}

func TestLatestTagFollowsRedirect(t *testing.T) {
	server := serveRelease(t, t.TempDir(), "v0.3.0", map[string][]byte{})
	defer server.Close()
	tag, err := latestTag(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.3.0" {
		t.Fatalf("tag = %q", tag)
	}
}

func makeInstall(t *testing.T, root, tag string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cli", "monet_cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "bin", "monet-tui"), "old tui")
	write(t, filepath.Join(root, "cli", "monet_cli", "__main__.py"), "old cli")
	write(t, filepath.Join(root, MarkerName), fmt.Sprintf("repo=acme/monet\nmode=release\ntag=%s\nref=%s\n", tag, tag))
}

// makeRelease writes a release tree and returns its asset contents so the test
// server can serve them under any tag.
func makeRelease(t *testing.T, dir, asset, tui, cli string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{asset: []byte(tui)}
	files[CLIAsset] = tarGz(t, map[string]string{"monet_cli/__main__.py": cli})
	var sums bytes.Buffer
	for name, data := range files {
		digest := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	files["checksums.txt"] = sums.Bytes()
	return files
}

func tarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serveRelease(t *testing.T, dir, tag string, files map[string][]byte) *httptest.Server {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(dir, tag, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/latest":
			http.Redirect(w, r, "/tag/"+tag, http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/tag/"):
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/download/"):
			rest := strings.TrimPrefix(r.URL.Path, "/download/")
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) != 2 || parts[0] != tag {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(dir, tag, parts[1]))
		default:
			http.NotFound(w, r)
		}
	}))
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
