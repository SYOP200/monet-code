// Package agent implements Monet Code's local coding agent: workspace-scoped
// tools, an approval gate for writes, and a streaming tool-calling loop that
// speaks Ollama's native /api/chat protocol.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syop200/monet-code/internal/files"
	"github.com/syop200/monet-code/internal/ollama"
)

// Executor runs tools inside a single workspace root.
type Executor struct {
	Root     string
	ReadOnly bool
	Hooks    Hooks
}

// NewExecutor creates an executor rooted at root.
func NewExecutor(root string) *Executor {
	return &Executor{Root: root, Hooks: LoadHooks(root)}
}

// NewReadOnlyExecutor creates an executor that can inspect a workspace but not
// mutate it. Subagents use this.
func NewReadOnlyExecutor(root string) *Executor {
	return &Executor{Root: root, ReadOnly: true}
}

// Definitions returns the tool schema advertised to the model.
func (e *Executor) Definitions() []ollama.Tool {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	num := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	tools := []ollama.Tool{
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
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "delete_file",
				Description: "Delete one regular file inside the workspace. Directories are never deleted recursively. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": str("File path relative to the workspace root"),
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "move_file",
				Description: "Move or rename one file inside the workspace. Refuses to overwrite an existing file. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from": str("Existing file path relative to the workspace root"),
						"to":   str("New file path relative to the workspace root"),
					},
					"required": []string{"from", "to"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "run_command",
				Description: "Run a shell command inside the workspace, e.g. go test ./..., npm run build or git status. Destructive commands (rm -rf, sudo, mkfs, git clean) are refused. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":         str("Shell command to run"),
						"cwd":             str("Working directory relative to the workspace root (optional)"),
						"timeout_seconds": num("Timeout in seconds, 1-300 (optional, default 60)"),
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "git_status",
				Description: "Show short git status for the workspace.",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "git_diff",
				Description: "Show the working-tree diff, optionally limited to a path.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": str("Optional path relative to the workspace root"),
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "git_log",
				Description: "Show recent commits in one-line form.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"max": num("Number of commits, 1-50 (optional, default 10)"),
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "git_commit",
				Description: "Stage all changes and create a commit with the given message. Requires user approval.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message": str("Commit message"),
					},
					"required": []string{"message"},
				},
			},
		},
	}
	if e.ReadOnly {
		allowed := map[string]bool{
			"read_file": true, "list_files": true, "search_files": true,
			"git_status": true, "git_diff": true, "git_log": true,
		}
		var filtered []ollama.Tool
		for _, tool := range tools {
			if allowed[tool.Function.Name] {
				filtered = append(filtered, tool)
			}
		}
		return filtered
	}
	return tools
}

func readOnlyTool(name string) bool {
	switch name {
	case "read_file", "list_files", "search_files", "git_status", "git_diff", "git_log":
		return true
	}
	return false
}

// RequiresApproval reports whether a tool mutates the workspace.
func (e *Executor) RequiresApproval(name string) bool {
	if e.ReadOnly {
		return false
	}
	switch name {
	case "write_file", "create_directory", "delete_file", "move_file", "run_command", "git_commit":
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
	case "delete_file":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		return a.Path
	case "move_file":
		var a struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		_ = json.Unmarshal(args, &a)
		return a.From + " → " + a.To
	case "run_command":
		var a struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
		}
		_ = json.Unmarshal(args, &a)
		if a.Cwd != "" {
			return a.Command + "  (in " + a.Cwd + ")"
		}
		return a.Command
	case "git_diff":
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &a)
		if a.Path == "" {
			return "working tree"
		}
		return a.Path
	case "git_log":
		var a struct {
			Max int `json:"max"`
		}
		_ = json.Unmarshal(args, &a)
		return fmt.Sprintf("%d commits", maxInt(a.Max, 10))
	case "git_commit":
		var a struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(args, &a)
		return a.Message
	}
	return ""
}

// Execute runs a tool call with a background context.
func (e *Executor) Execute(name string, args json.RawMessage) (string, error) {
	return e.ExecuteContext(context.Background(), name, args)
}

// ExecuteContext runs a tool call and returns the text handed back to the model.
func (e *Executor) ExecuteContext(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if e.ReadOnly && !readOnlyTool(name) {
		return "", fmt.Errorf("read-only subagent cannot call %s", name)
	}
	switch name {
	case "read_file":
		return e.readFile(args)
	case "write_file":
		return e.writeFile(ctx, args)
	case "list_files":
		return e.listFiles(args)
	case "search_files":
		return e.searchFiles(args)
	case "create_directory":
		return e.createDir(args)
	case "delete_file":
		return e.deleteFile(ctx, args)
	case "move_file":
		return e.moveFile(ctx, args)
	case "run_command":
		return e.runCommand(ctx, args)
	case "git_status":
		return e.gitStatus(ctx)
	case "git_diff":
		return e.gitDiff(ctx, args)
	case "git_log":
		return e.gitLog(ctx, args)
	case "git_commit":
		return e.gitCommit(ctx, args)
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

func (e *Executor) writeFile(ctx context.Context, args json.RawMessage) (string, error) {
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
	abs, err := e.mutablePath(a.Path)
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
	if _, err := files.Snapshot(e.Root, files.Rel(e.Root, abs), "write_file"); err != nil {
		return "", fmt.Errorf("could not checkpoint before writing: %w", err)
	}
	if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	action := "created"
	if existed {
		action = "updated"
	}
	result := fmt.Sprintf("%s %s (%d bytes, %d lines)", action, files.Rel(e.Root, abs), len(a.Content), strings.Count(a.Content, "\n")+1)
	if out, hookErr := e.Hooks.Run(ctx, e.Root, "after_edit", map[string]string{"MONET_FILE": files.Rel(e.Root, abs)}); hookErr != nil {
		result += fmt.Sprintf("\nhook after_edit failed: %v\n%s", hookErr, out)
	} else if strings.TrimSpace(out) != "" {
		result += "\nhook after_edit:\n" + out
	}
	return result, nil
}

// mutablePath rejects Monet's own state and git internals.
func (e *Executor) mutablePath(rel string) (string, error) {
	abs, err := files.SafeJoin(e.Root, rel)
	if err != nil {
		return "", err
	}
	clean := files.Rel(e.Root, abs)
	if clean == ".git" || strings.HasPrefix(clean, ".git/") || clean == ".monet" || strings.HasPrefix(clean, ".monet/") {
		return "", fmt.Errorf("refusing to modify %s", clean)
	}
	return abs, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = time.Second

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
