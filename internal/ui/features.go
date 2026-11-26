package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/syop200/monet-code/internal/agent"
	"github.com/syop200/monet-code/internal/files"
	"github.com/syop200/monet-code/internal/ollama"
	"github.com/syop200/monet-code/internal/session"
	"github.com/syop200/monet-code/internal/update"
)

type compactDoneMsg struct {
	summary string
	err     error
}

type shellDoneMsg struct {
	title  string
	output string
	err    error
}

type updateDoneMsg struct {
	result update.Result
	check  bool
	err    error
}

type subagentTask struct {
	ID      string
	Task    string
	Status  string
	Result  string
	Started time.Time
}

type subagentDoneMsg struct {
	id     string
	task   string
	result string
	err    error
}

func (a *App) refreshSystemPrompt() {
	prompt := agent.BuildSystemPrompt(a.root)
	if strings.TrimSpace(a.cfg.SystemPrompt) != "" {
		prompt += "\nAdditional user instructions:\n" + a.cfg.SystemPrompt + "\n"
	}
	a.runner.SetSystemPrompt(prompt)
}

func (a *App) saveSessionCmd() tea.Cmd {
	id := a.sessionID
	root := a.root
	model := a.chat.model
	history := a.runner.History()
	if id == "" {
		id = session.NewID()
		a.sessionID = id
	}
	return func() tea.Msg {
		_, _ = session.Save(id, root, model, history)
		return nil
	}
}

func (a *App) compactCmd() tea.Cmd {
	if a.compacting {
		return a.notify("compaction is already running", false)
	}
	a.compacting = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		summary, err := a.runner.Compact(ctx)
		return compactDoneMsg{summary: summary, err: err}
	}
}

func (a *App) runShell(title, command string) tea.Cmd {
	root := a.root
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		res, err := agent.RunShell(ctx, root, command, "", 40*time.Second)
		return shellDoneMsg{title: title, output: res.Output, err: err}
	}
}

// checkUpdate asks GitHub for the latest tag without touching the install.
func (a *App) checkUpdate() tea.Cmd {
	return a.startUpdate("", true)
}

// runUpdate replaces the installed TUI and CLI with a verified release.
func (a *App) runUpdate(arg string) tea.Cmd {
	return a.startUpdate(strings.TrimSpace(arg), false)
}

func (a *App) startUpdate(arg string, check bool) tea.Cmd {
	if a.updating {
		return a.notify("an update is already running", false)
	}
	a.updating = true
	opts := update.Options{CurrentVersion: Version, Check: check}
	switch {
	case arg == "force":
		opts.Force = true
	case strings.HasPrefix(arg, "v"):
		opts.Tag = arg
	}
	return tea.Batch(a.kick(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		result, err := update.Run(ctx, opts)
		return updateDoneMsg{result: result, check: check, err: err}
	})
}

func (a *App) handleUpdateDone(msg updateDoneMsg) tea.Cmd {
	a.updating = false
	if msg.err != nil {
		a.chat.addError("update: " + msg.err.Error())
		return a.notify("update failed", true)
	}
	if msg.check {
		if update.IsNewer(msg.result.Latest, msg.result.Current) {
			a.chat.addNotice(fmt.Sprintf("update available · %s → %s · run /update to install",
				msg.result.Current, msg.result.Latest))
			return a.notify("update available · "+msg.result.Latest, false)
		}
		return a.notify("up to date · "+msg.result.Current, false)
	}
	if msg.result.Updated {
		a.chat.addNotice(fmt.Sprintf("updated %s → %s · restart Monet Code to run the new build",
			msg.result.Current, msg.result.Latest))
		return a.notify("updated to "+msg.result.Latest+" · restart to apply", false)
	}
	return a.notify("already on the latest release · "+msg.result.Current, false)
}

func (a *App) checkpoint(label string) tea.Cmd {
	if a.editor.path == "" {
		return a.notify("open a file first — /checkpoint snapshots the editor buffer", true)
	}
	rel := files.Rel(a.root, a.editor.path)
	cp, err := files.Snapshot(a.root, rel, label)
	if err != nil {
		return a.notify("checkpoint failed: "+err.Error(), true)
	}
	return a.notify("checkpoint "+cp.ID+" · "+cp.Rel, false)
}

