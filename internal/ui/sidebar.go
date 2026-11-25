package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet/internal/files"
	"github.com/syop200/monet/internal/theme"
)

type treeNode struct {
	name     string
	path     string
	dir      bool
	expanded bool
	depth    int
	children []*treeNode
}

type sidebar struct {
	root     string
	rootNode *treeNode
	visible  []*treeNode
	cursor   int
	offset   int
	focused  bool
	width    int
	height   int
	err      string
}

func newSidebar(root string) *sidebar {
	s := &sidebar{root: root}
	s.reload()
	return s
}

func (s *sidebar) reload() {
	expanded := map[string]bool{}
	if s.rootNode != nil {
		var collect func(n *treeNode)
		collect = func(n *treeNode) {
			if n.dir && n.expanded {
				expanded[n.path] = true
			}
			for _, c := range n.children {
				collect(c)
			}
		}
		collect(s.rootNode)
	}
	selected := ""
	if n := s.current(); n != nil {
		selected = n.path
	}

	root := &treeNode{name: filepath.Base(s.root), path: s.root, dir: true, expanded: true, depth: 0}
	s.err = ""
	s.loadChildren(root)
	s.rootNode = root

	paths := make([]string, 0, len(expanded))
	for p := range expanded {
		if p != s.root {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], string(filepath.Separator)) < strings.Count(paths[j], string(filepath.Separator))
	})
	for pass := 0; pass < 3; pass++ {
		for _, p := range paths {
			if n := findNode(root, p); n != nil && n.dir {
				if !n.expanded {
					n.expanded = true
					s.loadChildren(n)
				}
			}
		}
	}

	s.flatten()
	if selected != "" {
		for i, n := range s.visible {
			if n.path == selected {
				s.cursor = i
				break
			}
		}
	}
	s.cursor = clamp(s.cursor, 0, max(0, len(s.visible)-1))
}

func findNode(root *treeNode, path string) *treeNode {
	if root.path == path {
		return root
	}
	for _, c := range root.children {
		if n := findNode(c, path); n != nil {
			return n
		}
	}
	return nil
}

func (s *sidebar) loadChildren(n *treeNode) {
	entries, err := os.ReadDir(n.path)
	if err != nil {
		s.err = err.Error()
		return
	}
	var dirs, normal []*treeNode
	count := 0
	for _, e := range entries {
		if count >= 400 {
			break
		}
		name := e.Name()
		if e.IsDir() {
			if files.ShouldSkipDir(name) {
				continue
			}
			dirs = append(dirs, &treeNode{name: name, path: filepath.Join(n.path, name), dir: true, depth: n.depth + 1})
		} else {
			normal = append(normal, &treeNode{name: name, path: filepath.Join(n.path, name), depth: n.depth + 1})
		}
		count++
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name) })
	sort.Slice(normal, func(i, j int) bool { return strings.ToLower(normal[i].name) < strings.ToLower(normal[j].name) })
	n.children = append(dirs, normal...)
}

func (s *sidebar) flatten() {
	s.visible = s.visible[:0]
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		for _, c := range n.children {
			s.visible = append(s.visible, c)
			if c.dir && c.expanded {
				walk(c)
			}
		}
	}
	walk(s.rootNode)
}

func (s *sidebar) current() *treeNode {
	if s.cursor < 0 || s.cursor >= len(s.visible) {
		return nil
	}
	return s.visible[s.cursor]
}

func (s *sidebar) move(delta int) {
	if len(s.visible) == 0 {
		return
	}
	s.cursor = clamp(s.cursor+delta, 0, len(s.visible)-1)
}

func (s *sidebar) toggle() {
	n := s.current()
	if n == nil || !n.dir {
		return
	}
	n.expanded = !n.expanded
	if n.expanded && len(n.children) == 0 {
		s.loadChildren(n)
	}
	s.flatten()
	s.focusPath(n.path)
}

