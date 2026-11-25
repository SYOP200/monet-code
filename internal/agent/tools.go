// Package agent implements Monet Code's local coding agent: workspace-scoped
// tools, an approval gate for writes, and a streaming tool-calling loop that
// speaks Ollama's native /api/chat protocol.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syop200/monet-code/internal/files"
	"github.com/syop200/monet-code/internal/ollama"
)

// Executor runs tools inside a single workspace root.
type Executor struct {
	Root string
}

// NewExecutor creates an executor rooted at root.
func NewExecutor(root string) *Executor { return &Executor{Root: root} }

// Definitions returns the tool schema advertised to the model.
func (e *Executor) Definitions() []ollama.Tool {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	num := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	return []ollama.Tool{
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "read_file",
				Description: "Read a UTF-8 text file from the workspace. Returns numbered lines. Use offset/limit to page through long files.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":   str("Path relative to the workspace root, e.g. internal/ui/app.go"),
						"offset": num("1-based line to start from (optional)"),
						"limit":  num("Maximum number of lines to return (optional, default 400)"),
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "write_file",
				Description: "Create or overwrite a text file in the workspace. Always provide the complete new file content. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    str("Path relative to the workspace root"),
						"content": str("The full file contents to write"),
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "list_files",
				Description: "List files and directories under a workspace path. Directories are suffixed with /.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  str("Directory relative to the workspace root (optional, defaults to .)"),
						"depth": num("Recursion depth, 1-3 (optional, default 1)"),
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "search_files",
				Description: "Case-insensitive text search across workspace source and docs. Returns file:line matches.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": str("Literal text to search for"),
						"max":   num("Maximum matches (optional, default 40)"),
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "create_directory",
				Description: "Create a directory (and parents) inside the workspace. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": str("Directory path relative to the workspace root"),
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

// RequiresApproval reports whether a tool mutates the workspace.
func (e *Executor) RequiresApproval(name string) bool {
	switch name {
	case "write_file", "create_directory":
		return true
	}
	return false
}

// DescribeCall summarizes a tool call for the approval banner / chat card.
func (e *Executor) DescribeCall(name string, args json.RawMessage) string {
	switch name {
	case "read_file":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		return a.Path
	case "write_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(args, &a)
		lines := strings.Count(a.Content, "\n") + 1
		return fmt.Sprintf("%s · %d lines", a.Path, lines)
	case "list_files":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		if a.Path == "" {
			a.Path = "."
		}
		return a.Path
	case "search_files":
		var a struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(args, &a)
		return fmt.Sprintf("%q", a.Query)
	case "create_directory":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		return a.Path
	}
	return ""
}

// Execute runs a tool call and returns the text handed back to the model.
func (e *Executor) Execute(name string, args json.RawMessage) (string, error) {
	switch name {
	case "read_file":
		return e.readFile(args)
	case "write_file":
		return e.writeFile(args)
	case "list_files":
		return e.listFiles(args)
	case "search_files":
		return e.searchFiles(args)
	case "create_directory":
		return e.createDir(args)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func (e *Executor) readFile(args json.RawMessage) (string, error) {
	var a struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if a.Limit <= 0 || a.Limit > 1000 {
		a.Limit = 400
	}
	abs, err := files.SafeJoin(e.Root, a.Path)
	if err != nil {
		return "", err
	}
	text, _, err := files.ReadText(abs, 2<<20)
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	start := a.Offset
	if start <= 0 {
		start = 1
	}
	if start > len(lines) {
		return "", fmt.Errorf("offset %d is past end of file (%d lines)", start, len(lines))
	}
	end := start - 1 + a.Limit
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	rel := files.Rel(e.Root, abs)
	fmt.Fprintf(&b, "%s (lines %d-%d of %d)\n", rel, start, end, len(lines))
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%4d │ %s\n", i, lines[i-1])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "… %d more lines (use offset=%d)\n", len(lines)-end, end+1)
	}
	return b.String(), nil
}

func (e *Executor) writeFile(args json.RawMessage) (string, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasSuffix(a.Path, "/") {
		return "", fmt.Errorf("path %q is a directory", a.Path)
	}
	if len(a.Content) > 2<<20 {
		return "", fmt.Errorf("refusing to write more than 2 MB")
	}
	abs, err := files.SafeJoin(e.Root, a.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	existed := false
	if _, statErr := os.Stat(abs); statErr == nil {
		existed = true
	}
	if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	action := "created"
	if existed {
		action = "updated"
	}
	return fmt.Sprintf("%s %s (%d bytes, %d lines)", action, files.Rel(e.Root, abs), len(a.Content), strings.Count(a.Content, "\n")+1), nil
}

func (e *Executor) listFiles(args json.RawMessage) (string, error) {
	var a struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Depth <= 0 || a.Depth > 3 {
		a.Depth = 1
	}
	if a.Path == "" {
		a.Path = "."
	}
	abs, err := files.SafeJoin(e.Root, a.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", a.Path)
	}
	prefix := files.Rel(e.Root, abs)
	if prefix == "." {
		prefix = ""
	}
	var lines []string
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := files.Rel(abs, path)
		if rel == "." {
			return nil
		}
		parts := strings.Split(rel, "/")
		if d.IsDir() && files.ShouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		if len(parts) > a.Depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		indent := strings.Repeat("  ", len(parts)-1)
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		lines = append(lines, indent+name)
		if len(lines) >= 250 {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	header := fmt.Sprintf("%s (depth %d)\n", filepath.ToSlash(filepath.Join(prefix)), a.Depth)
	if len(lines) == 0 {
		return header + "(empty)", nil
	}
	if len(lines) >= 250 {
		lines = append(lines, "… listing truncated")
	}
	return header + strings.Join(lines, "\n"), nil
}

func (e *Executor) searchFiles(args json.RawMessage) (string, error) {
	var a struct {
		Query string `json:"query"`
		Max   int    `json:"max"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Max <= 0 || a.Max > 100 {
		a.Max = 40
	}
	matches, err := files.Search(e.Root, a.Query, a.Max)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return fmt.Sprintf("no matches for %q", a.Query), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q\n", len(matches), a.Query)
	for _, m := range matches {
		fmt.Fprintf(&b, "%s:%d: %s\n", m.Path, m.Line, m.Text)
	}
	return b.String(), nil
}

func (e *Executor) createDir(args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := files.SafeJoin(e.Root, a.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return "created directory " + files.Rel(e.Root, abs), nil
}
