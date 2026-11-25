package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/syop200/monet-code/internal/config"
	"github.com/syop200/monet-code/internal/files"
	"github.com/syop200/monet-code/internal/ollama"
	"github.com/syop200/monet-code/internal/session"
	"github.com/syop200/monet-code/internal/theme"
)

func testApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("MONET_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(root, "qwen3-coder:30b", "http://127.0.0.1:1")
}

func TestBlackCanvas(t *testing.T) {
	if theme.Bg != "#000000" || theme.BgPanel != "#000000" || theme.BgCode != "#000000" {
		t.Fatalf("canvas is not black: bg=%q panel=%q code=%q", theme.Bg, theme.BgPanel, theme.BgCode)
	}
	if config.Dir() == "" {
		t.Fatal("config dir is empty")
	}
}

func TestBuiltinCommandsCoverWorkflow(t *testing.T) {
	app := testApp(t)
	want := []string{
		"/help", "/clear", "/compact", "/resume", "/sessions", "/checkpoint",
		"/rewind", "/memory", "/init", "/remember", "/model", "/files",
		"/editor", "/chat", "/review", "/commit", "/diff", "/git",
		"/worktree", "/agent", "/agents", "/hooks", "/skills", "/reload",
		"/msg", "/inbox", "/thinking", "/status", "/tools", "/quit",
		"/worktrees", "/branch", "/push", "/pr", "/prs",
	}
	for _, name := range want {
		if _, ok := app.findCommand(name); !ok {
			t.Errorf("missing command %s", name)
		}
	}
	for _, alias := range []string{"/q", "/exit", "/h", "/think", "/undo", "/continue"} {
		if _, ok := app.findCommand(alias); !ok {
			t.Errorf("missing alias %s", alias)
		}
	}
	seen := map[string]bool{}
	for _, c := range app.commands {
		if seen[c.Name] {
			t.Errorf("duplicate command %s", c.Name)
		}
		seen[c.Name] = true
	}
}

