package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/syop200/monet/internal/agent"
	"github.com/syop200/monet/internal/catalog"
	"github.com/syop200/monet/internal/config"
	"github.com/syop200/monet/internal/files"
	"github.com/syop200/monet/internal/ollama"
	"github.com/syop200/monet/internal/theme"
)

type tab int

const (
	tabChat tab = iota
	tabEditor
	tabModels
	tabFiles
)

var tabNames = []string{"Chat", "Editor", "Models", "Files"}

// Version is the Monet Code release string.
const Version = "0.1.0"

// App is the root bubbletea model.
type App struct {
	root   string
	client *ollama.Client
	cfg    config.Config
	runner *agent.Runner

	width, height   int
	sidebarW, mainW int
	active          tab
	sidebarFocused  bool

	sidebar *sidebar
	chat    *chatPane
	editor  *editorPane
	models  *modelsPane
	org     *filesPane
	modal   modalState

	ollamaVersion string
	ollamaErr     string
	ollamaOK      bool
	ollamaWarned  bool
	loadingModels bool

	toast    string
	toastErr bool
	toastSeq int

	frame     int
	animating bool

	pullCh chan pullProgressMsg
}

type (
	modelsLoadedMsg struct {
		models  []ollama.Model
		version string
		err     error
	}
	filesScanMsg struct {
		report files.Report
		err    error
	}
	filesAppliedMsg struct {
		moved int
		errs  []error
	}
	pullProgressMsg struct {
		status    string
		completed int64
		total     int64
		model     string
		done      bool
		err       error
	}
	modelDeletedMsg struct {
		name string
		err  error
	}
	chatEventMsg struct {
		ev agent.Event
	}
	animMsg         struct{}
	toastExpiredMsg struct{ seq int }
)

// New constructs the TUI.
func New(root, model, ollamaURL string) *App {
	cfg := config.Load()
	if ollamaURL == "" {
		ollamaURL = cfg.OllamaBaseURL()
	}
	if model == "" {
		model = cfg.Model
	}
	client := ollama.New(ollamaURL)
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}
	a := &App{
		root:          root,
		client:        client,
		cfg:           cfg,
		active:        tabChat,
		sidebar:       newSidebar(root),
		chat:          newChatPane(80),
		editor:        newEditorPane(cfg.EditorTabSize),
		models:        newModelsPane(),
		org:           newFilesPane(root),
		width:         100,
		height:        32,
		loadingModels: true,
	}
	a.runner = agent.NewRunner(client, root, model)
	a.models.activeModel = model
	a.chat.setModel(model)
	a.welcome()
	a.layout()
	return a
}

func (a *App) welcome() {
	a.chat.addNotice("monet code · local-first coding agent")
	a.chat.addNotice("workspace  " + tildePath(a.root))
	a.chat.addNotice("F2‑5 views  ·  F1 help  ·  / commands  ·  ask anything to begin")
	if a.chat.model == "" {
		a.chat.addNotice("no model selected yet — open Models and pick or pull an Ollama model")
	}
}

// Init starts background discovery and the persistent event reader.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.chat.input.Focus(),
		a.waitChat(),
		a.refreshModels(),
		a.scanFiles(),
	}
	if c := a.kick(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func (a *App) layout() {
	if a.width <= 0 || a.height <= 0 {
		return
	}
	bodyH := max(6, a.height-3)
	a.sidebarW = 0
	if a.width >= 74 {
		a.sidebarW = clamp(a.width/4, 24, 32)
	}
	a.mainW = a.width - a.sidebarW
	if a.sidebarW > 0 {
		a.mainW--
	}
	a.sidebar.resize(a.sidebarW, bodyH)
	a.chat.resize(a.mainW, bodyH)
	a.editor.resize(a.mainW, bodyH, a.active == tabEditor && !a.sidebarFocused && a.modal.kind == modalNone)
	a.models.width, a.models.height = a.mainW, bodyH
	a.org.width, a.org.height = a.mainW, bodyH
}

func (a *App) busy() bool {
	return a.chat.generating || a.models.pulling || a.models.loading || a.org.scanning
}

func (a *App) kick() tea.Cmd {
	if a.animating {
		return nil
	}
	a.animating = true
	return animTick()
}

func animTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return animMsg{} })
}

