package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/syop200/monet-code/internal/theme"
)

type chatMessage struct {
	role        string // user | assistant | tool | notice | error
	content     string
	toolName    string
	toolSummary string
	toolResult  string
	toolPending bool
	rendered    string
	renderWidth int
	complete    bool
}

type approvalRequest struct {
	tool    string
	summary string
	args    string
}

type chatPane struct {
	input    textarea.Model
	viewport viewport.Model

	messages   []*chatMessage
	streaming  *chatMessage
	thinking   strings.Builder
	activity   string
	generating bool

	model      string
	gen        int
	pending    *approvalRequest
	autoScroll bool
	width      int
	height     int
}

func newChatPane(width int) *chatPane {
	ta := textarea.New()
	ta.Placeholder = "Ask Monet to build, refactor, or explain something…"
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.SetWidth(max(20, width-6))
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	ta.FocusedStyle.Base = theme.Base
	ta.FocusedStyle.Text = theme.Base
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Fg)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Blue).Background(theme.Bg).Bold(true)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.Dim).Background(theme.Bg)
	ta.FocusedStyle.EndOfBuffer = theme.DimText
	ta.BlurredStyle.Base = theme.Base
	ta.BlurredStyle.Text = theme.Base
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle().Background(theme.Bg).Foreground(theme.Fg)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Muted).Background(theme.Bg)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.Dim).Background(theme.Bg)
	ta.BlurredStyle.EndOfBuffer = theme.DimText

	vp := viewport.New(max(20, width-2), 10)
	return &chatPane{input: ta, viewport: vp, autoScroll: true}
}

func (c *chatPane) resize(width, height int) {
	c.width, c.height = width, height
	c.input.SetWidth(max(20, width-6))
}

func (c *chatPane) setModel(model string) {
	c.model = model
	if model == "" {
		c.input.Placeholder = "Ask Monet · select a model in ⌃3 · /help for commands"
	} else {
		c.input.Placeholder = "Ask Monet · " + model + " · /help for commands"
	}
}

func (c *chatPane) add(m *chatMessage) *chatMessage {
	c.messages = append(c.messages, m)
	c.autoScroll = true
	return m
}

func (c *chatPane) addUser(text string) {
	c.add(&chatMessage{role: "user", content: text, complete: true})
}

func (c *chatPane) startAssistant() {
	c.streaming = c.add(&chatMessage{role: "assistant", complete: false})
	c.generating = true
	c.activity = "thinking"
}

func (c *chatPane) appendDelta(text string) {
	if c.streaming == nil {
		c.streaming = c.add(&chatMessage{role: "assistant", complete: false})
	}
	c.streaming.content += text
	c.streaming.renderWidth = -1
	c.activity = "writing"
}

func (c *chatPane) addTool(name, summary string) {
	c.add(&chatMessage{role: "tool", toolName: name, toolSummary: summary, toolPending: true})
	c.activity = name
}

func (c *chatPane) completeTool(name, result string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		m := c.messages[i]
		if m.role == "tool" && m.toolName == name && m.toolPending {
			m.toolPending = false
			m.toolResult = result
			m.renderWidth = -1
			return
		}
	}
	c.add(&chatMessage{role: "tool", toolName: name, toolSummary: result, complete: true})
}

func (c *chatPane) addNotice(text string) {
	c.add(&chatMessage{role: "notice", content: text, complete: true})
}

func (c *chatPane) addError(text string) {
	c.add(&chatMessage{role: "error", content: text, complete: true})
}

func (c *chatPane) finishStream() {
	if c.streaming != nil {
		c.streaming.complete = true
		c.streaming.renderWidth = -1
		if strings.TrimSpace(c.streaming.content) == "" && c.streaming.role == "assistant" {
			c.streaming.content = "_no content returned_"
		}
	}
	c.streaming = nil
	c.generating = false
	c.activity = ""
	c.thinking.Reset()
	c.pending = nil
}

func (c *chatPane) renderAll(width int) string {
	contentW := width - 2
	if contentW < 20 {
		contentW = 20
	}
	var parts []string
	for _, m := range c.messages {
		if m.renderWidth != contentW {
			m.rendered = renderMessage(m, contentW)
			m.renderWidth = contentW
		}
		if m.rendered != "" {
			parts = append(parts, m.rendered)
		}
	}
	if c.generating && c.thinking.Len() > 0 && c.streaming != nil {
		snippet := strings.ReplaceAll(c.thinking.String(), "\n", " ")
		snippet = strings.TrimSpace(snippet)
		if len(snippet) > 160 {
			snippet = "…" + snippet[len(snippet)-160:]
		}
		parts = append(parts, theme.Notice.Render("⋯ "+snippet))
	}
	return strings.Join(parts, "\n\n")
}