func TestGitRefValidation(t *testing.T) {
	for _, name := range []string{"feature/login", "fix-123", "release_2.0"} {
		if !validGitRef(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range []string{"", "-x", "a b", "a;b", "$(x)", "../escape", "a..b", "a|b"} {
		if validGitRef(name) {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestCustomCommandsLoadAndExpand(t *testing.T) {
	app := testApp(t)
	dir := filepath.Join(app.root, ".monet", "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: review a pull request\n---\nReview $ARGUMENTS carefully.\n"
	if err := os.WriteFile(filepath.Join(dir, "review-pr.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	app.reloadCommands()
	cmd, ok := app.findCommand("/review-pr")
	if !ok {
		t.Fatal("custom command was not discovered")
	}
	if !cmd.Custom || cmd.Desc != "review a pull request" || !cmd.NeedsArg {
		t.Fatalf("unexpected custom command: %+v", cmd)
	}
	if got := customPrompt(cmd, "42"); got != "Review 42 carefully." {
		t.Fatalf("expanded prompt = %q", got)
	}
	if got := customPrompt(cmd, ""); got != "Review  carefully." {
		t.Fatalf("empty expansion = %q", got)
	}
}

func TestSlashMenuFiltersAndCompletes(t *testing.T) {
	app := testApp(t)
	app.chat.input.SetValue("/co")
	app.updateSlashMenu()
	if !app.slash.visible {
		t.Fatal("slash menu should be visible")
	}
	var names []string
	for _, c := range app.slash.items {
		names = append(names, c.Name)
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"/compact", "/commit"} {
		if !strings.Contains(joined, want) {
			t.Errorf("filter /co missing %s (got %s)", want, joined)
		}
	}
	for _, c := range app.slash.items {
		hay := strings.ToLower(c.Name + " " + c.Desc + " " + strings.Join(c.Aliases, " "))
		if !strings.Contains(hay, "co") {
			t.Errorf("filter /co included unrelated %s", c.Name)
		}
	}

	app.chat.input.SetValue("/")
	app.updateSlashMenu()
	if len(app.slash.items) != len(app.commands) {
		t.Fatalf("bare / showed %d of %d commands", len(app.slash.items), len(app.commands))
	}

	app.chat.input.SetValue("/hel")
	app.updateSlashMenu()
	app.slash.sel = 0
	if cmd := app.acceptSlash(); cmd != nil {
		_ = cmd()
	}
	if app.modal.kind != modalHelp {
		t.Fatalf("accepting /help opened modal kind %d", app.modal.kind)
	}
}

func TestSlashMenuNeedsArgumentsCompletesInstead(t *testing.T) {
	app := testApp(t)
	app.chat.input.SetValue("/commit fix the tests")
	app.updateSlashMenu()
	if app.slash.visible {
		t.Fatal("menu should hide once an argument is typed")
	}
	app.chat.input.SetValue("/remember")
	app.updateSlashMenu()
	app.slash.sel = 0
	if cmd := app.acceptSlash(); cmd != nil {
		_ = cmd()
	}
	if got := app.chat.input.Value(); !strings.HasPrefix(got, "/remember ") {
		t.Fatalf("expected completion, got %q", got)
	}
}

func TestTabClicksSwitchViews(t *testing.T) {
	app := testApp(t)
	app.Snapshot(100, 30)
	var editorHit tabHit
	found := false
	for _, h := range app.tabHits {
		if h.tab == tabEditor {
			editorHit, found = h, true
		}
	}
	if !found {
		t.Fatal("editor tab hitbox was not registered")
	}
	app.handleMouse(tea.MouseMsg{
		X:      editorHit.x0 + 1,
		Y:      1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if app.active != tabEditor {
		t.Fatalf("clicking the editor tab left active = %d", app.active)
	}
}

func TestSlashMenuClickRunsCommand(t *testing.T) {
	app := testApp(t)
	app.chat.input.SetValue("/")
	app.updateSlashMenu()
	app.Snapshot(120, 34)
	top := 2 + app.chat.menuTop + 1
	app.handleMouse(tea.MouseMsg{
		X:      app.sidebarW + 4,
		Y:      top,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if app.modal.kind != modalHelp {
		t.Fatalf("clicking the first command should open help, got modal %d", app.modal.kind)
	}
}

func TestCtrlCDoesNotQuit(t *testing.T) {
	app := testApp(t)
	cmd := app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("ctrl+c quit the app")
		}
	}
	if app.modal.kind != modalMessage {
		t.Fatalf("ctrl+c should open the exit hint, got modal %d", app.modal.kind)
	}
}

func TestQuitRequiresCtrlDOrVim(t *testing.T) {
	app := testApp(t)
	cmd := app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("ctrl+q should notify")
	}
	if _, ok := cmd().(tea.QuitMsg); ok {
		t.Fatal("ctrl+q quit the app")
	}
	cmd = app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("ctrl+d should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+d did not quit: %T", cmd())
	}
	if cmd := app.runVimCommand(":q"); cmd == nil {
		t.Fatal(":q should quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf(":q did not quit: %T", cmd())
	}
}

func TestThinkingModalAndActivityLabels(t *testing.T) {
	app := testApp(t)
	cmd := app.openThinking("step 1\nstep 2")
	_ = cmd
	if app.modal.kind != modalThinking || !strings.Contains(app.modal.text, "step 1") {
		t.Fatalf("thinking modal = %+v", app.modal)
	}
	out := app.Snapshot(80, 24)
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("thinking modal rendered %d rows", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 80 {
			t.Errorf("thinking row %d has %d columns", i, got)
		}
	}
	for name, want := range map[string]string{
		"read_file":        "reading…",
		"write_file":       "editing file…",
		"create_directory": "designing…",
		"run_command":      "running a command…",
		"git_commit":       "committing…",
	} {
		if got := toolActivity(name); got != want {
			t.Errorf("toolActivity(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResumeRebuildsTranscript(t *testing.T) {
	app := testApp(t)
	history := []ollama.Message{
		{Role: ollama.RoleUser, Content: "please add tests"},
		{Role: ollama.RoleAssistant, Content: "Added them.", Thinking: "reasoning trace"},
	}
	if _, err := session.Save("20260101-120000", app.root, "qwen3-coder:30b", history); err != nil {
		t.Fatal(err)
	}
	app.resumeSession("20260101-120000")
	if app.sessionID != "20260101-120000" {
		t.Fatalf("session id = %q", app.sessionID)
	}
	if len(app.chat.messages) < 2 {
		t.Fatalf("transcript has %d messages, want at least 2", len(app.chat.messages))
	}
	if app.chat.messages[0].role != "user" || app.chat.messages[1].thinking != "reasoning trace" {
		t.Fatalf("transcript was not restored: %+v", app.chat.messages)
	}
	if app.chat.lastThinking != "reasoning trace" {
		t.Fatalf("last thinking = %q", app.chat.lastThinking)
	}
}

func TestModelAndPreviewRowHitTesting(t *testing.T) {
	app := testApp(t)
	app.models.width, app.models.height = 100, 30
	app.models.models = []ollama.Model{{Name: "a:1"}, {Name: "b:1"}}
	app.models.offset[0], app.models.offset[1] = 0, 0
	if section, row, ok := app.models.rowAt(2); !ok || section != 0 || row != 0 {
		t.Fatalf("first installed row = %d,%d,%v", section, row, ok)
	}
	if section, row, ok := app.models.rowAt(3); !ok || section != 0 || row != 1 {
		t.Fatalf("second installed row = %d,%d,%v", section, row, ok)
	}
	if section, _, ok := app.models.rowAt(16); !ok || section != 1 {
		t.Fatalf("first catalog row was not selectable: section=%d ok=%v", section, ok)
	}
	if _, _, ok := app.models.rowAt(1); ok {
		t.Fatal("the installed frame border should not select a row")
	}

	app.org.width, app.org.height = 100, 30
	app.org.preview = true
	app.org.report.Groups = nil
	app.org.moves = []files.Move{{RelFrom: "a.txt", RelTo: "Documents/a.txt"}, {RelFrom: "b.png", RelTo: "Images/b.png"}}
	if row, ok := app.org.rowAt(6); !ok || row != 0 {
		t.Fatalf("first preview row = %d,%v", row, ok)
	}
	if row, ok := app.org.rowAt(7); !ok || row != 1 {
		t.Fatalf("second preview row = %d,%v", row, ok)
	}
	if _, ok := app.org.rowAt(5); ok {
		t.Fatal("the preview frame border should not select a row")
	}
}
