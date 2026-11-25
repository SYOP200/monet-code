package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet/internal/theme"
)

// fitBlock forces a rendered block to exactly w columns and h rows, painting
// padding with bg so theme resets never punch holes in the canvas.
func fitBlock(s string, w, h int, bg lipgloss.Color) string {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	padStyle := lipgloss.NewStyle().Background(bg)
	for i, line := range lines {
		lines[i] = fitLine(line, w, padStyle)
	}
	blank := padStyle.Render(strings.Repeat(" ", w))
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

func fitLine(line string, w int, pad lipgloss.Style) string {
	lw := lipgloss.Width(line)
	if lw > w {
		if w <= 1 {
			return ansi.Truncate(line, w, "")
		}
		return ansi.Truncate(line, w, "…")
	}
	if lw < w {
		return line + pad.Render(strings.Repeat(" ", w-lw))
	}
	return line
}

// spread places left and right on one row, separated by padding of bg.
func spread(left, right string, w int, bg lipgloss.Color) string {
	style := lipgloss.NewStyle().Background(bg)
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		right = ""
		rw = 0
		if lw > w {
			left = ansi.Truncate(left, w, "")
			lw = lipgloss.Width(left)
		}
	}
	gap := w - lw - rw
	if gap < 0 {
		gap = 0
	}
	return left + style.Render(strings.Repeat(" ", gap)) + right
}

// frameBox draws a rounded box with a title embedded in the top border and one
// column of padding around the content. Content is clipped/padded to fit.
func frameBox(title, subtitle string, content string, w, h int, borderColor lipgloss.Color, bg lipgloss.Color) string {
	if w < 6 || h < 3 {
		return fitBlock(content, w, h, bg)
	}
	border := lipgloss.NewStyle().Foreground(borderColor).Background(bg)
	inner := w - 2
	textW := w - 4
	contentH := h - 2

	// Top border with embedded title.
	label := ""
	if title != "" {
		label = " " + theme.PanelTitle.Foreground(borderColor).Background(bg).Render(title) + " "
	}
	if subtitle != "" {
		label += theme.SidebarDim.Foreground(theme.Dim).Background(bg).Render(subtitle) + " "
	}
	labelW := lipgloss.Width(label)
	if labelW > inner {
		label = ansi.Truncate(label, inner, "")
		labelW = lipgloss.Width(label)
	}
	top := border.Render("╭") + label + border.Render(strings.Repeat("─", max(0, inner-labelW))) + border.Render("╮")

	lines := strings.Split(content, "\n")
	rows := make([]string, 0, h)
	rows = append(rows, top)
	blankContent := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", textW))
	for i := 0; i < contentH; i++ {
		text := ""
		if i < len(lines) {
			text = lines[i]
		}
		text = fitLine(text, textW, lipgloss.NewStyle().Background(bg))
		rows = append(rows, border.Render("│")+" "+text+" "+border.Render("│"))
	}
	rows = append(rows, border.Render("╰")+border.Render(strings.Repeat("─", inner))+border.Render("╯"))
	out := strings.Join(rows, "\n")
	_ = blankContent
	return out
}