func (a *App) rewind(arg string) tea.Cmd {
	list, err := files.ListCheckpoints(a.root)
	if err != nil {
		return a.notify(err.Error(), true)
	}
	if len(list) == 0 {
		return a.notify("no checkpoints yet", true)
	}
	n := 1
	if strings.TrimSpace(arg) != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil && v > 0 {
			n = v
		}
	}
	if n > len(list) {
		return a.notify(fmt.Sprintf("only %d checkpoints exist", len(list)), true)
	}
	cp := list[n-1]
	restored, err := files.Rewind(a.root, cp.ID)
	if err != nil {
		return a.notify("rewind failed: "+err.Error(), true)
	}
	target, _ := files.SafeJoin(a.root, restored.Rel)
	if a.editor.path == target {
		_ = a.editor.open(target)
	}
	a.sidebar.reload()
	return tea.Batch(a.scanFiles(), a.notify("restored "+restored.Rel+" from "+restored.ID, false))
}

func (a *App) initMemory() tea.Cmd {
	path := filepath.Join(a.root, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return a.notify("AGENTS.md already exists", false)
	}
	template := `# AGENTS.md

Monet Code loads this file at the start of every session. Keep it short and
specific; it is project memory, not documentation.

## Conventions

- Language and framework:
- Formatting and lint commands:
- Test command:
- Branch and commit style:

## Architecture

- Entry points:
- Important packages:
- Things to avoid:

## Workflow

- Read this file before editing.
- Run tests before committing.
- Ask before changing public APIs.
`
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return a.notify(err.Error(), true)
	}
	a.refreshSystemPrompt()
	a.sidebar.reload()
	return a.notify("created AGENTS.md — edit it to teach Monet your conventions", false)
}

func (a *App) remember(note string) tea.Cmd {
	note = strings.TrimSpace(note)
	if note == "" {
		return a.notify("usage: /remember <note>", true)
	}
	path := filepath.Join(a.root, ".monet", "memory.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return a.notify(err.Error(), true)
	}
	line := "- " + time.Now().Format("2006-01-02") + ": " + note + "\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return a.notify(err.Error(), true)
	}
	_, err = f.WriteString(line)
	_ = f.Close()
	if err != nil {
		return a.notify(err.Error(), true)
	}
	a.refreshSystemPrompt()
	return a.notify("remembered for this project", false)
}

func (a *App) showMemory() tea.Cmd {
	loaded := agent.MemoryFiles(a.root)
	if len(loaded) == 0 {
		return a.notify("no project memory yet — run /init to create AGENTS.md", false)
	}
	for _, rel := range loaded {
		path := filepath.Join(a.root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		a.chat.addNotice(fmt.Sprintf("memory  %s  ·  %s", rel, files.HumanBytes(info.Size())))
	}
	return nil
}

func (a *App) showSkills() tea.Cmd {
	var lines []string
	for _, c := range a.commands {
		if !c.Custom {
			continue
		}
		lines = append(lines, c.Name+"  ·  "+c.Desc)
	}
	if len(lines) == 0 {
		return a.notify("no custom skills — add .monet/commands/<name>.md or ~/.config/monet/commands/<name>.md", false)
	}
	for _, line := range lines {
		a.chat.addNotice("skill  " + line)
	}
	return nil
}

func (a *App) reloadSkills() tea.Cmd {
	a.reloadCommands()
	a.refreshSystemPrompt()
	a.runner.ReloadHooks()
	a.sidebar.reload()
	custom := 0
	for _, c := range a.commands {
		if c.Custom {
			custom++
		}
	}
	return a.notify(fmt.Sprintf("reloaded %s and %s", plural(custom, "skill", "skills"), plural(len(a.commands), "command", "commands")), false)
}

// loadTranscript rebuilds the visible chat from a saved session history.
func (a *App) loadTranscript(history []ollama.Message) {
	a.chat.clear()
	for _, m := range history {
		switch m.Role {
		case ollama.RoleUser:
			a.chat.addUser(m.Content)
		case ollama.RoleAssistant:
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Thinking) == "" {
				continue
			}
			a.chat.add(&chatMessage{
				role:     "assistant",
				content:  m.Content,
				thinking: m.Thinking,
				complete: true,
			})
			if strings.TrimSpace(m.Thinking) != "" {
				a.chat.lastThinking = m.Thinking
			}
		case ollama.RoleTool:
			a.chat.add(&chatMessage{
				role:       "tool",
				toolName:   m.ToolName,
				toolResult: m.Content,
				complete:   true,
			})
		}
	}
}

