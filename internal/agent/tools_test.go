package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiresApproval(t *testing.T) {
	e := NewExecutor(t.TempDir())
	for name, want := range map[string]bool{
		"read_file":        false,
		"list_files":       false,
		"search_files":     false,
		"write_file":       true,
		"create_directory": true,
	} {
		if got := e.RequiresApproval(name); got != want {
			t.Errorf("RequiresApproval(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestWriteReadAndList(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)

	result, err := e.Execute("write_file", raw(`{"path":"src/main.go","content":"package main\n\nfunc main() {}\n"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "created src/main.go") {
		t.Errorf("unexpected write result: %q", result)
	}

	result, err = e.Execute("read_file", raw(`{"path":"src/main.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "package main") || !strings.Contains(result, "1 │") {
		t.Errorf("read result missing content or line numbers: %q", result)
	}

	result, err = e.Execute("list_files", raw(`{"path":".","depth":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "src/") || !strings.Contains(result, "main.go") {
		t.Errorf("list result = %q", result)
	}

	if _, err := os.Stat(filepath.Join(root, "src", "main.go")); err != nil {
		t.Fatalf("file was not written: %v", err)
	}
}

func TestToolPathEscapeBlocked(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)
	calls := []struct {
		tool string
		args string
	}{
		{"read_file", `{"path":"../../etc/passwd"}`},
		{"write_file", `{"path":"../escape.txt","content":"nope"}`},
		{"list_files", `{"path":".."}`},
		{"create_directory", `{"path":"../newdir"}`},
	}
	for _, call := range calls {
		if _, err := e.Execute(call.tool, raw(call.args)); err == nil {
			t.Errorf("%s(%s) should be rejected", call.tool, call.args)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Fatal("write_file escaped the workspace")
	}
}

func TestCreateDirectoryAndSearch(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)
	if _, err := e.Execute("create_directory", raw(`{"path":"docs/api"}`)); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "docs", "api")); err != nil || !info.IsDir() {
		t.Fatalf("directory was not created: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "api", "note.md"), []byte("needle in a haystack\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := e.Execute("search_files", raw(`{"query":"needle"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "note.md") {
		t.Errorf("search result = %q", result)
	}
}

func TestUnknownTool(t *testing.T) {
	e := NewExecutor(t.TempDir())
	if _, err := e.Execute("delete_everything", raw(`{}`)); err == nil {
		t.Fatal("unknown tool should fail")
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
