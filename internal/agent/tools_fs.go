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
)

func (e *Executor) deleteFile(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	abs, err := e.mutablePath(a.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory; recursive deletion is not available", a.Path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", a.Path)
	}
	if _, err := files.Snapshot(e.Root, files.Rel(e.Root, abs), "delete_file"); err != nil {
		return "", fmt.Errorf("could not checkpoint before deleting: %w", err)
	}
	if err := os.Remove(abs); err != nil {
		return "", err
	}
	result := "deleted " + files.Rel(e.Root, abs)
	if out, hookErr := e.Hooks.Run(ctx, e.Root, "after_edit", map[string]string{"MONET_FILE": files.Rel(e.Root, abs)}); hookErr != nil {
		result += fmt.Sprintf("\nhook after_edit failed: %v\n%s", hookErr, out)
	} else if strings.TrimSpace(out) != "" {
		result += "\nhook after_edit:\n" + out
	}
	return result, nil
}

func (e *Executor) moveFile(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	from, err := e.mutablePath(a.From)
	if err != nil {
		return "", err
	}
	to, err := e.mutablePath(a.To)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(from)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", a.From)
	}
	if _, err := os.Lstat(to); err == nil {
		return "", fmt.Errorf("%s already exists; refusing to overwrite", a.To)
	}
	if _, err := files.Snapshot(e.Root, files.Rel(e.Root, from), "move_file"); err != nil {
		return "", fmt.Errorf("could not checkpoint before moving: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(from, to); err != nil {
		return "", err
	}
	result := fmt.Sprintf("moved %s → %s", files.Rel(e.Root, from), files.Rel(e.Root, to))
	if out, hookErr := e.Hooks.Run(ctx, e.Root, "after_edit", map[string]string{"MONET_FILE": files.Rel(e.Root, to)}); hookErr != nil {
		result += fmt.Sprintf("\nhook after_edit failed: %v\n%s", hookErr, out)
	} else if strings.TrimSpace(out) != "" {
		result += "\nhook after_edit:\n" + out
	}
	return result, nil
}

func (e *Executor) runCommand(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Command        string `json:"command"`
		Cwd            string `json:"cwd"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	timeout := time.Duration(a.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	res, err := RunShell(ctx, e.Root, a.Command, a.Cwd, timeout)
	out := res.Output
	if out == "" {
		out = "(no output)"
	}
	header := fmt.Sprintf("$ %s\nexit %d\n", a.Command, res.ExitCode)
	if err != nil {
		header += "error: " + err.Error() + "\n"
	}
	result := header + out
	if hookOut, hookErr := e.Hooks.Run(ctx, e.Root, "after_tool", map[string]string{
		"MONET_TOOL":    "run_command",
		"MONET_COMMAND": a.Command,
	}); hookErr != nil {
		result += fmt.Sprintf("\nhook after_tool failed: %v\n%s", hookErr, hookOut)
	} else if strings.TrimSpace(hookOut) != "" {
		result += "\nhook after_tool:\n" + hookOut
	}
	return result, err
}