func renderMessage(m *chatMessage, width int) string {
	inner := width - 2
	if inner < 16 {
		inner = 16
	}
	var b strings.Builder
	switch m.role {
	case "user":
		b.WriteString(theme.UserLabel.Render("❯ you"))
		b.WriteString("\n")
		b.WriteString(renderPlain(m.content, inner))
	case "assistant":
		b.WriteString(theme.AssistantLabel.Render("◆ monet"))
		b.WriteString("\n")
		switch {
		case m.content == "":
			b.WriteString(theme.DimText.Render("  working…"))
		case m.complete:
			b.WriteString(renderMarkdown(m.content, inner))
		default:
			b.WriteString(renderPlain(m.content, inner) + theme.Accent.Render("▌"))
		}
	case "tool":
		icon := "⚙"
		if m.toolPending {
			icon = "◌"
		}
		label := theme.ToolLabel.Render(icon+" "+m.toolName) + "  " + theme.DimText.Render(m.toolSummary)
		b.WriteString(label)
		if m.toolResult != "" {
			b.WriteString("\n")
			lines := strings.Split(strings.TrimRight(m.toolResult, "\n"), "\n")
			if len(lines) > 4 {
				lines = append(lines[:4], theme.DimText.Render("…"))
			}
			for i, l := range lines {
				l = strings.TrimRight(l, " ")
				if len(l) > inner-6 {
					l = l[:inner-6] + "…"
				}
				prefix := theme.DimText.Render("  └ ")
				if i > 0 {
					prefix = "    "
				}
				b.WriteString(prefix + theme.MutedText.Render(l))
				if i < len(lines)-1 {
					b.WriteString("\n")
				}
			}
		}
	case "notice":
		b.WriteString(theme.Notice.Render("· " + m.content))
	case "error":
		b.WriteString(theme.ErrorText.Render("✗ " + m.content))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (c *chatPane) view(w, h int, frame int, focused bool) string {
	if w < 20 || h < 8 {
		return fitBlock(theme.DimText.Render("terminal too small"), w, h, theme.Bg)
	}
	approvalH := 0
	if c.pending != nil {
		approvalH = 4
	}
	vpH := h - 5 - approvalH
	if vpH < 3 {
		vpH = 3
	}
	contentW := w - 2
	c.viewport.Width = max(10, w-2)
	c.viewport.Height = vpH
	c.width = w

	wasAtBottom := c.viewport.AtBottom() || c.autoScroll
	c.viewport.SetContent(c.renderAll(contentW))
	if wasAtBottom {
		c.viewport.GotoBottom()
		c.autoScroll = true
	}

	border := theme.Border
	if focused {
		border = theme.Muted
	}
	rows := []string{c.viewport.View()}
	if c.pending != nil {
		msg := theme.WarnText.Render("⚠ "+c.pending.tool) + "  " + theme.ModalText.Render(c.pending.summary)
		rows = append(rows, frameBox("APPROVAL", "", msg, w, 4, theme.Yellow, theme.Bg))
	}
	rows = append(rows, frameBox("", "", promptLines(c.input.View()), w, 5, border, theme.Bg))
	return strings.Join(rows, "\n")
}

// promptLines puts a single caret at the start of the input and indents the
// continuation lines, instead of repeating the prompt on every row.
func promptLines(view string) string {
	lines := strings.Split(view, "\n")
	caret := lipgloss.NewStyle().Foreground(theme.Blue).Background(theme.Bg).Bold(true).Render("❯ ")
	for i := range lines {
		if i == 0 {
			lines[i] = caret + lines[i]
			continue
		}
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (c *chatPane) clear() {
	c.messages = nil
	c.streaming = nil
	c.generating = false
	c.thinking.Reset()
	c.pending = nil
	c.autoScroll = true
	c.viewport.SetContent("")
}

func (c *chatPane) scrollUp() {
	c.viewport.PageUp()
	c.autoScroll = false
}

func (c *chatPane) scrollDown() {
	c.viewport.PageDown()
	if c.viewport.AtBottom() {
		c.autoScroll = true
	}
}
