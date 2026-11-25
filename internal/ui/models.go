package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/syop200/monet-code/internal/catalog"
	"github.com/syop200/monet-code/internal/ollama"
	"github.com/syop200/monet-code/internal/theme"
)

type modelsPane struct {
	models  []ollama.Model
	catalog []catalog.Entry
	version string
	err     string
	loading bool

	section     int // 0 installed, 1 catalog
	cursor      [2]int
	offset      [2]int
	filter      string
	filtering   bool
	filterInput textinput.Model

	activeModel string

	pulling    bool
	pullModel  string
	pullStatus string
	pullPct    float64

	width, height int
}

func newModelsPane() *modelsPane {
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.Placeholder = "filter models…"
	fi.CharLimit = 64
	fi.PromptStyle = lipgloss.NewStyle().Foreground(theme.Blue).Background(theme.Bg)
	fi.TextStyle = lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Bg)
	fi.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Teal).Background(theme.Bg)
	fi.Width = 32
	return &modelsPane{
		catalog:     catalog.Sorted(),
		loading:     true,
		filterInput: fi,
	}
}

func (m *modelsPane) setLoaded(models []ollama.Model, version string, err error) {
	prev := ""
	if sel := m.selectedInstalled(); sel != nil {
		prev = sel.Name
	}
	m.loading = false
	m.version = version
	if err != nil {
		m.err = err.Error()
		m.models = nil
		return
	}
	m.err = ""
	m.models = models
	if prev != "" {
		for i, mod := range m.models {
			if mod.Name == prev {
				m.cursor[0] = i
			}
		}
	}
	m.clampCursors()
}

func (m *modelsPane) clampCursors() {
	inst := m.filteredInstalled()
	cat := m.filteredCatalog()
	if len(inst) == 0 {
		m.cursor[0] = 0
	} else {
		m.cursor[0] = clamp(m.cursor[0], 0, len(inst)-1)
	}
	if len(cat) == 0 {
		m.cursor[1] = 0
	} else {
		m.cursor[1] = clamp(m.cursor[1], 0, len(cat)-1)
	}
}

func (m *modelsPane) filteredInstalled() []ollama.Model {
	if m.filter == "" {
		return m.models
	}
	q := strings.ToLower(m.filter)
	var out []ollama.Model
	for _, mod := range m.models {
		if strings.Contains(strings.ToLower(mod.Name), q) || strings.Contains(strings.ToLower(mod.Details.Family), q) {
			out = append(out, mod)
		}
	}
	return out
}