func (s *sidebar) expand() {
	n := s.current()
	if n == nil {
		return
	}
	if n.dir {
		if !n.expanded {
			s.toggle()
			return
		}
		if len(n.children) > 0 {
			s.focusPath(n.children[0].path)
		}
	}
}

func (s *sidebar) collapse() {
	n := s.current()
	if n == nil {
		return
	}
	if n.dir && n.expanded {
		s.toggle()
		return
	}
	parent := filepath.Dir(n.path)
	if parent == n.path || parent == s.root {
		return
	}
	if pn := findNode(s.rootNode, parent); pn != nil {
		s.focusPath(pn.path)
	}
}

func (s *sidebar) focusPath(path string) {
	for i, n := range s.visible {
		if n.path == path {
			s.cursor = i
			return
		}
	}
}

func (s *sidebar) resize(width, height int) {
	s.width, s.height = width, height
}

func (s *sidebar) view(w, h int, frame int) string {
	if w < 8 || h < 4 {
		return ""
	}
	s.width, s.height = w, h
	pad := lipgloss.NewStyle().Background(theme.BgPanel)
	rows := make([]string, 0, h)

	title := theme.SidebarTitle.Render(" FILES")
	rootName := filepath.Base(s.root)
	right := theme.SidebarDim.Render(ansi.Truncate(rootName, max(4, w-10), "…") + " ")
	rows = append(rows, spread(title, right, w, theme.BgPanel))
	rows = append(rows, theme.SidebarDivider.Render(strings.Repeat("─", w)))

	treeH := h - 4
	if treeH < 1 {
		treeH = 1
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+treeH {
		s.offset = s.cursor - treeH + 1
	}
	maxOffset := max(0, len(s.visible)-treeH)
	s.offset = clamp(s.offset, 0, maxOffset)

	treeRows := 0
	for i := s.offset; i < len(s.visible) && treeRows < treeH; i++ {
		n := s.visible[i]
		indent := strings.Repeat("  ", max(0, n.depth-1))
		glyph := "·"
		if n.dir {
			if n.expanded {
				glyph = "▾"
			} else {
				glyph = "▸"
			}
		}
		nameWidth := w - 3 - lipgloss.Width(indent)
		if nameWidth < 4 {
			nameWidth = 4
		}
		name := ansi.Truncate(n.name, nameWidth, "…")
		plain := " " + indent + glyph + " " + name
		if i == s.cursor {
			gap := max(0, w-lipgloss.Width(plain))
			if s.focused {
				rows = append(rows, theme.SelectedRow.Render(plain+strings.Repeat(" ", gap)))
			} else {
				row := lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Fg).Render(plain + strings.Repeat(" ", gap))
				rows = append(rows, row)
			}
		} else {
			glyphStyle := theme.SidebarDim
			if n.dir {
				glyphStyle = lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Blue)
			}
			nameStyle := lipgloss.NewStyle().Background(theme.BgPanel).Foreground(theme.Fg)
			if !n.dir && strings.HasSuffix(n.name, ".md") {
				nameStyle = nameStyle.Foreground(theme.Muted)
			}
			line := " " + indent + glyphStyle.Render(glyph) + " " + nameStyle.Render(name)
			gap := w - lipgloss.Width(line)
			rows = append(rows, line+pad.Render(strings.Repeat(" ", max(0, gap))))
		}
		treeRows++
	}
	for treeRows < treeH {
		rows = append(rows, pad.Render(strings.Repeat(" ", w)))
		treeRows++
	}

	footer := ""
	if s.focused {
		footer = theme.KeyHint.Render("a file · A dir · R ren · d del")
		if s.err != "" {
			footer = theme.ErrorText.Render(ansi.Truncate(s.err, w, "…"))
		}
	} else {
		if n := s.current(); n != nil {
			rel := files.Rel(s.root, n.path)
			footer = theme.SidebarDim.Render(" " + ansi.Truncate(rel, w-1, "…"))
		}
	}
	rows = append(rows, pad.Render(strings.Repeat(" ", w)))
	rows = append(rows, footer)
	return strings.Join(rows, "\n")
}
