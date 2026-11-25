package ui

import (
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
)

type modalState struct {
	kind    modalKind
	action  string
	title   string
	desc    string
	payload string
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
		lines := strings.Count(content, "\n") + 1
		boxH := min(max(8, lines+2), max(8, h-2))
		box := frameBox("HELP", "", content, boxW, boxH, theme.Teal, theme.BgPanel)
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
		keyLine("F2", "chat") + "     " + keyLine("F3", "editor") + "     " + keyLine("F4", "models") + "     " + keyLine("F5", "files"),
		keyLine("⌃o", "focus the file tree") + "     " + keyLine("⌃c", "quit / cancel") + "     " + keyLine("F1", "help"),
		"",
		sectionLine("chat"),
		keyLine("enter", "send") + "     " + keyLine("alt+enter", "newline") + "     " + keyLine("esc", "cancel turn"),
		keyLine("pgup/pgdn", "scroll the transcript"),
		theme.ModalDim.Render("  commands: ") + cmd("/help /clear /model <name> /files /editor /new /save /status /tools /quit"),
		"",
		sectionLine("editor"),
		keyLine("⌃s", "save file") + "     " + keyLine("⌃n", "new file") + "     " + keyLine("tab", "indent"),
		"",
		sectionLine("file tree"),
		keyLine("↑↓", "move") + "     " + keyLine("enter", "open / expand") + "     " + keyLine("←→", "collapse / expand"),
		keyLine("a", "new file") + "     " + keyLine("A", "new folder") + "     " + keyLine("R", "rename") + "     " + keyLine("d", "delete"),
		"",
		sectionLine("organizer"),
		keyLine("o", "preview moves") + "     " + keyLine("y", "apply plan") + "     " + keyLine("r", "rescan") + "     " + keyLine("n / N", "new file / folder"),
		"",
		sectionLine("models"),
		keyLine("tab", "installed ↔ catalog") + "     " + keyLine("enter", "use / pull") + "     " + keyLine("p", "pull by name"),
		keyLine("d", "delete model") + "     " + keyLine("r", "refresh") + "     " + keyLine("esc", "back to chat"),
		"",
		sectionLine("palette"),
		"  " + swatch("#0D1117", theme.Bg) + " canvas   " +
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
