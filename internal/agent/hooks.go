package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Hooks are user-defined shell commands run around agent actions. They live in
// .monet/hooks.json:
//
//	{
//	  "after_edit": "gofmt -w \"$MONET_FILE\"",
//	  "before_commit": "go test ./...",
//	  "after_tool": "echo $MONET_TOOL >> .monet/tool.log"
//	}
type Hooks struct {
	AfterEdit    string `json:"after_edit,omitempty"`
	BeforeCommit string `json:"before_commit,omitempty"`
	AfterTool    string `json:"after_tool,omitempty"`
}

// LoadHooks reads project hooks, returning an empty set when none exist.
func LoadHooks(root string) Hooks {
	var h Hooks
	data, err := os.ReadFile(filepath.Join(root, ".monet", "hooks.json"))
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, &h)
	return h
}

// Run executes a hook with Monet environment variables and a short timeout.
func (h Hooks) Run(ctx context.Context, root, name string, extra map[string]string) (string, error) {
	var command string
	switch name {
	case "after_edit":
		command = h.AfterEdit
	case "before_commit":
		command = h.BeforeCommit
	case "after_tool":
		command = h.AfterTool
	}
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	for k, v := range extra {
		command = strings.ReplaceAll(command, "$"+k, shellQuote(v))
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := RunShell(runCtx, root, command, "", 30*time.Second)
	if err != nil {
		return res.Output, err
	}
	return res.Output, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
