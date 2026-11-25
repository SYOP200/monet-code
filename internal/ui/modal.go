package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet-code/internal/theme"
)

type modalKind int

const (
	modalNone modalKind = iota
	modalInput
	modalConfirm
	modalHelp
	modalThinking
	modalMessage
)

type modalState struct {
	kind    modalKind
	action  string
	title   string
	desc    string
	payload string
	text    string
	scroll  int
	input   textinput.Model
}

func (a *App) openInput(action, title, desc, value, placeholder string) tea.Cmd {
	m := &a.modal
	m.kind = modalInput
	m.action = action
	m.title = title
	m.desc = desc
	m.payload = ""
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	ti.CharLimit = 512
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.Blue).Background(theme.BgPanel).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.BgPanel)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Dim).Background(theme.BgPanel)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Teal).Background(theme.BgPanel)
	ti.Width = 52
	if value != "" {
		ti.SetValue(value)
		ti.CursorEnd()
	}
	m.input = ti
	a.blurAll()
	return m.input.Focus()
}

func (a *App) openConfirm(action, title, desc, payload string) tea.Cmd {
	m := &a.modal
	m.kind = modalConfirm
	m.action = action
	m.title = title
	m.desc = desc
	m.payload = payload
	m.input = textinput.New()
	a.blurAll()
	return nil
}

func (a *App) openHelp() tea.Cmd {
	a.modal = modalState{kind: modalHelp, title: "HELP", action: "help"}
	a.blurAll()
	return nil
}

// openThinking shows the model's reasoning trace in a scrollable modal.
func (a *App) openThinking(text string) tea.Cmd {
	if text == "" {
		text = "The model has not emitted reasoning tokens yet."
	}
	a.modal = modalState{kind: modalThinking, title: "THINKING", action: "thinking", text: text}
	a.blurAll()
	return nil
}

// openMessage shows an informational popup with a single dismissal key.
func (a *App) openMessage(title, desc string) tea.Cmd {
	a.modal = modalState{kind: modalMessage, title: title, desc: desc}
	a.blurAll()
	return nil
}

func (a *App) closeModal() tea.Cmd {
	a.modal = modalState{}
	return a.refocus()
}

func (a *App) blurAll() {
	a.chat.input.Blur()
	a.editor.ta.Blur()
	a.models.filterInput.Blur()
}

