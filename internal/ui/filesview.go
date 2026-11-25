package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet/internal/files"
	"github.com/syop200/monet/internal/theme"
)

type filesPane struct {
	root     string
	report   files.Report
	moves    []files.Move
	preview  bool
	scanning bool
	err      string
	cursor   int
	offset   int
	scanned  time.Time
	width    int
	height   int
}

func newFilesPane(root string) *filesPane {
	return &filesPane{root: root}
}

func (f *filesPane) setReport(rep files.Report, err error) {
	f.scanning = false
	f.scanned = time.Now()
	if err != nil {
		f.err = err.Error()
		return
	}
	f.err = ""
	f.report = rep
	f.preview = false
	f.moves = nil
	f.cursor = 0
	f.offset = 0
}

func (f *filesPane) buildPreview() error {
	moves, err := files.Plan(f.report)
	if err != nil {
		return err
	}
	f.moves = moves
	f.preview = true
	f.cursor = 0
	f.offset = 0
	return nil
}

func (f *filesPane) move(delta int) {
	if len(f.moves) == 0 {
		return
	}
	f.cursor = clamp(f.cursor+delta, 0, len(f.moves)-1)
}

func (f *filesPane) view(w, h int, frame int) string {
	if w < 30 || h < 10 {
		return fitBlock(theme.DimText.Render("terminal too small"), w, h, theme.Bg)
	}
	f.width, f.height = w, h
	groups := f.report.Groups
	groupsH := clamp(len(groups)+2, 4, max(4, (h-2)/2))
	previewH := h - 1 - groupsH

	header := theme.Accent.Render(" FILE ORGANIZATION")
	stats := ""
	if f.scanning {
		stats = theme.Accent.Render(spinnerFrame(frame) + " scanning…")
	} else if f.err != "" {
		stats = theme.ErrorText.Render("scan failed")
	} else {
		stats = theme.DimText.Render(fmt.Sprintf("%s  ·  %s  ·  %s",
			tildePath(f.root), plural(f.report.FileCount, "loose file", "loose files"), files.HumanBytes(f.report.Size)))
	}
	rows := []string{spread(header, stats+" ", w, theme.Bg)}

	rows = append(rows, frameBox("BY CATEGORY", "", f.groupsContent(w, groupsH), w, groupsH, theme.Border, theme.Bg))
	rows = append(rows, frameBox("PREVIEW", "", f.previewContent(w, previewH), w, previewH, theme.Border, theme.Bg))
	return strings.Join(rows, "\n")
}

func (f *filesPane) groupsContent(w, h int) string {
	inner := w - 4
	if f.scanning {
		return theme.DimText.Render("  scanning the top level of " + tildePath(f.root) + "…")
	}
	if f.err != "" {
		return theme.ErrorText.Render("  " + ansi.Truncate(f.err, inner-2, "…"))
	}
	if len(f.report.Groups) == 0 {
		return theme.OKText.Render("  ✓ nothing to sort — the top level is already tidy") + "\n" +
			theme.DimText.Render("  hidden files and folders are never touched")
	}
	var rows []string
	for _, g := range f.report.Groups {
		cat := ansi.Truncate(g.Category.Name, 12, "…")
		count := fmt.Sprintf("%d", len(g.Files))
		size := files.HumanBytes(g.Size)
		line := " " + theme.Base.Render(cat) + " " + theme.DimText.Render(strings.Repeat(".", max(2, 14-lipgloss.Width(cat)))) +
			" " + theme.Accent.Render(fmt.Sprintf("%2s", count)) + theme.DimText.Render(" files  ") +
			theme.MutedText.Render(fmt.Sprintf("%9s", size)) +
			theme.DimText.Render("  →  ") + theme.BlueText.Render(g.Category.Folder+"/")
		rows = append(rows, line)
	}
	if len(rows) > h-2 {
		rows = rows[:h-2]
	}
	return strings.Join(rows, "\n")
}

func (f *filesPane) previewContent(w, h int) string {
	inner := w - 4
	contentH := h - 2
	if f.err != "" {
		return theme.ErrorText.Render("  " + ansi.Truncate(f.err, inner-2, "…"))
	}
	if !f.preview {
		if f.report.FileCount == 0 {
			return theme.DimText.Render("  nothing loose to move") + "\n" +
				theme.DimText.Render("  use the file tree (⌃o) to open, rename or delete files")
		}
		return theme.MutedText.Render("  o  build a dry-run plan for the "+plural(f.report.FileCount, "file", "files")+" above") + "\n" +
			theme.DimText.Render("  y  apply the plan once you have previewed it") + "\n" +
			theme.DimText.Render("  files move into category folders next to them; nothing leaves the workspace")
	}
	if len(f.moves) == 0 {
		return theme.OKText.Render("  ✓ plan is empty — everything is already in place")
	}
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if f.cursor >= f.offset+contentH {
		f.offset = f.cursor - contentH + 1
	}
	f.offset = clamp(f.offset, 0, max(0, len(f.moves)-contentH))

	var rows []string
	for i := f.offset; i < len(f.moves) && len(rows) < contentH; i++ {
		m := f.moves[i]
		label := m.RelFrom + theme.DimText.Render("  →  ") + theme.BlueText.Render(m.RelTo)
		if lipgloss.Width(label) > inner-2 {
			label = ansi.Truncate(label, inner-2, "…")
		}
		if i == f.cursor {
			plain := ansi.Truncate(m.RelFrom+"  →  "+m.RelTo, inner, "…")
			rows = append(rows, theme.SelectedRow.Render(fitPlain(" "+plain, inner)))
		} else {
			rows = append(rows, " "+label)
		}
	}
	remaining := len(f.moves) - f.offset - len(rows)
	if remaining > 0 {
		rows[len(rows)-1] = theme.DimText.Render(fmt.Sprintf(" … %d more", remaining))
	}
	return strings.Join(rows, "\n")
}
