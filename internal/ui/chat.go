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
	thinking    string
	toolName    string
	toolSummary string
	toolResult  string
	toolPending bool
	rendered    string
	renderWidth int
	complete    bool
}

type messageSpan struct {
	start int
	end   int
	index int
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

	model        string
	gen          int
	pending      *approvalRequest
	autoScroll   bool
	width        int
	height       int
	menu         string
	menuH        int
	menuTop      int
	spans        []messageSpan
	lastThinking string
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
	c.activity = "thinking…"
}

func (c *chatPane) appendDelta(text string) {
	if c.streaming == nil {
		c.streaming = c.add(&chatMessage{role: "assistant", complete: false})
	}
	c.streaming.content += text
	c.streaming.renderWidth = -1
	c.activity = "writing…"
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
		if strings.TrimSpace(c.streaming.thinking) != "" {
			c.lastThinking = c.streaming.thinking
		}
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

func (c *chatPane) renderAll(contentW int) string {
	if contentW < 20 {
		contentW = 20
	}
	var b strings.Builder
	c.spans = c.spans[:0]
	line := 0
	for i, m := range c.messages {
		if m.renderWidth != contentW {
			m.rendered = renderMessage(m, contentW)
			m.renderWidth = contentW
		}
		if m.rendered == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
			line += 2
		}
		start := line
		b.WriteString(m.rendered)
		line += strings.Count(m.rendered, "\n") + 1
		c.spans = append(c.spans, messageSpan{start: start, end: line - 1, index: i})
	}
	return b.String()
}

// messageAt maps a rendered content line back to a message.
func (c *chatPane) messageAt(line int) *chatMessage {
	for _, span := range c.spans {
		if line >= span.start && line <= span.end {
			if span.index >= 0 && span.index < len(c.messages) {
				return c.messages[span.index]
			}
		}
	}
	return nil
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return "…" + s[len(s)-max:]
	}
	return s
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
		if m.thinking != "" || !m.complete {
			marker := "  ▸ thinking · click to view"
			if !m.complete {
				snippet := oneLine(m.thinking, 90)
				marker = "  ⋯ thinking…"
				if snippet != "" {
					marker += "  " + snippet
				}
				marker += "  · click to view"
			}
			b.WriteString("\n")
			b.WriteString(theme.Notice.Render(marker))
		}
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
	menuH := c.menuH
	if menuH > h-12 {
		menuH = max(0, h-12)
	}
	vpH := h - 5 - approvalH - menuH
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
	c.menuTop = vpH + approvalH
	if menuH > 0 && c.menu != "" {
		rows = append(rows, c.menu)
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

func (c *chatPane) scrollBy(delta int) {
	if delta < 0 {
		c.viewport.LineUp(-delta)
		c.autoScroll = false
		return
	}
	c.viewport.LineDown(delta)
	if c.viewport.AtBottom() {
		c.autoScroll = true
	}
}

// toolActivity turns a tool name into a short present-tense status label.
func toolActivity(name string) string {
	switch name {
	case "read_file":
		return "reading…"
	case "write_file":
		return "editing file…"
	case "create_directory":
		return "designing…"
	case "list_files", "search_files":
		return "exploring…"
	case "delete_file":
		return "removing a file…"
	case "move_file":
		return "moving a file…"
	case "run_command":
		return "running a command…"
	case "git_status", "git_diff", "git_log":
		return "checking git…"
	case "git_commit":
		return "committing…"
	default:
		return name + "…"
	}
}