func (m *modelsPane) filteredCatalog() []catalog.Entry {
	if m.filter == "" {
		return m.catalog
	}
	q := strings.ToLower(m.filter)
	var out []catalog.Entry
	for _, e := range m.catalog {
		hay := strings.ToLower(e.Name + " " + e.Title + " " + e.Description + " " + strings.Join(e.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

func (m *modelsPane) isInstalled(name string) bool {
	trim := func(s string) string { return strings.TrimSuffix(s, ":latest") }
	for _, mod := range m.models {
		if mod.Name == name || trim(mod.Name) == trim(name) {
			return true
		}
	}
	return false
}

func (m *modelsPane) selectedInstalled() *ollama.Model {
	list := m.filteredInstalled()
	m.clampCursors()
	if len(list) == 0 {
		return nil
	}
	return &list[m.cursor[0]]
}

func (m *modelsPane) selectedCatalog() *catalog.Entry {
	list := m.filteredCatalog()
	m.clampCursors()
	if len(list) == 0 {
		return nil
	}
	return &list[m.cursor[1]]
}

func (m *modelsPane) move(delta int) {
	list := m.filteredInstalled()
	if m.section == 1 {
		list = nil
	}
	var n int
	if m.section == 0 {
		n = len(list)
	} else {
		n = len(m.filteredCatalog())
	}
	if n == 0 {
		return
	}
	m.cursor[m.section] = clamp(m.cursor[m.section]+delta, 0, n-1)
}

func (m *modelsPane) syncFilter() {
	m.filter = strings.TrimSpace(m.filterInput.Value())
	m.clampCursors()
}

func (m *modelsPane) view(w, h int, frame int) string {
	if w < 30 || h < 10 {
		return fitBlock(theme.DimText.Render("terminal too small"), w, h, theme.Bg)
	}
	m.width, m.height = w, h
	extra := 0
	if m.pulling {
		extra++
	}
	if m.filtering {
		extra++
	}
	avail := h - 1 - extra
	instH := avail / 2
	if instH < 4 {
		instH = 4
	}
	catH := avail - instH

	header := theme.Accent.Render(" MODEL CATALOG") +
		theme.DimText.Render("  curated for coding + installed locally")
	right := ""
	if m.loading {
		right = theme.Accent.Render(spinnerFrame(frame) + " scanning ollama…")
	} else if m.err != "" {
		right = theme.ErrorText.Render("offline")
	} else {
		right = theme.DimText.Render(fmt.Sprintf("ollama %s · %s", m.version, plural(len(m.models), "model", "models")))
	}
	rows := []string{spread(header, right+" ", w, theme.Bg)}

	if m.pulling {
		rows = append(rows, " "+m.progressLine(w))
	}
	if m.filtering {
		m.filterInput.Width = max(16, w-8)
		rows = append(rows, " "+m.filterInput.View())
	}

	rows = append(rows, frameBox("INSTALLED", "", m.installedContent(w, instH, frame), w, instH, m.sectionBorder(0), theme.Bg))
	rows = append(rows, frameBox("CATALOG", "", m.catalogContent(w, catH), w, catH, m.sectionBorder(1), theme.Bg))
	return strings.Join(rows, "\n")
}

func (m *modelsPane) sectionBorder(section int) lipgloss.Color {
	if m.section == section {
		return theme.Muted
	}
	return theme.Border
}

func (m *modelsPane) progressLine(w int) string {
	pct := clamp(int(m.pullPct*100), 0, 100)
	barW := clamp(w-46, 10, 36)
	filled := barW * pct / 100
	bar := theme.Accent.Render(strings.Repeat("█", filled)) + theme.DimText.Render(strings.Repeat("░", barW-filled))
	status := m.pullStatus
	if len(status) > 24 {
		status = status[:24]
	}
	label := fmt.Sprintf(" pulling %s  %s %3d%%  %s", m.pullModel, bar, pct, status)
	return theme.MutedText.Render(ansi.Truncate(label, w, "…"))
}

func (m *modelsPane) installedContent(w, h int, frame int) string {
	inner := w - 4
	contentH := h - 2
	if m.loading && len(m.models) == 0 {
		return theme.DimText.Render("  " + spinnerFrame(frame) + " reading local models…")
	}
	if m.err != "" {
		return theme.ErrorText.Render("  " + ansi.Truncate(m.err, inner-2, "…"))
	}
	list := m.filteredInstalled()
	if len(list) == 0 {
		if m.filter != "" {
			return theme.DimText.Render("  no installed models match “" + m.filter + "”")
		}
		return theme.DimText.Render("  no models installed yet — tab to the catalog and press enter to pull one")
	}
	if m.cursor[0] < m.offset[0] {
		m.offset[0] = m.cursor[0]
	}
	if m.cursor[0] >= m.offset[0]+contentH {
		m.offset[0] = m.cursor[0] - contentH + 1
	}
	m.offset[0] = clamp(m.offset[0], 0, max(0, len(list)-contentH))

	var rows []string
	for i := m.offset[0]; i < len(list) && len(rows) < contentH; i++ {
		mod := list[i]
		meta := strings.TrimSpace(strings.Join([]string{
			ollama.DisplaySize(mod.Size),
			mod.Details.ParameterSize,
			mod.Details.QuantizationLevel,
		}, "  ·  "))
		nameW := inner - lipgloss.Width(meta) - 5
		if nameW < 8 {
			nameW = 8
		}
		name := ansi.Truncate(mod.Name, nameW, "…")
		marker := " "
		if mod.Name == m.activeModel {
			marker = "●"
		}
		left := " " + marker + " " + name
		gap := inner - lipgloss.Width(left) - lipgloss.Width(meta) - 1
		plain := left + strings.Repeat(" ", max(1, gap)) + meta + " "
		if i == m.cursor[0] {
			style := theme.SelectedRow
			if m.section != 0 {
				style = lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Fg)
			}
			rows = append(rows, style.Render(fitPlain(plain, inner)))
		} else {
			line := " " + theme.DimText.Render(marker) + " " + theme.Base.Render(name)
			pad := inner - lipgloss.Width(line) - lipgloss.Width(meta) - 1
			rows = append(rows, line+strings.Repeat(" ", max(1, pad))+theme.MutedText.Render(meta)+" ")
		}
	}
	return strings.Join(rows, "\n")
}

func (m *modelsPane) catalogContent(w, h int) string {
	inner := w - 4
	contentH := h - 2
	list := m.filteredCatalog()
	if len(list) == 0 {
		return theme.DimText.Render("  nothing matches “" + m.filter + "”")
	}
	perItem := 2
	visible := max(1, contentH/perItem)
	if m.cursor[1] < m.offset[1] {
		m.offset[1] = m.cursor[1]
	}
	if m.cursor[1] >= m.offset[1]+visible {
		m.offset[1] = m.cursor[1] - visible + 1
	}
	m.offset[1] = clamp(m.offset[1], 0, max(0, len(list)-visible))

	var rows []string
	for i := m.offset[1]; i < len(list) && len(rows) < contentH; i++ {
		e := list[i]
		badge := ""
		if m.isInstalled(e.Name) {
			badge = " ✓"
		}
		size := fmt.Sprintf("~%.1f GB", e.SizeGB)
		nameW := inner - lipgloss.Width(size) - lipgloss.Width(badge) - 5
		if nameW < 8 {
			nameW = 8
		}
		name := ansi.Truncate(e.Name, nameW, "…")
		title := " " + name + badge
		desc := "   " + ansi.Truncate(e.Description, inner-4, "…")
		if i == m.cursor[1] {
			style := theme.SelectedRow
			if m.section != 1 {
				style = lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Fg)
			}
			gap := inner - lipgloss.Width(title) - lipgloss.Width(size) - 1
			rows = append(rows, style.Render(fitPlain(title+strings.Repeat(" ", max(1, gap))+size+" ", inner)))
			rows = append(rows, lipgloss.NewStyle().Background(theme.BgRaised).Foreground(theme.Muted).Render(fitPlain(desc, inner)))
		} else {
			line := " " + theme.Base.Render(name) + theme.OKText.Render(badge)
			pad := inner - lipgloss.Width(line) - lipgloss.Width(size) - 1
			rows = append(rows, line+strings.Repeat(" ", max(1, pad))+theme.DimText.Render(size)+" ")
			rows = append(rows, theme.DimText.Render(desc))
		}
	}
	return strings.Join(rows, "\n")
}

func fitPlain(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}