// centerBlock centers a block inside w×h, filling the rest with bg.
func centerBlock(block string, w, h int, bg lipgloss.Color) string {
	lines := strings.Split(block, "\n")
	bw := 0
	for _, l := range lines {
		if x := lipgloss.Width(l); x > bw {
			bw = x
		}
	}
	if bw > w {
		bw = w
	}
	top := (h - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	left := (w - bw) / 2
	if left < 0 {
		left = 0
	}
	pad := lipgloss.NewStyle().Background(bg)
	var out []string
	for i := 0; i < top; i++ {
		out = append(out, pad.Render(strings.Repeat(" ", w)))
	}
	for _, l := range lines {
		l = fitLine(l, bw, pad)
		line := pad.Render(strings.Repeat(" ", left)) + l
		if lipgloss.Width(line) < w {
			line += pad.Render(strings.Repeat(" ", w-lipgloss.Width(line)))
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, pad.Render(strings.Repeat(" ", w)))
	}
	if len(out) > h {
		out = out[:h]
	}
	return strings.Join(out, "\n")
}

// splitLong breaks a too-wide word into chunks that fit width.
func splitLong(word string, width int) []string {
	if width < 2 {
		width = 2
	}
	var chunks []string
	var cur strings.Builder
	curW := 0
	for _, r := range word {
		rw := lipgloss.Width(string(r))
		if curW+rw > width && cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	if len(chunks) == 0 {
		chunks = []string{""}
	}
	return chunks
}

// wrapText word-wraps plain text to width, preserving blank lines and
// hard-breaking long words.
func wrapText(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		cur := ""
		for _, word := range words {
			pieces := []string{word}
			if lipgloss.Width(word) > width {
				pieces = splitLong(word, width)
			}
			for _, piece := range pieces {
				if cur == "" {
					cur = piece
				} else if lipgloss.Width(cur)+1+lipgloss.Width(piece) <= width {
					cur += " " + piece
				} else {
					out = append(out, cur)
					cur = piece
				}
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

var (
	boldRe  = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	linkRe  = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	numRe   = regexp.MustCompile(`^(\d+)[.)]\s+(.*)$`)
	plainRe = regexp.MustCompile("`([^`]+)`")
)

func inlineStyles(s string) string {
	s = linkRe.ReplaceAllString(s, "$1 ($2)")
	var b strings.Builder
	parts := strings.Split(s, "`")
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString(theme.InlineCode.Render(part))
			continue
		}
		part = boldRe.ReplaceAllStringFunc(part, func(m string) string {
			txt := strings.Trim(m, "*")
			return lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Bg).Bold(true).Render(txt)
		})
		b.WriteString(part)
	}
	return b.String()
}

func indentLines(lines []string, prefix string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(prefix + l)
	}
	return b.String()
}

// renderMarkdown is a compact Markdown renderer tuned for the frost palette.
// It supports headings, bullets, numbers, quotes, rules, inline code, bold,
// links, and fenced code blocks.
func renderMarkdown(md string, width int) string {
	if width < 12 {
		width = 12
	}
	md = strings.ReplaceAll(md, "\r\n", "\n")
	lines := strings.Split(md, "\n")
	var out []string
	inCode := false
	lang := ""
	var code []string

	flushCode := func() {
		if len(code) == 0 {
			return
		}
		var clean []string
		for _, l := range code {
			clean = append(clean, ansi.Truncate(l, width-6, "…"))
		}
		if lang != "" {
			out = append(out, "  "+theme.DimText.Render(strings.ToLower(lang)))
		}
		boxStyle := theme.CodeBlock.Width(width - 4)
		out = append(out, indentLines(strings.Split(boxStyle.Render(strings.Join(clean, "\n")), "\n"), "  "))
		out = append(out, "")
		code = nil
		lang = ""
	}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
			} else {
				inCode = true
				lang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			}
			continue
		}
		if inCode {
			code = append(code, raw)
			continue
		}
		switch {
		case trimmed == "":
			out = append(out, "")
		case strings.HasPrefix(trimmed, "### "):
			out = append(out, theme.Heading2.Render(strings.TrimPrefix(trimmed, "### ")), "")
		case strings.HasPrefix(trimmed, "## "):
			out = append(out, theme.Heading.Render(strings.TrimPrefix(trimmed, "## ")), "")
		case strings.HasPrefix(trimmed, "# "):
			out = append(out, theme.Heading.Render(strings.TrimPrefix(trimmed, "# ")), "")
		case trimmed == "---" || trimmed == "***":
			out = append(out, theme.DimText.Render(strings.Repeat("─", width-4)))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ "):
			body := strings.TrimSpace(trimmed[2:])
			wrapped := wrapText(body, width-6)
			out = append(out, indentLines(wrapped, "  "+theme.BlueText.Render("•")+" "))
			out = append(out, "")
		case numRe.MatchString(trimmed):
			m := numRe.FindStringSubmatch(trimmed)
			wrapped := wrapText(m[2], width-7)
			out = append(out, indentLines(wrapped, "  "+theme.MutedText.Render(m[1]+".")+" "))
			out = append(out, "")
		case strings.HasPrefix(trimmed, "> "):
			wrapped := wrapText(strings.TrimPrefix(trimmed, "> "), width-6)
			out = append(out, indentLines(wrapped, "  "+theme.MutedText.Render("▏")+" "))
		default:
			wrapped := wrapText(trimmed, width-4)
			for _, w := range wrapped {
				out = append(out, "  "+inlineStyles(w))
			}
		}
	}
	if inCode {
		flushCode()
	}
	// Collapse runs of blank lines.
	var compact []string
	for _, l := range out {
		if l == "" && len(compact) > 0 && compact[len(compact)-1] == "" {
			continue
		}
		compact = append(compact, l)
	}
	return strings.TrimRight(strings.Join(compact, "\n"), "\n")
}

// renderPlain wraps text and styles inline code, used while streaming.
func renderPlain(text string, width int) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		for _, wrapped := range wrapText(line, width) {
			out = append(out, "  "+inlineStyles(wrapped))
		}
	}
	return strings.Join(out, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// chip renders a small inverted label.
func chip(text string) string {
	return theme.TabBadge.Render(" " + text + " ")
}

// spinnerFrames are used for every busy indicator.
var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func spinnerFrame(frame int) string {
	return string(spinnerFrames[frame%len(spinnerFrames)])
}
