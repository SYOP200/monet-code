package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/syop200/monet-code/internal/files"
)

// ShellResult is the captured result of a shell command.
type ShellResult struct {
	Output   string
	ExitCode int
}

// DangerousCommand rejects destructive shell patterns before they reach bash.
// The agent can still work in subdirectories, run builds and use git, but it
// cannot recursively delete a tree or format a disk.
func DangerousCommand(command string) string {
	lower := strings.ToLower(command)
	fields := strings.Fields(lower)
	for i, f := range fields {
		base := filepath.Base(f)
		switch base {
		case "sudo", "doas", "mkfs", "mkfs.ext4", "shutdown", "reboot", "halt", "poweroff":
			return "privileged or system-level commands are blocked: " + base
		case "rm":
			recursive := false
			for _, arg := range fields[i+1:] {
				if strings.ContainsAny(arg, ";|&") {
					break
				}
				if strings.HasPrefix(arg, "--") {
					if arg == "--recursive" {
						recursive = true
					}
					if arg == "--force" && recursive {
						return "recursive forced deletion (rm -rf) is blocked"
					}
					continue
				}
				if strings.HasPrefix(arg, "-") && len(arg) > 1 {
					if strings.ContainsAny(arg[1:], "rR") {
						recursive = true
					}
					if strings.Contains(arg[1:], "f") && recursive {
						return "recursive forced deletion (rm -rf) is blocked"
					}
				}
			}
			if recursive {
				return "recursive deletion (rm -r) is blocked; delete files one at a time"
			}
		case "dd":
			return "dd is blocked because it can overwrite disks"
		case "killall", "pkill":
			return "broad process killing is blocked"
		case "chmod", "chown":
			if i+1 < len(fields) && (strings.HasPrefix(fields[i+1], "-r") || fields[i+1] == "--recursive") {
				return "recursive permission changes are blocked"
			}
		}
	}
	for _, bad := range []string{
		"git clean", "git reset --hard", "git push --force", "git push -f",
		"find / -delete", "-delete", "> /dev/sd", ":(){", "fork bomb",
		"mv /*", "chmod -r 777 /", "chown -r /",
	} {
		if strings.Contains(lower, bad) {
			return "destructive pattern blocked: " + bad
		}
	}
	if strings.Contains(lower, "rm") && strings.Contains(lower, "--no-preserve-root") {
		return "destructive rm flags are blocked"
	}
	return ""
}

// RunShell executes a command inside the workspace with a hard timeout. It is
// used by the run_command tool, git helpers, hooks and the TUI.
func RunShell(ctx context.Context, root, command, cwd string, timeout time.Duration) (ShellResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return ShellResult{}, fmt.Errorf("command is required")
	}
	if reason := DangerousCommand(command); reason != "" {
		return ShellResult{}, fmt.Errorf("refused: %s", reason)
	}
	dir := root
	if strings.TrimSpace(cwd) != "" {
		var err error
		dir, err = files.SafeJoin(root, cwd)
		if err != nil {
			return ShellResult{}, err
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return ShellResult{}, fmt.Errorf("working directory %q does not exist", cwd)
		}
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MONET_ROOT="+root, "MONET_CWD="+dir)
	var buf limitedBuffer
	buf.limit = 256 << 10
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	res := ShellResult{Output: strings.TrimRight(buf.String(), "\n")}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		if res.Output == "" {
			return res, err
		}
		return res, fmt.Errorf("exit %d: %w", res.ExitCode, err)
	}
	return res, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len() >= b.limit {
		return len(p), nil
	}
	room := b.limit - b.Len()
	if len(p) > room {
		b.Buffer.Write(p[:room])
		b.Buffer.WriteString("\n… output truncated")
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
