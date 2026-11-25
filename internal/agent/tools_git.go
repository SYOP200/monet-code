package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func (e *Executor) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = e.Root
	var buf bytes.Buffer
	buf.Grow(4096)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := strings.TrimRight(buf.String(), "\n")
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (e *Executor) gitStatus(ctx context.Context) (string, error) {
	out, err := e.git(ctx, "status", "--short", "--branch")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "working tree clean", nil
	}
	return out, nil
}

func (e *Executor) gitDiff(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &a)
	gitArgs := []string{"diff", "--no-color"}
	if a.Path != "" {
		if _, err := e.mutablePath(a.Path); err != nil {
			return "", err
		}
		gitArgs = append(gitArgs, "--", a.Path)
	}
	out, err := e.git(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		return "no unstaged changes", nil
	}
	if len(out) > 24000 {
		out = out[:24000] + "\n… diff truncated"
	}
	return out, nil
}

func (e *Executor) gitLog(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Max int `json:"max"`
	}
	_ = json.Unmarshal(args, &a)
	if a.Max <= 0 || a.Max > 50 {
		a.Max = 10
	}
	out, err := e.git(ctx, "log", "--oneline", "--decorate", fmt.Sprintf("-n%d", a.Max))
	if err != nil {
		return "", err
	}
	if out == "" {
		return "no commits yet", nil
	}
	return out, nil
}

func (e *Executor) gitCommit(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	msg := strings.TrimSpace(a.Message)
	if msg == "" {
		return "", fmt.Errorf("commit message is required")
	}
	if out, err := e.Hooks.Run(ctx, e.Root, "before_commit", nil); err != nil {
		return "before_commit hook failed: " + err.Error() + "\n" + out, err
	}
	if out, err := e.git(ctx, "add", "-A"); err != nil {
		return out, err
	}
	// Never commit Monet's checkpoints or session state.
	_, _ = e.git(ctx, "reset", "-q", "--", ".monet")
	out, err := e.git(ctx, "commit", "-m", msg)
	if err != nil {
		return out, err
	}
	return out, nil
}
