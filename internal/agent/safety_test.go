package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDangerousCommandBlocksDestruction(t *testing.T) {
	blocked := []string{
		"rm -rf .",
		"rm -r src",
		"rm --recursive --force build",
		"sudo rm file",
		"git clean -fdx",
		"git reset --hard HEAD",
		"git push --force origin main",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		"find . -delete",
	}
	for _, command := range blocked {
		if reason := DangerousCommand(command); reason == "" {
			t.Errorf("expected %q to be blocked", command)
		}
	}
	allowed := []string{
		"go test ./...",
		"npm run build",
		"git status --short",
		"git diff --stat",
		"rm one-file.txt",
		"mkdir -p internal/new",
		"gofmt -w internal/ui/app.go",
	}
	for _, command := range allowed {
		if reason := DangerousCommand(command); reason != "" {
			t.Errorf("expected %q to be allowed, got %q", command, reason)
		}
	}
}

func TestRunShellWorkspaceCWD(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := RunShell(context.Background(), root, "pwd", "sub", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(res.Output), "/sub") {
		t.Fatalf("cwd output = %q", res.Output)
	}
	if _, err := RunShell(context.Background(), root, "pwd", "../..", 10*time.Second); err == nil {
		t.Fatal("expected escaping cwd to be refused")
	}
	if _, err := RunShell(context.Background(), root, "rm -rf .", "", 10*time.Second); err == nil {
		t.Fatal("expected rm -rf to be refused")
	}
}

func TestDeleteAndMoveTools(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)
	if _, err := e.Execute("write_file", raw(`{"path":"a.txt","content":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute("delete_file", raw(`{"path":"."}`)); err == nil {
		t.Fatal("delete_file must refuse directories")
	}
	if _, err := e.Execute("move_file", raw(`{"from":"a.txt","to":"b.txt"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("move did not create b.txt: %v", err)
	}
	if _, err := e.Execute("delete_file", raw(`{"path":"b.txt"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("delete_file did not remove b.txt")
	}
	if _, err := e.Execute("delete_file", raw(`{"path":".git/config"}`)); err == nil {
		t.Fatal("delete_file must refuse .git")
	}
}

func TestReadOnlySubagentCannotMutate(t *testing.T) {
	root := t.TempDir()
	e := NewReadOnlyExecutor(root)
	if _, err := e.Execute("write_file", raw(`{"path":"x.txt","content":"nope"}`)); err == nil {
		t.Fatal("read-only executor accepted write_file")
	}
	if _, err := e.Execute("run_command", raw(`{"command":"touch x.txt"}`)); err == nil {
		t.Fatal("read-only executor accepted run_command")
	}
	for _, tool := range e.Definitions() {
		if !readOnlyTool(tool.Function.Name) {
			t.Errorf("read-only executor advertised %q", tool.Function.Name)
		}
	}
	if len(e.Definitions()) == 0 {
		t.Fatal("read-only executor advertised no tools")
	}
}

func TestAfterEditHookRuns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".monet"), 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := `{"after_edit":"echo $MONET_FILE > edited.txt"}`
	if err := os.WriteFile(filepath.Join(root, ".monet", "hooks.json"), []byte(hooks), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(root)
	if _, err := e.Execute("write_file", raw(`{"path":"src/main.go","content":"package main\n"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "edited.txt"))
	if err != nil {
		t.Fatalf("after_edit hook did not run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "src/main.go" {
		t.Fatalf("hook saw %q", data)
	}
}

func TestReloadHooks(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)
	if e.Hooks.AfterEdit != "" {
		t.Fatal("unexpected hooks in a fresh workspace")
	}
	if err := os.MkdirAll(filepath.Join(root, ".monet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".monet", "hooks.json"), []byte(`{"after_edit":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e.ReloadHooks()
	if e.Hooks.AfterEdit != "true" {
		t.Fatalf("hooks were not reloaded: %+v", e.Hooks)
	}
}

func TestGitTools(t *testing.T) {
	root := t.TempDir()
	e := NewExecutor(root)
	ctx := context.Background()
	if out, err := e.git(ctx, "init", "-q"); err != nil {
		t.Skipf("git unavailable: %v (%s)", err, out)
	}
	_, _ = e.git(ctx, "config", "user.email", "monet@example.com")
	_, _ = e.git(ctx, "config", "user.name", "Monet Test")

	status, err := e.ExecuteContext(ctx, "git_status", raw(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "No commits") && !strings.Contains(status, "##") {
		t.Errorf("git_status = %q", status)
	}
	if _, err := e.ExecuteContext(ctx, "write_file", raw(`{"path":"note.txt","content":"hi\n"}`)); err != nil {
		t.Fatal(err)
	}
	diff, err := e.ExecuteContext(ctx, "git_diff", raw(`{}`))
	if err != nil {
		t.Fatalf("git_diff on untracked file: %v", err)
	}
	_ = diff
	commit, err := e.ExecuteContext(ctx, "git_commit", raw(`{"message":"first"}`))
	if err != nil {
		t.Fatalf("git_commit: %v", err)
	}
	if !strings.Contains(commit, "first") {
		t.Errorf("commit output = %q", commit)
	}
	log, err := e.ExecuteContext(ctx, "git_log", raw(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "first") {
		t.Errorf("git_log = %q", log)
	}
	if _, err := os.Stat(filepath.Join(root, ".monet")); err != nil {
		t.Fatalf("checkpoint directory missing: %v", err)
	}
}
