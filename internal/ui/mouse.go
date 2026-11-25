package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type hitbox struct {
	x0, y0, x1, y1 int
}

type tabHit struct {
	hitbox
	tab tab
}

func (h hitbox) contains(x, y int) bool {
	return x >= h.x0 && x <= h.x1 && y >= h.y0 && y <= h.y1
}

// handleMouse implements tab clicks, transcript scrolling, sidebar clicks,
// thinking disclosure and slash-command clicks.
func (a *App) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
		return a.scrollWheel(-3, msg.X)
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown {
		return a.scrollWheel(3, msg.X)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if a.modal.kind != modalNone {
		return nil
	}
	x, y := msg.X, msg.Y

	// Header tabs.
	for _, h := range a.tabHits {
		if h.contains(x, y) {
			return a.switchTab(h.tab)
		}
	}

	// Slash command popup.
	if a.slash.visible && len(a.slash.items) > 0 {
		mainX := a.sidebarW
		if a.sidebarW > 0 {
			mainX++
		}
		top := 2 + a.chat.menuTop + 1
		if x > mainX && x < mainX+a.mainW-1 && y >= top && y < top+a.slashVisibleCount() {
			idx := a.slash.offset + (y - top)
			if idx >= 0 && idx < len(a.slash.items) {
				a.slash.sel = idx
				return a.acceptSlash()
			}
		}
	}

	// Sidebar file tree.
	bodyH := max(6, a.height-3)
	if a.sidebarW > 0 && x < a.sidebarW && y >= 2 && y < 2+bodyH {
		row := y - 4 + a.sidebar.offset
		if row >= 0 && row < len(a.sidebar.visible) {
			a.sidebarFocused = true
			a.sidebar.cursor = row
			n := a.sidebar.current()
			if n != nil {
				if n.dir {
					a.sidebar.toggle()
					return a.refocus()
				}
				return a.openFile(n.path)
			}
		}
	}

	// Model rows select on click.
	if a.active == tabModels {
		if section, row, ok := a.models.rowAt(y - 2); ok {
			a.models.selectRow(section, row)
			return nil
		}
	}

	// File-organizer preview rows select on click.
	if a.active == tabFiles {
		if row, ok := a.org.rowAt(y - 2); ok {
			a.org.cursor = row
			return nil
		}
	}

	// Chat transcript: clicking a thinking marker opens the reasoning trace.
	if a.active == tabChat && y >= 2 && y < 2+bodyH {
		if y-2 < a.chat.viewport.Height {
			line := a.chat.viewport.YOffset + (y - 2)
			if m := a.chat.messageAt(line); m != nil {
				if m.thinking != "" || (m.role == "assistant" && !m.complete) {
					return a.openThinking(m.thinking)
				}
			}
		}
	}
	return nil
}

func (a *App) scrollWheel(delta, x int) tea.Cmd {
	if a.sidebarFocused && a.sidebarW > 0 && x < a.sidebarW {
		a.sidebar.move(delta)
		return nil
	}
	switch a.active {
	case tabChat:
		a.chat.scrollBy(delta)
	case tabModels:
		a.models.move(delta)
	case tabFiles:
		a.org.move(delta)
	}
	return nil
}