func (a *App) notify(text string, isErr bool) tea.Cmd {
	a.toast = text
	a.toastErr = isErr
	a.toastSeq++
	seq := a.toastSeq
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastExpiredMsg{seq} })
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		return a, nil
	case tea.KeyMsg:
		return a, a.handleKey(msg)
	case tea.MouseMsg:
		return a, nil
	case modelsLoadedMsg:
		return a, a.handleModelsLoaded(msg)
	case filesScanMsg:
		a.org.setReport(msg.report, msg.err)
		return a, nil
	case filesAppliedMsg:
		return a, a.handleFilesApplied(msg)
	case pullProgressMsg:
		return a, a.handlePull(msg)
	case modelDeletedMsg:
		return a, a.handleModelDeleted(msg)
	case chatEventMsg:
		return a, a.handleChatEvent(msg.ev)
	case animMsg:
		a.frame++
		if a.busy() {
			return a, animTick()
		}
		a.animating = false
		return a, nil
	case toastExpiredMsg:
		if msg.seq == a.toastSeq {
			a.toast = ""
		}
		return a, nil
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	a.chat.input, cmd = a.chat.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	a.editor.ta, cmd = a.editor.ta.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	a.editor.syncDirty()
	if a.modal.kind == modalInput {
		a.modal.input, cmd = a.modal.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if a.models.filtering {
		a.models.filterInput, cmd = a.models.filterInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return a, tea.Batch(cmds...)
}

func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		if a.chat.generating {
			a.cancelTurn()
			return nil
		}
		return a.quit()
	}
	if a.modal.kind != modalNone {
		return a.handleModalKey(msg)
	}
	switch k {
	case "ctrl+1", "alt+1", "f2":
		return a.switchTab(tabChat)
	case "ctrl+2", "alt+2", "f3":
		return a.switchTab(tabEditor)
	case "ctrl+3", "alt+3", "f4":
		return a.switchTab(tabModels)
	case "ctrl+4", "alt+4", "f5":
		return a.switchTab(tabFiles)
	case "ctrl+o":
		if a.sidebarW == 0 {
			return a.notify("file tree hidden — widen the terminal", false)
		}
		a.sidebarFocused = !a.sidebarFocused
		return a.refocus()
	case "f1":
		return a.openHelp()
	case "ctrl+s":
		if a.active == tabEditor && a.editor.path != "" {
			return a.saveEditor()
		}
		return nil
	case "ctrl+q":
		return a.quit()
	case "esc":
		if a.chat.generating {
			a.cancelTurn()
			return nil
		}
		if a.models.filtering {
			a.models.filtering = false
			a.models.filter = ""
			a.models.filterInput.SetValue("")
			a.models.filterInput.Blur()
			return nil
		}
		if a.sidebarFocused {
			a.sidebarFocused = false
			return a.refocus()
		}
		if a.active != tabChat {
			return a.switchTab(tabChat)
		}
		return nil
	}

	if a.chat.pending != nil && !a.sidebarFocused && a.active == tabChat {
		switch k {
		case "y":
			a.answerApproval(true)
			return nil
		case "n":
			a.answerApproval(false)
			return nil
		}
	}
	if a.sidebarFocused {
		return a.handleSidebarKey(msg)
	}
	switch a.active {
	case tabChat:
		return a.handleChatKey(msg)
	case tabEditor:
		return a.handleEditorKey(msg)
	case tabModels:
		return a.handleModelsKey(msg)
	case tabFiles:
		return a.handleFilesKey(msg)
	}
	return nil
}

func (a *App) handleModalKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	if a.modal.kind == modalHelp {
		if k == "esc" || k == "enter" || k == "q" || k == "?" {
			return a.closeModal()
		}
		return nil
	}
	switch k {
	case "esc":
		return a.closeModal()
	case "enter":
		return a.submitModal()
	}
	if a.modal.kind == modalConfirm {
		switch k {
		case "y", "Y":
			return a.submitModal()
		case "n", "N":
			return a.closeModal()
		}
		return nil
	}
	var cmd tea.Cmd
	a.modal.input, cmd = a.modal.input.Update(msg)
	return cmd
}

func (a *App) submitModal() tea.Cmd {
	m := &a.modal
	action := m.action
	value := strings.TrimSpace(m.input.Value())
	payload := m.payload
	a.modal = modalState{}

	var cmd tea.Cmd
	switch action {
	case "sidebar-new-file", "files-new-file":
		cmd = a.createPath(value, false, false)
	case "sidebar-new-dir", "files-new-dir":
		cmd = a.createPath(value, true, false)
	case "sidebar-rename":
		cmd = a.renamePath(payload, value)
	case "sidebar-delete":
		cmd = a.deletePath(payload)
	case "editor-new":
		cmd = a.createPath(value, false, true)
	case "editor-discard":
		cmd = a.openInEditor(payload)
	case "models-pull":
		cmd = a.startPull(value)
	case "models-delete":
		cmd = a.deleteModel(payload)
	case "files-apply":
		cmd = a.applyPlan()
	case "quit":
		return tea.Quit
	}
	return tea.Batch(cmd, a.refocus())
}

