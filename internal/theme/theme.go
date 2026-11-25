// Package theme defines the Monet Code palette and shared lipgloss styles.
//
// Everything is painted on a GitHub-dark style canvas (#0D1117) with a
// Nord-inspired frost palette: Frost Blue for body text, Glacier Teal and
// Arctic Blue for accents.
package theme

import "github.com/charmbracelet/lipgloss"

// Core palette.
var (
	// Bg is the app canvas.
	Bg = lipgloss.Color("#0D1117")
	// BgPanel is used for the chrome (header, sidebar, status bar).
	BgPanel = lipgloss.Color("#0F141B")
	// BgRaised is used for selected rows and input fields.
	BgRaised = lipgloss.Color("#16202B")
	// BgCode is used for inline/fenced code.
	BgCode = lipgloss.Color("#0A0F16")

	// Fg is the primary frost-blue foreground.
	Fg = lipgloss.Color("#88C0D0")
	// Teal is glacier teal, the primary accent.
	Teal = lipgloss.Color("#8FBCBB")
	// Blue is arctic blue, the secondary accent.
	Blue = lipgloss.Color("#81A1C1")
	// Muted is a desaturated blue for secondary text.
	Muted = lipgloss.Color("#5E81AC")
	// Dim is used for hints and inert chrome.
	Dim = lipgloss.Color("#4C5A68")
	// Border is the panel border color.
	Border = lipgloss.Color("#22303D")

	// Semantic colors, all muted to fit the frost palette.
	Red    = lipgloss.Color("#BF616A")
	Green  = lipgloss.Color("#A3BE8C")
	Yellow = lipgloss.Color("#EBCB8B")
	Purple = lipgloss.Color("#B48EAD")
)

// Styles shared across views. Every style carries an explicit background so
// resets inside nested renders never punch holes in the canvas.
var (
	Base = lipgloss.NewStyle().Foreground(Fg).Background(Bg)

	Title = lipgloss.NewStyle().
		Foreground(Teal).
		Background(BgPanel).
		Bold(true)

	Subtitle = lipgloss.NewStyle().
			Foreground(Dim).
			Background(BgPanel)

	Accent    = lipgloss.NewStyle().Foreground(Teal).Background(Bg)
	BlueText  = lipgloss.NewStyle().Foreground(Blue).Background(Bg)
	MutedText = lipgloss.NewStyle().Foreground(Muted).Background(Bg)
	DimText   = lipgloss.NewStyle().Foreground(Dim).Background(Bg)

	ErrorText = lipgloss.NewStyle().Foreground(Red).Background(Bg)
	WarnText  = lipgloss.NewStyle().Foreground(Yellow).Background(Bg)
	OKText    = lipgloss.NewStyle().Foreground(Green).Background(Bg)

	Key = lipgloss.NewStyle().
		Foreground(Blue).
		Background(BgPanel).
		Bold(true)
	KeyHint = lipgloss.NewStyle().
		Foreground(Dim).
		Background(BgPanel)

	Header = lipgloss.NewStyle().
		Background(BgPanel).
		Foreground(Fg)
	HeaderTitle = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Teal).
			Bold(true)
	HeaderDim = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Dim)

	Sidebar = lipgloss.NewStyle().
		Background(BgPanel).
		Foreground(Fg)
	SidebarTitle = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Blue).
			Bold(true)
	SidebarDim = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Dim)

	StatusBar = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Dim)
	StatusText = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Muted)

	TabActive = lipgloss.NewStyle().
			Background(BgRaised).
			Foreground(Teal).
			Bold(true)
	TabInactive = lipgloss.NewStyle().
			Background(BgPanel).
			Foreground(Dim)
	TabBadge = lipgloss.NewStyle().
			Background(BgRaised).
			Foreground(Blue)

	SelectedRow = lipgloss.NewStyle().
			Background(BgRaised).
			Foreground(Teal).
			Bold(true)
	HoverRow = lipgloss.NewStyle().
			Background(Bg).
			Foreground(Fg)

	UserLabel = lipgloss.NewStyle().
			Foreground(Blue).
			Background(Bg).
			Bold(true)
	AssistantLabel = lipgloss.NewStyle().
			Foreground(Teal).
			Background(Bg).
			Bold(true)
	ToolLabel = lipgloss.NewStyle().
			Foreground(Purple).
			Background(Bg)
	Notice = lipgloss.NewStyle().
		Foreground(Dim).
		Background(Bg).
		Italic(true)

	InlineCode = lipgloss.NewStyle().
			Foreground(Teal).
			Background(BgCode)
	CodeBlock = lipgloss.NewStyle().
			Foreground(Fg).
			Background(BgCode).
			Border(lipgloss.NormalBorder()).
			BorderForeground(Border).
			Padding(0, 1)

	Heading = lipgloss.NewStyle().
		Foreground(Teal).
		Background(Bg).
		Bold(true)
	Heading2 = lipgloss.NewStyle().
			Foreground(Blue).
			Background(Bg).
			Bold(true)

	// Panel is the standard bordered container.
	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Background(Bg).
		Padding(0, 1)
	PanelTitle = lipgloss.NewStyle().
			Foreground(Blue).
			Background(Bg).
			Bold(true)

	// Input is the chat/editor frame.
	Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Background(Bg)
	InputFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Background(Bg)

	ModalBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Teal).
			Background(BgPanel).
			Padding(1, 2)
	ModalTitle = lipgloss.NewStyle().
			Foreground(Teal).
			Background(BgPanel).
			Bold(true)
	ModalText = lipgloss.NewStyle().
			Foreground(Fg).
			Background(BgPanel)
	ModalDim = lipgloss.NewStyle().
			Foreground(Dim).
			Background(BgPanel)

	// Scrollbar / dividers.
	Divider = lipgloss.NewStyle().
		Foreground(Border).
		Background(Bg)
	SidebarDivider = lipgloss.NewStyle().
			Foreground(Border).
			Background(BgPanel)
)

// BG returns a style that paints n spaces with the given background.
func BG(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(c)
}
