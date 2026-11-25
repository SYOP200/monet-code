package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet-code/internal/config"
	"github.com/syop200/monet-code/internal/theme"
)

// slashCommand is one entry in the command palette. Built-ins dispatch through
// runSlash; custom commands expand a Markdown prompt from disk.
type slashState struct {
	visible bool
	items   []slashCommand
	sel     int
	offset  int
	prefix  string
}

type slashCommand struct {
	Name     string
	Args     string
	Desc     string
	Aliases  []string
	NeedsArg bool
	Custom   bool
	Prompt   string
}

func builtinCommands() []slashCommand {
	return []slashCommand{
		{Name: "/help", Desc: "keys, commands and the palette", Aliases: []string{"/h", "/?"}},
		{Name: "/clear", Desc: "reset the conversation and context"},
		{Name: "/compact", Desc: "summarize the conversation to save context"},
		{Name: "/resume", Args: "[session]", Desc: "load a saved session", Aliases: []string{"/continue"}},
		{Name: "/sessions", Desc: "list saved sessions"},
		{Name: "/session", Desc: "show this session id and workspace"},
		{Name: "/checkpoint", Args: "[label]", Desc: "snapshot the workspace before risky edits"},
		{Name: "/rewind", Args: "[n]", Desc: "restore the most recent checkpoint", Aliases: []string{"/undo"}},
		{Name: "/memory", Desc: "show the project memory that is loaded"},
		{Name: "/init", Desc: "create AGENTS.md with project conventions"},
		{Name: "/remember", Args: "<note>", Desc: "append a note to project memory", NeedsArg: true},
		{Name: "/model", Args: "[name]", Desc: "switch model or open the model catalog"},
		{Name: "/models", Desc: "open the model catalog"},
		{Name: "/files", Desc: "open the file organizer", Aliases: []string{"/tree"}},
		{Name: "/editor", Args: "[path]", Desc: "open the editor or a file", Aliases: []string{"/edit"}},
		{Name: "/chat", Desc: "back to the conversation"},
		{Name: "/new", Args: "<path>", Desc: "create a file and open it", NeedsArg: true},
		{Name: "/save", Desc: "save the editor buffer"},
		{Name: "/review", Args: "[path]", Desc: "ask the agent to review the working tree"},
		{Name: "/commit", Args: "<message>", Desc: "stage all changes and commit", NeedsArg: true},
		{Name: "/diff", Args: "[path]", Desc: "show the working-tree diff"},
		{Name: "/git", Args: "<status|diff|log|branch>", Desc: "git helpers inside the workspace"},
		{Name: "/worktree", Args: "<name>", Desc: "create a git worktree beside this repo", NeedsArg: true},
		{Name: "/agent", Args: "<task>", Desc: "run a read-only subagent", NeedsArg: true},
		{Name: "/agents", Desc: "list subagents and their status"},
		{Name: "/hooks", Desc: "show configured hooks"},
		{Name: "/skills", Desc: "list custom commands discovered on disk"},
		{Name: "/reload", Desc: "reload skills, memory and the file tree"},
		{Name: "/msg", Args: "<session> <text>", Desc: "message another Monet session", NeedsArg: true},
		{Name: "/inbox", Desc: "read messages from other sessions"},
		{Name: "/worktrees", Desc: "list git worktrees"},
		{Name: "/branch", Args: "<name>", Desc: "create and switch to a branch", NeedsArg: true},
		{Name: "/push", Args: "[branch]", Desc: "push the current branch"},
		{Name: "/pr", Args: "[title]", Desc: "open a GitHub pull request with gh"},
		{Name: "/prs", Desc: "list open pull requests"},
		{Name: "/thinking", Desc: "show the last reasoning trace", Aliases: []string{"/think"}},
		{Name: "/status", Desc: "model, workspace, git and context status"},
		{Name: "/tools", Desc: "list the tools the model can call"},
		{Name: "/palette", Desc: "show the palette"},
		{Name: "/quit", Desc: "leave Monet Code", Aliases: []string{"/exit", "/q"}},
	}
}

