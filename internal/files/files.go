// Package files implements Monet Code's filesystem helpers: safe path
// handling, text loading, directory trees, text search, and the file
// organizer that sorts loose top-level files into category folders.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Category is a destination bucket for the file organizer.
type Category struct {
	Name   string
	Folder string
	Exts   []string
}

// Categories are checked in order. Extensionless and unknown files land in
// "Other".
var Categories = []Category{
	{Name: "Documents", Folder: "Documents", Exts: []string{"pdf", "doc", "docx", "txt", "md", "markdown", "rtf", "pages", "epub", "odt", "tex", "rst"}},
	{Name: "Images", Folder: "Images", Exts: []string{"png", "jpg", "jpeg", "gif", "svg", "webp", "heic", "tiff", "bmp", "ico", "avif"}},
	{Name: "Video", Folder: "Video", Exts: []string{"mp4", "mov", "mkv", "avi", "webm", "m4v", "flv"}},
	{Name: "Audio", Folder: "Audio", Exts: []string{"mp3", "wav", "flac", "aac", "ogg", "m4a", "opus"}},
	{Name: "Archives", Folder: "Archives", Exts: []string{"zip", "tar", "gz", "bz2", "xz", "7z", "rar", "tgz", "dmg", "iso"}},
	{Name: "Code", Folder: "Code", Exts: []string{"go", "py", "js", "ts", "tsx", "jsx", "rs", "c", "h", "cpp", "hpp", "cc", "java", "kt", "rb", "php", "swift", "sh", "bash", "zsh", "fish", "html", "css", "scss", "less", "json", "yaml", "yml", "toml", "xml", "sql", "lua", "pl", "r", "vue", "svelte"}},
	{Name: "Data", Folder: "Data", Exts: []string{"csv", "tsv", "xlsx", "xls", "jsonl", "parquet", "db", "sqlite", "sqlite3", "ndjson", "avro"}},
}

// Other is the fallback category.
var Other = Category{Name: "Other", Folder: "Other"}

// CategoryFor picks a category from a filename's extension.
func CategoryFor(name string) Category {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	for _, c := range Categories {
		for _, e := range c.Exts {
			if ext == e {
				return c
			}
		}
	}
	return Other
}

// Group is a category's loose files.
type Group struct {
	Category Category
	Files    []string // base names, top level only
	Size     int64
}

// Move is a single planned rename.
type Move struct {
	From     string // absolute source
	To       string // absolute destination
	RelFrom  string
	RelTo    string
	Category string
	Size     int64
}

// Report is the result of analyzing a directory.
type Report struct {
	Root      string
	Groups    []Group
	FileCount int
	Size      int64
	Invalid   int // unreadable entries
}

// Analyze inspects the top-level files of root. Directories are never opened,
// hidden files are ignored, and only files that would actually move are
// counted.
func Analyze(root string) (Report, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Root: root}
	byCat := map[string]*Group{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			rep.Invalid++
			continue
		}
		cat := CategoryFor(e.Name())
		g := byCat[cat.Name]
		if g == nil {
			g = &Group{Category: cat}
			byCat[cat.Name] = g
		}
		g.Files = append(g.Files, e.Name())
		g.Size += info.Size()
		rep.FileCount++
		rep.Size += info.Size()
	}
	order := map[string]int{}
	for i, c := range Categories {
		order[c.Name] = i
	}
	order[Other.Name] = len(Categories)
	for _, g := range byCat {
		sort.Strings(g.Files)
		rep.Groups = append(rep.Groups, *g)
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		if order[rep.Groups[i].Category.Name] != order[rep.Groups[j].Category.Name] {
			return order[rep.Groups[i].Category.Name] < order[rep.Groups[j].Category.Name]
		}
		return rep.Groups[i].Category.Name < rep.Groups[j].Category.Name
	})
	return rep, nil
}

// Plan builds the move list for an analyzed report. It creates category
// folders lazily and resolves filename collisions with " (n)" suffixes.
func Plan(rep Report) ([]Move, error) {
	var moves []Move
	used := map[string]bool{}
	for _, g := range rep.Groups {
		for _, name := range g.Files {
			from := filepath.Join(rep.Root, name)
			to, relTo, err := uniqueDestination(rep.Root, g.Category.Folder, name, used)
			if err != nil {
				return nil, err
			}
			moves = append(moves, Move{
				From:     from,
				To:       to,
				RelFrom:  name,
				RelTo:    filepath.ToSlash(relTo),
				Category: g.Category.Name,
				Size:     fileSize(from),
			})
		}
	}
	return moves, nil
}

