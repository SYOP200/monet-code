package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet/internal/files"
	"github.com/syop200/monet/internal/theme"
)

type editorPane struct {
	ta       textarea.Model
	path     string
	original string
	dirty    bool
	tabSize  int
	width    int
	height   int
	err      string
}

func newEditorPane(tabSize int) *editorPane {
	ta := textarea.New()
	ta.Prompt = " "
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	base := theme.Base
	cursorLine := lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Fg)
	focused := textarea.Style{
		Base:             base,
		Text:             base,
		CursorLine:       cursorLine,
		CursorLineNumber: lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Blue),
		LineNumber:       lipgloss.NewStyle().Background(theme.Bg).Foreground(theme.Dim),
		EndOfBuffer:      theme.DimText,
		Placeholder:      theme.DimText,
		Prompt:           theme.DimText,
	}
	blurred := focused
	blurred.CursorLine = lipgloss.NewStyle().Background(theme.Bg).Foreground(theme.Fg)
	blurred.CursorLineNumber = lipgloss.NewStyle().Background(theme.Bg).Foreground(theme.Dim)
	ta.FocusedStyle = focused
	ta.BlurredStyle = blurred
	if tabSize <= 0 {
		tabSize = 4
	}
	return &editorPane{ta: ta, tabSize: tabSize}
}

func (e *editorPane) open(path string) error {
	text, _, err := files.ReadText(path, 5<<20)
	if err != nil {
		return err
	}
	e.path = path
	e.original = text
	e.dirty = false
	e.err = ""
	e.ta.SetValue(text)
	// SetValue leaves the cursor at the end of the inserted text; Update only
	// accepts keys while focused.
	e.ta.Focus()
	e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	e.ta.Blur()
	return nil
}

func (e *editorPane) clear() {
	e.path = ""
	e.original = ""
	e.dirty = false
	e.err = ""
	e.ta.SetValue("")
	e.ta.Blur()
}

func (e *editorPane) syncDirty() {
	if e.path == "" {
		e.dirty = false
		return
	}
	e.dirty = e.ta.Value() != e.original
}

func (e *editorPane) save() error {
	if e.path == "" {
		return os.ErrNotExist
	}
	text := e.ta.Value()
	if err := os.WriteFile(e.path, []byte(text), 0o644); err != nil {
		return err
	}
	e.original = text
	e.dirty = false
	return nil
}

func (e *editorPane) insertTab() {
	e.ta.InsertString(strings.Repeat(" ", e.tabSize))
	e.syncDirty()
}

func (e *editorPane) resize(w, h int, focused bool) {
	e.width, e.height = w, h
	e.ta.SetWidth(max(20, w-4))
	e.ta.SetHeight(max(1, h-4))
	if focused {
		e.ta.Focus()
	} else {
		e.ta.Blur()
	}
}

func (e *editorPane) view(root string, w, h int, focused bool) string {
	if w < 20 || h < 8 {
		return fitBlock(theme.DimText.Render("terminal too small"), w, h, theme.Bg)
	}
	e.ta.SetWidth(max(20, w-4))
	e.ta.SetHeight(max(1, h-4))

	if e.path == "" {
		body := strings.Join([]string{
			"",
			theme.Title.Render("  ◆ MONET CODE"),
			theme.DimText.Render("  a small atelier for local models"),
			"",
			theme.MutedText.Render("  No file open."),
			"",
			theme.Key.Render("  ⌃o") + theme.StatusText.Render("  focus the file tree, then enter to open"),
			theme.Key.Render("  ⌃n") + theme.StatusText.Render("  create a new file"),
			theme.Key.Render("  F2") + theme.StatusText.Render("  back to chat"),
			"",
			theme.DimText.Render("  " + ansi.Truncate(root, max(10, w-8), "…")),
		}, "\n")
		return frameBox("EDITOR", "", body, w, h, theme.Border, theme.Bg)
	}

	rel := files.Rel(root, e.path)
	left := theme.Accent.Render(" " + rel)
	if e.dirty {
		left += theme.WarnText.Render(" ●")
	}
	info := e.ta.LineInfo()
	pos := theme.DimText.Render(strings.TrimSpace(
		strings.Join([]string{
			"ln " + itoa(e.ta.Line()+1),
			"col " + itoa(info.CharOffset+1),
			files.HumanBytes(int64(len(e.ta.Value()))),
		}, "  ")))
	header := spread(left, pos, w, theme.Bg)
	if e.err != "" {
		header = spread(left, theme.ErrorText.Render(ansi.Truncate(e.err, max(10, w/2), "…")), w, theme.Bg)
	}

	border := theme.Border
	if focused {
		border = theme.Muted
	}
	box := frameBox("", "", e.ta.View(), w, h-1, border, theme.Bg)
	return strings.Join([]string{header, box}, "\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func baseName(path string) string { return filepath.Base(path) }