// commandDirs returns project and user command directories, in priority order.
func commandDirs(root string) []string {
	dirs := []string{
		filepath.Join(root, ".monet", "commands"),
		filepath.Join(root, ".claude", "commands"),
		filepath.Join(config.Dir(), "commands"),
	}
	seen := map[string]bool{}
	out := dirs[:0]
	for _, d := range dirs {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// reloadCommands rebuilds the palette from built-ins plus Markdown commands on
// disk. Project commands override user commands with the same name.
func (a *App) reloadCommands() {
	commands := builtinCommands()
	byName := map[string]int{}
	for i, c := range commands {
		byName[c.Name] = i
	}
	for _, dir := range commandDirs(a.root) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var files []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
				continue
			}
			files = append(files, e.Name())
		}
		sort.Strings(files)
		for _, name := range files {
			cmd, err := readCommandFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			if i, ok := byName[cmd.Name]; ok {
				if commands[i].Custom {
					commands[i] = cmd
				}
				continue
			}
			byName[cmd.Name] = len(commands)
			commands = append(commands, cmd)
		}
	}
	a.commands = commands
}

func readCommandFile(path string) (slashCommand, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return slashCommand{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = "/" + strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "_", "-"), " ", "-"))
	body := strings.ReplaceAll(string(data), "\r\n", "\n")
	desc := "custom command from " + filepath.Base(path)
	// Optional YAML-ish front matter: only description is understood.
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---"); end >= 0 {
			head := body[4 : 4+end]
			body = strings.TrimLeft(body[4+end+4:], "\n")
			for _, line := range strings.Split(head, "\n") {
				key, val, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				if strings.TrimSpace(strings.ToLower(key)) == "description" {
					desc = strings.TrimSpace(val)
				}
			}
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return slashCommand{}, fmt.Errorf("empty command")
	}
	return slashCommand{
		Name:     name,
		Args:     "[args]",
		Desc:     desc,
		Custom:   true,
		Prompt:   body,
		NeedsArg: strings.Contains(body, "$ARGUMENTS") || strings.Contains(body, "{{args}}"),
	}, nil
}

func (a *App) findCommand(name string) (slashCommand, bool) {
	name = strings.ToLower(name)
	for _, c := range a.commands {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
		for _, alias := range c.Aliases {
			if strings.EqualFold(alias, name) {
				return c, true
			}
		}
	}
	return slashCommand{}, false
}

func (a *App) updateSlashMenu() {
	value := a.chat.input.Value()
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
		a.hideSlashMenu()
		return
	}
	prefix := strings.ToLower(strings.TrimPrefix(value, "/"))
	var items []slashCommand
	for _, c := range a.commands {
		hay := strings.ToLower(strings.TrimPrefix(c.Name, "/") + " " + c.Desc + " " + strings.Join(c.Aliases, " "))
		if prefix == "" || strings.Contains(hay, prefix) || strings.HasPrefix(strings.ToLower(strings.TrimPrefix(c.Name, "/")), prefix) {
			items = append(items, c)
		}
	}
	a.slash.items = items
	a.slash.visible = len(items) > 0
	a.slash.prefix = prefix
	if a.slash.sel >= len(items) {
		a.slash.sel = max(0, len(items)-1)
	}
	a.slash.offset = 0
	a.ensureSlashVisible()
}

func (a *App) hideSlashMenu() {
	a.slash.visible = false
	a.slash.items = nil
	a.slash.sel = 0
	a.slash.offset = 0
}

func (a *App) slashVisibleCount() int {
	return min(len(a.slash.items), 8)
}

func (a *App) ensureSlashVisible() {
	n := a.slashVisibleCount()
	if n == 0 {
		a.slash.offset = 0
		return
	}
	if a.slash.sel < a.slash.offset {
		a.slash.offset = a.slash.sel
	}
	if a.slash.sel >= a.slash.offset+n {
		a.slash.offset = a.slash.sel - n + 1
	}
	a.slash.offset = clamp(a.slash.offset, 0, max(0, len(a.slash.items)-n))
}

