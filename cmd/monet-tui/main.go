// Command monet-tui starts the Monet Code terminal interface.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/syop200/monet-code/internal/ui"
)

func main() {
	var (
		dir       string
		model     string
		ollamaURL string
		version   bool
		snapshot  bool
		snapTab   string
		snapFile  string
		snapW     int
		snapH     int
	)
	flag.StringVar(&dir, "dir", "", "workspace directory (default: current directory)")
	flag.StringVar(&model, "model", "", "Ollama model to start with")
	flag.StringVar(&ollamaURL, "ollama-url", "", "Ollama base URL (default: config or OLLAMA_HOST)")
	flag.BoolVar(&version, "version", false, "print the version and exit")
	flag.BoolVar(&snapshot, "snapshot", false, "render one frame to stdout and exit (for testing)")
	flag.StringVar(&snapTab, "snapshot-tab", "chat", "view to render in snapshot mode: chat, editor, models, files, help, thinking, message, slash")
	flag.StringVar(&snapFile, "snapshot-file", "", "open this file in the editor for snapshot mode")
	flag.IntVar(&snapW, "width", 120, "snapshot width")
	flag.IntVar(&snapH, "height", 36, "snapshot height")
	flag.Parse()

	if version {
		fmt.Printf("monet-tui %s\n", ui.Version)
		return
	}

	root := dir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "monet:", err)
			os.Exit(1)
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "monet:", err)
		os.Exit(1)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "monet: %s is not a directory\n", abs)
		os.Exit(1)
	}

	app := ui.New(abs, model, ollamaURL)
	if snapshot {
		if snapFile != "" {
			if err := app.OpenForSnapshot(snapFile); err != nil {
				fmt.Fprintln(os.Stderr, "monet:", err)
				os.Exit(1)
			}
		}
		fmt.Print(app.SnapshotTab(snapTab, snapW, snapH) + "\n")
		return
	}

	restoreBackground := setTerminalBackground()
	defer restoreBackground()
	program := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		restoreBackground()
		fmt.Fprintln(os.Stderr, "monet:", err)
		os.Exit(1)
	}
}

// setTerminalBackground paints the terminal's default background black for the
// duration of the session. The app fills every cell itself, but the terminal
// still uses its own background for erased areas (for example while the alt
// screen is being set up or torn down). OSC 111 restores the user's colour.
func setTerminalBackground() func() {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return func() {}
	}
	fmt.Fprint(os.Stdout, "\x1b]11;#000000\x07")
	return func() {
		fmt.Fprint(os.Stdout, "\x1b]111\x07")
	}
}