func (a *App) viewModal(w, h int) string {
	m := &a.modal
	boxW := min(72, w-6)
	if boxW < 32 {
		boxW = max(20, w-2)
	}
	switch m.kind {
	case modalInput:
		desc := strings.Join(wrapText(m.desc, boxW-6), "\n")
		content := desc + "\n\n" + m.input.View() + "\n\n" +
			theme.ModalDim.Render("enter submit  ·  esc cancel")
		lines := strings.Count(content, "\n") + 1
		boxH := min(max(7, lines+2), max(7, h-2))
		box := frameBox(m.title, "", content, boxW, boxH, theme.Teal, theme.BgPanel)
		return centerBlock(box, w, h, theme.Bg)
	case modalConfirm:
		desc := strings.Join(wrapText(m.desc, boxW-6), "\n")
		content := theme.ModalText.Render(desc) + "\n\n" +
			theme.Key.Render("y") + theme.ModalDim.Render(" confirm     ") +
			theme.Key.Render("n") + theme.ModalDim.Render(" cancel")
		lines := strings.Count(content, "\n") + 1
		boxH := min(max(7, lines+2), max(7, h-2))
		box := frameBox(m.title, "", content, boxW, boxH, theme.Yellow, theme.BgPanel)
		return centerBlock(box, w, h, theme.Bg)
	case modalHelp:
		content := helpContent(boxW - 4)
		lines := strings.Split(content, "\n")
		boxH := min(max(8, len(lines)+2), max(8, h-2))
		innerH := boxH - 2
		maxScroll := max(0, len(lines)-(innerH-1))
		m.scroll = clamp(m.scroll, 0, maxScroll)
		end := min(len(lines), m.scroll+innerH-1)
		body := strings.Join(lines[m.scroll:end], "\n")
		body += "\n" + theme.ModalDim.Render(" ↑↓ scroll  ·  esc close")
		box := frameBox("HELP", "", body, boxW, boxH, theme.Teal, theme.BgPanel)
		return centerBlock(box, w, h, theme.Bg)
	case modalThinking:
		boxW = min(88, max(40, w-6))
		boxH := min(max(12, h-6), max(8, h-2))
		innerW := boxW - 4
		innerH := boxH - 2
		var lines []string
		for _, raw := range strings.Split(strings.ReplaceAll(m.text, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(raw) == "" {
				lines = append(lines, "")
				continue
			}
			for _, wrapped := range wrapText(raw, innerW-2) {
				lines = append(lines, theme.ModalText.Render(" "+wrapped))
			}
		}
		maxScroll := max(0, len(lines)-(innerH-3))
		m.scroll = clamp(m.scroll, 0, maxScroll)
		end := min(len(lines), m.scroll+innerH-3)
		view := lines[m.scroll:end]
		meta := theme.ModalDim.Render(fmt.Sprintf(" %d lines · %d-%d", len(lines), m.scroll+1, end))
		content := meta + "\n" + strings.Join(view, "\n")
		for strings.Count(content, "\n") < innerH-2 {
			content += "\n"
		}
		content += theme.ModalDim.Render(" ↑↓ scroll  ·  esc close")
		box := frameBox(m.title, "", content, boxW, boxH, theme.Purple, theme.BgPanel)
		return centerBlock(box, w, h, theme.Bg)
	case modalMessage:
		desc := strings.Join(wrapText(m.desc, boxW-6), "\n")
		content := theme.ModalText.Render(desc) + "\n\n" +
			theme.Key.Render("esc") + theme.ModalDim.Render(" close")
		lines := strings.Count(content, "\n") + 1
		boxH := min(max(7, lines+2), max(7, h-2))
		box := frameBox(m.title, "", content, boxW, boxH, theme.Blue, theme.BgPanel)
		return centerBlock(box, w, h, theme.Bg)
	}
	return ""
}

func keyLine(keys, desc string) string {
	return theme.Key.Render(keys) + "  " + theme.ModalDim.Render(desc)
}

func sectionLine(title string) string {
	return theme.ModalTitle.Render(strings.ToUpper(title))
}

func helpContent(width int) string {
	cmd := func(s string) string { return theme.BlueText.Render(s) }
	lines := []string{
		sectionLine("views"),
		keyLine("click", "the tabs") + "     " + keyLine("F2", "chat") + "     " + keyLine("F3", "editor") + "     " + keyLine("F4", "models") + "     " + keyLine("F5", "files"),
		keyLine("⌃o", "focus the file tree") + "     " + keyLine("F1", "help") + "     " + keyLine("⌃d", "quit") + "     " + keyLine("⌃c", "shows the exit hint"),
		"",
		sectionLine("chat"),
		keyLine("enter", "send") + "     " + keyLine("alt+enter", "newline") + "     " + keyLine("esc", "cancel turn"),
		keyLine("pgup/pgdn", "scroll") + "     " + keyLine("wheel", "scroll the transcript") + "     " + keyLine(":q", "quit (also :q!, :wq)"),
		theme.ModalDim.Render("  typing / opens the command popup · ↑↓ choose · tab complete · enter run"),
		theme.ModalDim.Render("  click a thinking line to read the model's reasoning"),
		"",
		sectionLine("commands"),
		cmd("/help /clear /compact /status /tools /thinking /palette /quit"),
		cmd("/model /models /files /editor /chat /new /save"),
		cmd("/review /diff /git /commit /worktree"),
		cmd("/agent /agents /sessions /session /resume /checkpoint /rewind"),
		cmd("/init /remember /memory /hooks /skills /reload"),
		cmd("/msg /inbox /worktrees /branch /push /pr /prs"),
		"",
		sectionLine("editor"),
		keyLine("⌃s", "save") + "     " + keyLine("⌃n", "new file") + "     " + keyLine("tab", "indent"),
		"",
		sectionLine("file tree"),
		keyLine("↑↓", "move") + "     " + keyLine("enter", "open / expand") + "     " + keyLine("←→", "collapse / expand"),
		keyLine("a", "new file") + "     " + keyLine("A", "new folder") + "     " + keyLine("R", "rename") + "     " + keyLine("d", "delete"),
		"",
		sectionLine("agent safety"),
		theme.ModalDim.Render("  writes, deletes, moves and commands ask for approval; rm -rf is refused"),
		theme.ModalDim.Render("  every edit is checkpointed — /rewind restores the last one"),
		"",
		sectionLine("palette"),
		"  " + swatch("#000000", theme.Bg) + " canvas   " +
			swatch("#88C0D0", theme.Fg) + " frost blue   " +
			swatch("#8FBCBB", theme.Teal) + " glacier teal   " +
			swatch("#81A1C1", theme.Blue) + " arctic blue",
	}
	out := strings.Join(lines, "\n")
	// Avoid overflowing narrow terminals.
	if width > 0 {
		var clipped []string
		for _, l := range strings.Split(out, "\n") {
			clipped = append(clipped, ansi.Truncate(l, width, "…"))
		}
		out = strings.Join(clipped, "\n")
	}
	return out
}
func swatch(hex string, c lipgloss.Color) string {
	return lipgloss.NewStyle().Background(c).Foreground(c).Render("██") +
		theme.ModalDim.Render(" "+hex)
}