func (a *App) switchTab(t tab) tea.Cmd {
	a.active = t
	a.sidebarFocused = false
	return a.refocus()
}

func (a *App) refocus() tea.Cmd {
	if a.sidebarFocused {
		a.blurAll()
		return nil
	}
	switch a.active {
	case tabChat:
		return a.chat.input.Focus()
	case tabEditor:
		if a.editor.path != "" {
			return a.editor.ta.Focus()
		}
		a.blurAll()
		return nil
	case tabModels:
		if a.models.filtering {
			return a.models.filterInput.Focus()
		}
		a.blurAll()
		return nil
	default:
		a.blurAll()
		return nil
	}
}

func (a *App) quit() tea.Cmd {
	if a.editor.dirty {
		return a.openConfirm("quit", "UNSAVED CHANGES",
			"The editor has unsaved changes. Quit Monet Code and discard them?", "")
	}
	a.runner.Cancel()
	return tea.Quit
}

// --- chat -----------------------------------------------------------------

func (a *App) handleChatKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		return a.sendChat()
	case "pgup":
		a.chat.scrollUp()
		return nil
	case "pgdown":
		a.chat.scrollDown()
		return nil
	}
	var cmd tea.Cmd
	a.chat.input, cmd = a.chat.input.Update(msg)
	return cmd
}

func (a *App) sendChat() tea.Cmd {
	text := strings.TrimSpace(a.chat.input.Value())
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "/") {
		a.chat.input.Reset()
		return a.runSlash(text)
	}
	if a.chat.generating {
		return nil
	}
	if a.chat.model == "" {
		return a.notify("no model selected — open Models (⌃3)", true)
	}
	a.runner.SetModel(a.chat.model)
	a.chat.addUser(text)
	a.chat.input.Reset()
	a.chat.startAssistant()
	a.chat.pending = nil
	a.chat.thinking.Reset()
	gen, err := a.runner.Send(text)
	if err != nil {
		a.chat.addError(err.Error())
		a.chat.finishStream()
		return nil
	}
	a.chat.gen = gen
	return a.kick()
}

func (a *App) waitChat() tea.Cmd {
	ch := a.runner.Events()
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return chatEventMsg{ev: ev}
	}
}

func (a *App) handleChatEvent(ev agent.Event) tea.Cmd {
	if ev.Gen != a.chat.gen {
		return a.waitChat()
	}
	var extra []tea.Cmd
	switch ev.Type {
	case agent.EventDelta:
		a.chat.appendDelta(ev.Text)
	case agent.EventThinking:
		a.chat.thinking.WriteString(ev.Text)
		a.chat.activity = "thinking"
	case agent.EventToolCall:
		a.chat.pending = nil
		a.chat.addTool(ev.ToolName, ev.Text)
	case agent.EventApproval:
		a.chat.pending = &approvalRequest{tool: ev.ToolName, summary: ev.Text, args: ev.ToolArgs}
		a.chat.activity = "awaiting approval"
	case agent.EventToolResult:
		a.chat.completeTool(ev.ToolName, ev.ToolResult)
		if ev.ToolName == "write_file" || ev.ToolName == "create_directory" {
			a.sidebar.reload()
			extra = append(extra, a.scanFiles())
		}
	case agent.EventNotice:
		a.chat.addNotice(ev.Text)
	case agent.EventError:
		a.chat.addError(ev.Text)
		a.chat.finishStream()
		a.chat.gen = 0
	case agent.EventDone:
		a.chat.finishStream()
		a.chat.gen = 0
	}
	extra = append(extra, a.waitChat())
	return tea.Batch(extra...)
}

func (a *App) cancelTurn() {
	a.runner.Cancel()
	a.chat.finishStream()
	a.chat.gen = 0
	a.chat.addNotice("turn cancelled")
}

func (a *App) answerApproval(ok bool) {
	a.runner.Approve(ok)
	a.chat.pending = nil
	a.chat.activity = "running tool"
}