func uniqueDestination(root, folder, name string, used map[string]bool) (abs string, rel string, err error) {
	dir := filepath.Join(root, folder)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if !used[candidate] {
			if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
				used[candidate] = true
				rel, rerr := filepath.Rel(root, candidate)
				if rerr != nil {
					return "", "", rerr
				}
				return candidate, rel, nil
			}
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
}

// Apply performs the planned moves, returning the number moved and any errors.
func Apply(moves []Move) (int, []error) {
	moved := 0
	var errs []error
	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.To), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.RelTo, err))
			continue
		}
		if err := os.Rename(m.From, m.To); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.RelFrom, err))
			continue
		}
		moved++
	}
	return moved, errs
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// SafeJoin joins rel onto root and verifies the result stays inside root.
func SafeJoin(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	p := rel
	if !filepath.IsAbs(p) {
		p = filepath.Join(rootAbs, rel)
	}
	p = filepath.Clean(p)
	if p != rootAbs && !strings.HasPrefix(p, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the workspace", rel)
	}
	return p, nil
}

// Rel returns a slash-separated path relative to root.
func Rel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// ReadText reads a UTF-8-ish text file up to maxBytes, reporting truncation.
func ReadText(path string, maxBytes int64) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxBytes {
		return "", false, fmt.Errorf("%s is too large (%.1f MB, limit %.1f MB)", path, float64(info.Size())/1e6, float64(maxBytes)/1e6)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if !IsText(data) {
		return "", false, fmt.Errorf("%s looks like a binary file", path)
	}
	return string(data), false, nil
}

// IsText reports whether data is likely text (no NUL bytes in the first 8 KiB).
func IsText(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	for _, b := range data[:n] {
		if b == 0 {
			return false
		}
	}
	return true
}

// HumanBytes formats a byte count.
func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// HumanAge renders a coarse relative age.
func HumanAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// SkipDirs are noisy directories the agent and search walkers avoid.
var SkipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true,
	"vendor": true, ".venv": true, "venv": true, "__pycache__": true,
	".mypy_cache": true, ".pytest_cache": true, ".ruff_cache": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true, "target": true,
	".idea": true, ".vscode": true, ".DS_Store": true, ".cache": true,
	"coverage": true, ".terraform": true, "Pods": true,
	".monet": true,
}

// ShouldSkipDir reports whether a directory is build output, a cache or a
// tooling folder that should stay out of trees, listings and searches.
func ShouldSkipDir(name string) bool {
	return SkipDirs[name] || strings.HasSuffix(name, ".egg-info")
}

// SummaryTree lists the workspace's top level for the agent's system prompt.
func SummaryTree(root string, max int) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var dirs, files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e.Name()+"/")
		} else {
			files = append(files, e.Name())
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	all := append(dirs, files...)
	if len(all) > max {
		all = all[:max]
		all = append(all, "…")
	}
	return strings.Join(all, "  ")
}

// SearchMatch is one text search hit.
type SearchMatch struct {
	Path string
	Line int
	Text string
}

// Search walks root for pattern, returning at most max hits. Query is treated
// as a case-insensitive literal substring.
func Search(root, query string, max int) ([]SearchMatch, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	var matches []SearchMatch
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (ShouldSkipDir(d.Name()) || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= max {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".go") &&
			!strings.HasSuffix(strings.ToLower(path), ".py") &&
			!strings.HasSuffix(strings.ToLower(path), ".md") &&
			!strings.HasSuffix(strings.ToLower(path), ".json") &&
			!strings.HasSuffix(strings.ToLower(path), ".yaml") &&
			!strings.HasSuffix(strings.ToLower(path), ".yml") &&
			!strings.HasSuffix(strings.ToLower(path), ".toml") &&
			!strings.HasSuffix(strings.ToLower(path), ".txt") &&
			!strings.HasSuffix(strings.ToLower(path), ".js") &&
			!strings.HasSuffix(strings.ToLower(path), ".ts") &&
			!strings.HasSuffix(strings.ToLower(path), ".tsx") &&
			!strings.HasSuffix(strings.ToLower(path), ".jsx") &&
			!strings.HasSuffix(strings.ToLower(path), ".css") &&
			!strings.HasSuffix(strings.ToLower(path), ".html") &&
			!strings.HasSuffix(strings.ToLower(path), ".rs") &&
			!strings.HasSuffix(strings.ToLower(path), ".sh") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !IsText(data) {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), query) {
				text := strings.TrimSpace(line)
				if len(text) > 200 {
					text = text[:200] + "…"
				}
				matches = append(matches, SearchMatch{Path: Rel(root, path), Line: i + 1, Text: text})
				if len(matches) >= max {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return matches, err
}