func (a *App) showSessions() tea.Cmd {
	list := session.List()
	if len(list) == 0 {
		return a.notify("no saved sessions yet", false)
	}
	for i, s := range list {
		if i >= 8 {
			break
		}
		a.chat.addNotice(fmt.Sprintf("session  %s  ·  %s  ·  %d messages  ·  %s",
			s.ID, filepath.Base(s.Root), len(s.History), files.HumanAge(s.Updated)))
	}
	return nil
}

func (a *App) resumeSession(arg string) tea.Cmd {
	var (
		s   session.Session
		err error
	)
	if strings.TrimSpace(arg) == "" {
		s, err = session.Latest()
	} else {
		s, err = session.Load(strings.TrimSpace(arg))
	}
	if err != nil {
		return a.notify("resume failed: "+err.Error(), true)
	}
	a.runner.SetHistory(s.History)
	a.sessionID = s.ID
	a.loadTranscript(s.History)
	if s.Model != "" {
		a.selectModel(s.Model)
	}
	a.chat.addNotice(fmt.Sprintf("resumed %s · %d messages · %s", s.ID, len(s.History), filepath.Base(s.Root)))
	a.deliverInbox()
	return nil
}

func (a *App) sendSessionMessage(arg string) tea.Cmd {
	target, text, ok := strings.Cut(strings.TrimSpace(arg), " ")
	if !ok || strings.TrimSpace(text) == "" {
		return a.notify("usage: /msg <session-id|latest> <text>", true)
	}
	target = strings.TrimSpace(target)
	if strings.EqualFold(target, "latest") {
		target = ""
		for _, s := range session.List() {
			if s.ID != a.sessionID {
				target = s.ID
				break
			}
		}
		if target == "" {
			return a.notify("no other session to message yet", true)
		}
	}
	msg, err := session.SendMessage(a.sessionID, target, text)
	if err != nil {
		return a.notify("message failed: "+err.Error(), true)
	}
	return a.notify("messaged "+msg.To+" · it appears in their /inbox", false)
}

// deliverInbox folds pending session messages into the transcript.
func (a *App) deliverInbox() {
	msgs, err := session.Inbox(a.sessionID, true)
	if err != nil {
		return
	}
	for _, m := range msgs {
		a.chat.addNotice(fmt.Sprintf("message from %s · %s: %s", m.From, files.HumanAge(m.Time), m.Text))
	}
}

func (a *App) showInbox() tea.Cmd {
	msgs, err := session.Inbox(a.sessionID, true)
	if err != nil {
		return a.notify("inbox failed: "+err.Error(), true)
	}
	if len(msgs) == 0 {
		return a.notify("inbox is empty · your session is "+a.sessionID, false)
	}
	for _, m := range msgs {
		a.chat.addNotice(fmt.Sprintf("message from %s · %s: %s", m.From, files.HumanAge(m.Time), m.Text))
	}
	return nil
}

func (a *App) showHooks() tea.Cmd {
	h := agent.LoadHooks(a.root)
	var lines []string
	if h.AfterEdit != "" {
		lines = append(lines, "after_edit     "+h.AfterEdit)
	}
	if h.BeforeCommit != "" {
		lines = append(lines, "before_commit  "+h.BeforeCommit)
	}
	if h.AfterTool != "" {
		lines = append(lines, "after_tool     "+h.AfterTool)
	}
	if len(lines) == 0 {
		return a.notify("no hooks — add .monet/hooks.json with after_edit, before_commit or after_tool", false)
	}
	for _, line := range lines {
		a.chat.addNotice("hook  " + line)
	}
	return nil
}

func (a *App) gitCommit(message string) tea.Cmd {
	message = strings.TrimSpace(message)
	if message == "" {
		return a.notify("commit message is required", true)
	}
	cmd := "git add -A && (git reset -q -- .monet 2>/dev/null || true) && git commit -m " + shellQuote(message)
	return a.runShell("GIT COMMIT", cmd)
}

func (a *App) listWorktrees() tea.Cmd {
	return a.runShell("GIT WORKTREES", "git worktree list")
}