func (a *App) runSlash(text string) tea.Cmd {
	fields := strings.Fields(text)
	cmd := fields[0]
	arg := strings.TrimSpace(strings.TrimPrefix(text, cmd))
	switch cmd {
	case "/help", "/h", "/?":
		return a.openHelp()
	case "/clear":
		a.chat.clear()
		a.runner.Clear()
		a.chat.addNotice("conversation cleared")
		return nil
	case "/quit", "/exit":
		return a.quit()
	case "/models", "/model":
		if arg != "" {
			a.selectModel(arg)
			return a.notify("active model → "+arg, false)
		}
		return a.switchTab(tabModels)
	case "/files", "/tree":
		a.active = tabFiles
		a.sidebarFocused = a.sidebarW > 0
		return a.refocus()
	case "/editor", "/edit":
		if arg != "" {
			abs, err := files.SafeJoin(a.root, arg)
			if err != nil {
				return a.notify(err.Error(), true)
			}
			return a.openFile(abs)
		}
		return a.switchTab(tabEditor)
	case "/chat":
		return a.switchTab(tabChat)
	case "/new":
		if arg == "" {
			return a.notify("usage: /new <path>", true)
		}
		return a.createPath(arg, false, true)
	case "/save":
		return a.saveEditor()
	case "/status":
		mode := a.chat.model
		if mode == "" {
			mode = "none"
		}
		a.chat.addNotice(fmt.Sprintf("model %s · ollama %s · %d installed · %d messages · %s",
			mode, a.ollamaVersion, len(a.models.models), len(a.runner.History()), tildePath(a.root)))
		return nil
	case "/tools":
		a.chat.addNotice("tools: read_file · write_file · list_files · search_files · create_directory (writes need approval)")
		return nil
	case "/palette":
		a.chat.addNotice("palette  #0D1117 canvas · #88C0D0 frost blue · #8FBCBB glacier teal · #81A1C1 arctic blue")
		return nil
	default:
		return a.notify("unknown command "+cmd+" — try /help", true)
	}
}

// --- editor ---------------------------------------------------------------

func (a *App) handleEditorKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+n":
		return a.openInput("editor-new", "NEW FILE", "Path relative to the workspace root.", a.defaultNewPath(), "path/to/file.go")
	case "tab":
		a.editor.insertTab()
		return nil
	}
	var cmd tea.Cmd
	a.editor.ta, cmd = a.editor.ta.Update(msg)
	a.editor.syncDirty()
	return cmd
}

func (a *App) saveEditor() tea.Cmd {
	if a.editor.path == "" {
		return a.notify("no file open", true)
	}
	if err := a.editor.save(); err != nil {
		a.editor.err = err.Error()
		return a.notify("save failed: "+err.Error(), true)
	}
	a.editor.err = ""
	a.sidebar.reload()
	return a.notify("saved "+files.Rel(a.root, a.editor.path), false)
}

func (a *App) openFile(path string) tea.Cmd {
	if a.editor.dirty && a.editor.path != path {
		return a.openConfirm("editor-discard", "UNSAVED CHANGES",
			"Discard unsaved changes to "+files.Rel(a.root, a.editor.path)+" and open "+files.Rel(a.root, path)+"?", path)
	}
	return a.openInEditor(path)
}

func (a *App) openInEditor(path string) tea.Cmd {
	if err := a.editor.open(path); err != nil {
		return a.notify(err.Error(), true)
	}
	a.sidebar.focusPath(path)
	a.active = tabEditor
	a.sidebarFocused = false
	return tea.Batch(a.refocus(), a.notify("opened "+files.Rel(a.root, path), false))
}

func (a *App) defaultNewPath() string {
	n := a.sidebar.current()
	dir := a.root
	if n != nil {
		if n.dir {
			dir = n.path
		} else {
			dir = filepath.Dir(n.path)
		}
	}
	rel := files.Rel(a.root, dir)
	if rel == "." || rel == "" {
		return ""
	}
	return strings.TrimSuffix(rel, "/") + "/"
}

func (a *App) createPath(rel string, dir bool, open bool) tea.Cmd {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return a.notify("a path is required", true)
	}
	abs, err := files.SafeJoin(a.root, rel)
	if err != nil {
		return a.notify(err.Error(), true)
	}
	if dir {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return a.notify(err.Error(), true)
		}
		a.sidebar.reload()
		return tea.Batch(a.scanFiles(), a.notify("created directory "+files.Rel(a.root, abs), false))
	}
	if _, err := os.Stat(abs); err == nil {
		if open {
			return a.openFile(abs)
		}
		return a.notify("already exists: "+files.Rel(a.root, abs), true)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return a.notify(err.Error(), true)
	}
	if err := os.WriteFile(abs, nil, 0o644); err != nil {
		return a.notify(err.Error(), true)
	}
	a.sidebar.reload()
	cmds := []tea.Cmd{a.scanFiles(), a.notify("created "+files.Rel(a.root, abs), false)}
	if open {
		cmds = append(cmds, a.openInEditor(abs))
	}
	return tea.Batch(cmds...)
}