func (a *App) slashMenuHeight(h int) int {
	rows := a.slashMenuRows(h)
	if rows == 0 {
		return 0
	}
	return rows + 2
}

// slashMenuRows bounds the popup so the input box and a usable transcript
// remain on screen even in a short terminal.
func (a *App) slashMenuRows(h int) int {
	if !a.slash.visible {
		return 0
	}
	return min(a.slashVisibleCount(), max(0, h-14))
}

func (a *App) slashMenuView(w, limit int) string {
	n := min(limit, a.slashVisibleCount())
	if n == 0 {
		return ""
	}
	inner := max(10, w-4)
	var rows []string
	for i := a.slash.offset; i < len(a.slash.items) && i < a.slash.offset+n; i++ {
		c := a.slash.items[i]
		label := c.Name
		if c.Args != "" {
			label += " " + c.Args
		}
		desc := c.Desc
		if c.Custom {
			desc = "custom · " + desc
		}
		pad := inner - lipgloss.Width(label) - lipgloss.Width(desc) - 3
		if pad < 1 {
			desc = ansi.Truncate(desc, max(4, inner-lipgloss.Width(label)-4), "…")
			pad = max(1, inner-lipgloss.Width(label)-lipgloss.Width(desc)-3)
		}
		plain := " " + label + strings.Repeat(" ", pad) + desc + " "
		if i == a.slash.sel {
			rows = append(rows, theme.SelectedRow.Render(fitPlain(plain, inner)))
		} else {
			rows = append(rows, theme.BlueText.Render(" "+label)+strings.Repeat(" ", pad)+theme.DimText.Render(desc)+" ")
		}
	}
	h := len(rows) + 2
	return frameBox("COMMANDS", "", strings.Join(rows, "\n"), w, h, theme.Teal, theme.BgPanel)
}

func (a *App) handleSlashKey(k string) (tea.Cmd, bool) {
	if !a.slash.visible {
		return nil, false
	}
	switch k {
	case "up", "ctrl+p":
		a.slash.sel = max(0, a.slash.sel-1)
		a.ensureSlashVisible()
		return nil, true
	case "down", "ctrl+n":
		a.slash.sel = min(len(a.slash.items)-1, a.slash.sel+1)
		a.ensureSlashVisible()
		return nil, true
	case "tab":
		return a.completeSlash(), true
	case "enter":
		return a.acceptSlash(), true
	case "esc":
		a.hideSlashMenu()
		return nil, true
	}
	return nil, false
}

func (a *App) completeSlash() tea.Cmd {
	if !a.slash.visible || len(a.slash.items) == 0 {
		return nil
	}
	c := a.slash.items[a.slash.sel]
	a.chat.input.SetValue(c.Name + " ")
	a.chat.input.CursorEnd()
	a.hideSlashMenu()
	return a.chat.input.Focus()
}

func (a *App) acceptSlash() tea.Cmd {
	if !a.slash.visible || len(a.slash.items) == 0 {
		return nil
	}
	c := a.slash.items[a.slash.sel]
	if c.NeedsArg {
		return a.completeSlash()
	}
	a.hideSlashMenu()
	a.chat.input.Reset()
	return a.runSlash(c.Name)
}

// customPrompt expands a Markdown command with the supplied arguments.
func customPrompt(c slashCommand, arg string) string {
	prompt := strings.ReplaceAll(c.Prompt, "$ARGUMENTS", arg)
	prompt = strings.ReplaceAll(prompt, "{{args}}", arg)
	if arg != "" && !strings.Contains(c.Prompt, "$ARGUMENTS") && !strings.Contains(c.Prompt, "{{args}}") {
		prompt += "\n\nArguments: " + arg
	}
	return prompt
}