func (a *App) createWorktree(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, "-") {
		return a.notify("worktree name must be a simple branch name", true)
	}
	base := filepath.Base(a.root)
	target := filepath.Join(filepath.Dir(a.root), base+"-"+name)
	cmd := fmt.Sprintf("git worktree add %s -b %s", shellQuote(target), shellQuote(name))
	return a.runShell("GIT WORKTREE", cmd)
}

func (a *App) runSubagent(task string) tea.Cmd {
	task = strings.TrimSpace(task)
	if task == "" {
		return a.notify("usage: /agent <task>", true)
	}
	if a.chat.model == "" {
		return a.notify("select a model before spawning a subagent", true)
	}
	running := 0
	for _, s := range a.subagents {
		if s.Status == "running" {
			running++
		}
	}
	if running >= 4 {
		return a.notify("four subagents are already running", true)
	}
	a.subagentSeq++
	id := fmt.Sprintf("agent-%d", a.subagentSeq)
	a.subagents = append(a.subagents, subagentTask{ID: id, Task: task, Status: "running", Started: time.Now()})
	client, root, model := a.client, a.root, a.chat.model
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		runner := agent.NewSubagentRunner(client, root, model)
		if _, err := runner.Send(task); err != nil {
			return subagentDoneMsg{id: id, task: task, err: err}
		}
		var out strings.Builder
		for {
			select {
			case ev := <-runner.Events():
				switch ev.Type {
				case agent.EventDelta:
					out.WriteString(ev.Text)
				case agent.EventError:
					return subagentDoneMsg{id: id, task: task, err: fmt.Errorf("%s", ev.Text)}
				case agent.EventDone:
					return subagentDoneMsg{id: id, task: task, result: strings.TrimSpace(out.String())}
				}
			case <-ctx.Done():
				runner.Cancel()
				return subagentDoneMsg{id: id, task: task, err: fmt.Errorf("subagent timed out")}
			}
		}
	}
}

func (a *App) handleSubagentDone(msg subagentDoneMsg) tea.Cmd {
	for i := range a.subagents {
		if a.subagents[i].ID != msg.id {
			continue
		}
		a.subagents[i].Result = msg.result
		if msg.err != nil {
			a.subagents[i].Status = "failed"
			a.subagents[i].Result = msg.err.Error()
			a.chat.addError("subagent " + msg.id + ": " + msg.err.Error())
		} else {
			a.subagents[i].Status = "done"
			content := msg.result
			if content == "" {
				content = "_no findings_"
			}
			a.chat.add(&chatMessage{
				role:     "assistant",
				content:  "**Subagent " + msg.id + "** · " + msg.task + "\n\n" + content,
				complete: true,
			})
		}
	}
	return nil
}

func (a *App) handleShellDone(msg shellDoneMsg) tea.Cmd {
	if msg.err != nil && strings.TrimSpace(msg.output) == "" {
		return a.notify(msg.title+" failed: "+msg.err.Error(), true)
	}
	output := msg.output
	if strings.TrimSpace(output) == "" {
		output = "(no output)"
	}
	a.chat.add(&chatMessage{
		role:     "assistant",
		content:  "**" + msg.title + "**\n\n```\n" + output + "\n```",
		complete: true,
	})
	if msg.err != nil {
		a.chat.addError(msg.title + ": " + msg.err.Error())
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// validGitRef keeps shell and option injection out of git/gh arguments.
func validGitRef(s string) bool {
	if s == "" || len(s) > 200 || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '/':
		default:
			return false
		}
	}
	return !strings.Contains(s, "..")
}

func (a *App) createBranch(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if !validGitRef(name) {
		return a.notify("usage: /branch <name> (letters, digits, . _ / -)", true)
	}
	return a.runShell("GIT BRANCH", "git switch -c "+shellQuote(name))
}

func (a *App) pushBranch(arg string) tea.Cmd {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return a.runShell("GIT PUSH", "git push")
	}
	if !validGitRef(arg) {
		return a.notify("branch name has invalid characters", true)
	}
	return a.runShell("GIT PUSH", "git push -u origin "+shellQuote(arg))
}

func (a *App) createPR(title string) tea.Cmd {
	title = strings.TrimSpace(title)
	cmd := "gh pr create --fill"
	if title != "" {
		cmd += " --title " + shellQuote(title)
	}
	return a.runShell("GITHUB PR", cmd)
}

func (a *App) listPRs() tea.Cmd {
	return a.runShell("GITHUB PRS", "gh pr list --limit 10")
}