func (a *App) renamePath(oldAbs, newName string) tea.Cmd {
	newName = strings.TrimSpace(newName)
	if newName == "" || filepath.Base(newName) != newName || strings.ContainsAny(newName, `/\`) {
		return a.notify("enter a file name without slashes", true)
	}
	target := filepath.Join(filepath.Dir(oldAbs), newName)
	if target == oldAbs {
		return nil
	}
	abs, err := files.SafeJoin(a.root, target)
	if err != nil {
		return a.notify(err.Error(), true)
	}
	if _, err := os.Stat(abs); err == nil {
		return a.notify("already exists: "+newName, true)
	}
	if err := os.Rename(oldAbs, abs); err != nil {
		return a.notify(err.Error(), true)
	}
	if a.editor.path == oldAbs {
		a.editor.path = abs
	}
	a.sidebar.reload()
	return tea.Batch(a.scanFiles(), a.notify("renamed to "+newName, false))
}

func (a *App) deletePath(abs string) tea.Cmd {
	rel := files.Rel(a.root, abs)
	if abs == a.root {
		return a.notify("refusing to delete the workspace root", true)
	}
	if err := os.RemoveAll(abs); err != nil {
		return a.notify(err.Error(), true)
	}
	if a.editor.path == abs || strings.HasPrefix(a.editor.path, abs+string(filepath.Separator)) {
		a.editor.clear()
	}
	a.sidebar.reload()
	return tea.Batch(a.scanFiles(), a.notify("deleted "+rel, false))
}

// --- sidebar --------------------------------------------------------------

func (a *App) handleSidebarKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	switch k {
	case "up", "k":
		a.sidebar.move(-1)
	case "down", "j":
		a.sidebar.move(1)
	case "pgup":
		a.sidebar.move(-10)
	case "pgdown":
		a.sidebar.move(10)
	case "home":
		a.sidebar.cursor = 0
	case "end":
		a.sidebar.cursor = max(0, len(a.sidebar.visible)-1)
	case "right", "l":
		a.sidebar.expand()
	case "left", "h":
		a.sidebar.collapse()
	case "enter":
		n := a.sidebar.current()
		if n == nil {
			return nil
		}
		if n.dir {
			a.sidebar.toggle()
			return nil
		}
		return a.openFile(n.path)
	case "a":
		return a.openInput("sidebar-new-file", "NEW FILE", "Path relative to the workspace root.", a.defaultNewPath(), "path/to/file.go")
	case "A":
		return a.openInput("sidebar-new-dir", "NEW FOLDER", "Path relative to the workspace root.", a.defaultNewPath(), "path/to/folder")
	case "R":
		n := a.sidebar.current()
		if n == nil {
			return nil
		}
		return a.openInput("sidebar-rename", "RENAME", "New name for "+files.Rel(a.root, n.path)+".", baseName(n.path), baseName(n.path))
	case "d":
		n := a.sidebar.current()
		if n == nil {
			return nil
		}
		what := "file"
		if n.dir {
			what = "folder (and everything inside it)"
		}
		return a.openConfirm("sidebar-delete", "DELETE "+strings.ToUpper(what),
			"Permanently delete "+files.Rel(a.root, n.path)+"?", n.path)
	case "r":
		a.sidebar.reload()
		return a.notify("file tree refreshed", false)
	}
	return nil
}

// --- models ---------------------------------------------------------------

func (a *App) refreshModels() tea.Cmd {
	a.loadingModels = true
	a.models.loading = true
	return a.loadModelsCmd()
}

func (a *App) loadModelsCmd() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		version, _ := client.Version(ctx)
		models, err := client.List(ctx)
		return modelsLoadedMsg{models: models, version: version, err: err}
	}
}

func (a *App) handleModelsLoaded(msg modelsLoadedMsg) tea.Cmd {
	a.loadingModels = false
	a.ollamaOK = msg.err == nil
	a.ollamaVersion = msg.version
	a.models.setLoaded(msg.models, msg.version, msg.err)
	if msg.err != nil {
		a.ollamaErr = msg.err.Error()
		if !a.ollamaWarned {
			a.ollamaWarned = true
			a.chat.addError("ollama: " + msg.err.Error())
		}
		return nil
	}
	a.ollamaErr = ""
	a.ollamaWarned = false
	if a.chat.model == "" && len(msg.models) > 0 {
		a.selectModel(a.defaultModel(msg.models))
	}
	return nil
}

// defaultModel prefers the first curated coding model that is installed so a
// fresh install starts on the recommended model rather than an arbitrary one.
func (a *App) defaultModel(installed []ollama.Model) string {
	if len(installed) == 0 {
		return ""
	}
	trim := func(s string) string { return strings.TrimSuffix(s, ":latest") }
	for _, entry := range catalog.All() {
		for _, model := range installed {
			if trim(model.Name) == trim(entry.Name) {
				return model.Name
			}
		}
	}
	return installed[0].Name
}

func (a *App) handleModelsKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	if a.models.filtering {
		switch k {
		case "enter":
			a.models.filtering = false
			a.models.filterInput.Blur()
			return nil
		}
		var cmd tea.Cmd
		a.models.filterInput, cmd = a.models.filterInput.Update(msg)
		a.models.syncFilter()
		return cmd
	}
	switch k {
	case "up", "k":
		a.models.move(-1)
	case "down", "j":
		a.models.move(1)
	case "tab":
		a.models.section = (a.models.section + 1) % 2
	case "enter":
		return a.activateModel()
	case "p":
		return a.openInput("models-pull", "PULL MODEL", "Download from Ollama using name:tag, e.g. qwen2.5-coder:7b.", "", "qwen2.5-coder:7b")
	case "d":
		if sel := a.models.selectedInstalled(); sel != nil {
			return a.openConfirm("models-delete", "DELETE MODEL",
				"Remove "+sel.Name+" from disk? You will need to download it again later.", sel.Name)
		}
	case "r":
		return tea.Batch(a.refreshModels(), a.kick())
	case "/":
		a.models.filtering = true
		a.models.filterInput.SetValue(a.models.filter)
		a.models.filterInput.CursorEnd()
		return a.models.filterInput.Focus()
	}
	return nil
}

func (a *App) activateModel() tea.Cmd {
	if a.models.section == 0 {
		sel := a.models.selectedInstalled()
		if sel == nil {
			return nil
		}
		a.selectModel(sel.Name)
		return a.notify("active model → "+sel.Name, false)
	}
	sel := a.models.selectedCatalog()
	if sel == nil {
		return nil
	}
	if a.models.isInstalled(sel.Name) {
		a.selectModel(sel.Name)
		return a.notify("active model → "+sel.Name, false)
	}
	return a.openInput("models-pull", "PULL MODEL",
		"Download "+sel.Name+" from Ollama? "+sel.Description, sel.Name, "model:tag")
}

func (a *App) selectModel(name string) {
	name = strings.TrimSpace(name)
	a.chat.setModel(name)
	a.runner.SetModel(name)
	a.models.activeModel = name
	a.cfg.SetModel(name)
	_ = a.cfg.Save()
}

func (a *App) clearModel() {
	a.chat.setModel("")
	a.runner.SetModel("")
	a.models.activeModel = ""
	a.cfg.Model = ""
	_ = a.cfg.Save()
}

func (a *App) startPull(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		return a.notify("enter a model name", true)
	}
	if a.models.pulling {
		return a.notify("a download is already running", true)
	}
	a.models.pulling = true
	a.models.pullModel = name
	a.models.pullStatus = "starting"
	a.models.pullPct = 0
	ch := make(chan pullProgressMsg, 128)
	a.pullCh = ch
	client := a.client
	go func() {
		err := client.Pull(context.Background(), name, func(p ollama.PullProgress) {
			select {
			case ch <- pullProgressMsg{status: p.Status, completed: p.Completed, total: p.Total, model: name}:
			default:
			}
		})
		ch <- pullProgressMsg{model: name, done: true, err: err}
	}()
	return tea.Batch(a.waitPull(), a.kick())
}

func (a *App) waitPull() tea.Cmd {
	ch := a.pullCh
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (a *App) handlePull(msg pullProgressMsg) tea.Cmd {
	if !msg.done {
		a.models.pullStatus = msg.status
		if msg.total > 0 {
			a.models.pullPct = float64(msg.completed) / float64(msg.total)
		} else if msg.status != "" {
			a.models.pullPct = 0
		}
		return a.waitPull()
	}
	a.models.pulling = false
	a.models.pullPct = 0
	a.pullCh = nil
	if msg.err != nil {
		return tea.Batch(a.refreshModels(), a.kick(), a.notify("pull failed: "+msg.err.Error(), true))
	}
	if a.chat.model == "" {
		a.selectModel(msg.model)
	}
	return tea.Batch(a.refreshModels(), a.kick(), a.notify("pulled "+msg.model, false))
}

func (a *App) deleteModel(name string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return modelDeletedMsg{name: name, err: client.Delete(ctx, name)}
	}
}

func (a *App) handleModelDeleted(msg modelDeletedMsg) tea.Cmd {
	if msg.err != nil {
		return a.notify("delete failed: "+msg.err.Error(), true)
	}
	if a.chat.model == msg.name {
		a.clearModel()
	}
	return tea.Batch(a.refreshModels(), a.kick(), a.notify("deleted "+msg.name, false))
}

// --- files ----------------------------------------------------------------

func (a *App) scanFiles() tea.Cmd {
	root := a.root
	a.org.scanning = true
	return func() tea.Msg {
		rep, err := files.Analyze(root)
		return filesScanMsg{report: rep, err: err}
	}
}

func (a *App) handleFilesKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "o":
		if err := a.org.buildPreview(); err != nil {
			return a.notify(err.Error(), true)
		}
		if len(a.org.moves) == 0 {
			return a.notify("nothing to organize", false)
		}
		return a.notify(fmt.Sprintf("preview ready — %d files would move · press y to apply", len(a.org.moves)), false)
	case "y":
		if a.org.scanning {
			return a.notify("scan in progress…", false)
		}
		if !a.org.preview {
			if err := a.org.buildPreview(); err != nil {
				return a.notify(err.Error(), true)
			}
			if len(a.org.moves) == 0 {
				return a.notify("nothing to organize", false)
			}
			return a.notify("preview built — press y again to apply", false)
		}
		if len(a.org.moves) == 0 {
			return a.notify("nothing to organize", false)
		}
		return a.openConfirm("files-apply", "ORGANIZE FILES",
			fmt.Sprintf("Move %d files into category folders? This renames paths inside %s.", len(a.org.moves), tildePath(a.root)), "")
	case "r":
		a.sidebar.reload()
		return tea.Batch(a.scanFiles(), a.kick(), a.notify("rescanning…", false))
	case "n":
		return a.openInput("files-new-file", "NEW FILE", "Path relative to the workspace root.", "", "path/to/file.go")
	case "N":
		return a.openInput("files-new-dir", "NEW FOLDER", "Path relative to the workspace root.", "", "path/to/folder")
	case "up", "k":
		a.org.move(-1)
	case "down", "j":
		a.org.move(1)
	case "pgup":
		a.org.move(-10)
	case "pgdown":
		a.org.move(10)
	case "home":
		a.org.cursor = 0
	case "end":
		a.org.cursor = max(0, len(a.org.moves)-1)
	}
	return nil
}

func (a *App) applyPlan() tea.Cmd {
	moves := a.org.moves
	if len(moves) == 0 {
		return a.notify("nothing to organize", false)
	}
	return func() tea.Msg {
		moved, errs := files.Apply(moves)
		return filesAppliedMsg{moved: moved, errs: errs}
	}
}

func (a *App) handleFilesApplied(msg filesAppliedMsg) tea.Cmd {
	a.sidebar.reload()
	cmds := []tea.Cmd{a.scanFiles(), a.kick()}
	if msg.moved > 0 {
		cmds = append(cmds, a.notify(fmt.Sprintf("organized %s", plural(msg.moved, "file", "files")), false))
	}
	if len(msg.errs) > 0 {
		cmds = append(cmds, a.notify(fmt.Sprintf("%d moves failed: %v", len(msg.errs), msg.errs[0]), true))
	}
	if a.editor.path != "" {
		if _, err := os.Stat(a.editor.path); err != nil {
			a.editor.clear()
			cmds = append(cmds, a.notify("the open file moved — editor buffer closed", false))
		}
	}
	return tea.Batch(cmds...)
}

// --- chrome ---------------------------------------------------------------

func (a *App) headerView() string {
	dot := theme.DimText.Render("○ ollama")
	switch {
	case a.models.loading && a.ollamaVersion == "":
		dot = theme.Accent.Render(spinnerFrame(a.frame) + " ollama")
	case a.ollamaOK:
		dot = lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Green).Render("●") +
			theme.HeaderDim.Render(" ollama "+a.ollamaVersion)
	default:
		dot = lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Red).Render("●") +
			theme.HeaderDim.Render(" ollama offline")
	}
	model := a.chat.model
	if model == "" {
		model = "no model"
	}
	modelChip := lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Teal).Bold(true).Render(" " + model + " ")
	left := theme.HeaderTitle.Render(" ◆ MONET CODE") + theme.HeaderDim.Render("  v"+Version+"  ·  local agent")
	right := dot + "  " + modelChip + " "
	line1 := spread(left, right, a.width, theme.BgPanel)

	tabs := ""
	for i, name := range tabNames {
		label := " " + name + " "
		if tab(i) == a.active {
			tabs += theme.TabActive.Render(label)
		} else {
			tabs += theme.TabInactive.Render(label)
		}
		tabs += " "
	}
	hint := "F2‑5 views  ·  ⌃o files  ·  F1 help"
	if a.sidebarW == 0 {
		hint = "F2‑5 views  ·  F1 help"
	}
	if a.sidebarFocused {
		hint = "file tree focused  ·  esc to leave"
	}
	line2 := spread(" "+tabs, theme.HeaderDim.Render(hint+" "), a.width, theme.BgPanel)
	return line1 + "\n" + line2
}

func (a *App) statusView() string {
	hint := ""
	switch {
	case a.sidebarFocused:
		hint = "↑↓ move  ·  enter open  ·  a file  ·  A folder  ·  R rename  ·  d delete"
	case a.active == tabChat && a.chat.pending != nil:
		hint = "y approve  ·  n deny  ·  esc cancel"
	case a.active == tabChat && a.chat.generating:
		hint = spinnerFrame(a.frame) + " " + a.chat.activity + "  ·  esc cancel  ·  streaming from " + a.chat.model
	case a.active == tabChat:
		hint = "enter send  ·  alt+enter newline  ·  / commands  ·  pgup/pgdn scroll"
	case a.active == tabEditor:
		hint = "⌃s save  ·  ⌃n new  ·  tab indent  ·  esc chat"
	case a.active == tabModels:
		hint = "↑↓ select  ·  tab section  ·  enter use/pull  ·  p pull  ·  d delete  ·  / filter"
	case a.active == tabFiles:
		hint = "o preview  ·  y apply  ·  r rescan  ·  n file  ·  N folder  ·  ↑↓ preview"
	}
	right := ""
	if a.toast != "" {
		style := theme.StatusText
		if a.toastErr {
			style = lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Red)
		} else {
			style = lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Teal)
		}
		right = style.Render(" " + a.toast + " ")
	} else {
		right = theme.StatusText.Render(tildePath(a.root) + " ")
	}
	return spread(" "+hint, right, a.width, theme.BgPanel)
}

func (a *App) mainView(w, h int) string {
	if a.modal.kind != modalNone {
		return fitBlock(a.viewModal(w, h), w, h, theme.Bg)
	}
	var content string
	switch a.active {
	case tabChat:
		content = a.chat.view(w, h, a.frame, a.chat.input.Focused() && !a.sidebarFocused)
	case tabEditor:
		content = a.editor.view(a.root, w, h, a.editor.ta.Focused() && !a.sidebarFocused)
	case tabModels:
		content = a.models.view(w, h, a.frame)
	case tabFiles:
		content = a.org.view(w, h, a.frame)
	}
	return fitBlock(content, w, h, theme.Bg)
}

// View implements tea.Model.
func (a *App) View() string {
	if a.width <= 0 || a.height <= 0 {
		return ""
	}
	bodyH := max(6, a.height-3)
	header := fitBlock(a.headerView(), a.width, 2, theme.BgPanel)
	status := fitBlock(a.statusView(), a.width, 1, theme.BgPanel)
	var body string
	if a.sidebarW > 0 {
		left := fitBlock(a.sidebar.view(a.sidebarW, bodyH, a.frame), a.sidebarW, bodyH, theme.BgPanel)
		right := a.mainView(a.mainW, bodyH)
		body = joinColumns(left, right, a.sidebarW, a.mainW, bodyH)
	} else {
		body = a.mainView(a.width, bodyH)
	}
	return header + "\n" + body + "\n" + status
}

func joinColumns(left, right string, lw, rw, h int) string {
	ll := strings.Split(left, "\n")
	rl := strings.Split(right, "\n")
	divider := theme.SidebarDivider.Render("│")
	var out []string
	for i := 0; i < h; i++ {
		l, r := "", ""
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(rl) {
			r = rl[i]
		}
		out = append(out, l+divider+r)
	}
	return strings.Join(out, "\n")
}

// Snapshot renders a single frame without starting the terminal program. It is
// used for layout testing and documentation.
func (a *App) Snapshot(w, h int) string {
	a.width, a.height = w, h
	a.layout()
	return a.View()
}

// SnapshotTab renders a specific view for layout testing.
func (a *App) SnapshotTab(name string, w, h int) string {
	switch name {
	case "editor":
		a.active = tabEditor
	case "models":
		a.active = tabModels
	case "files":
		a.active = tabFiles
	case "help":
		a.modal = modalState{kind: modalHelp, title: "HELP", action: "help"}
	default:
		a.active = tabChat
	}
	return a.Snapshot(w, h)
}

// LoadModelsForSnapshot injects model data for tests and documentation.
func (a *App) LoadModelsForSnapshot(models []ollama.Model, version string) {
	a.handleModelsLoaded(modelsLoadedMsg{models: models, version: version})
}

// OpenForSnapshot opens a file in the editor for tests and documentation.
func (a *App) OpenForSnapshot(path string) error { return a.editor.open(path) }

func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(filepath.Separator)) {
			return "~" + path[len(home):]
		}
	}
	return path
}
