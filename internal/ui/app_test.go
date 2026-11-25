package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSnapshotsFillTheirFrame(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	app := New(root, "qwen3.5:4b", "http://127.0.0.1:1")

	sizes := []struct{ w, h int }{{60, 18}, {80, 24}, {100, 30}, {120, 40}}
	for _, size := range sizes {
		for _, view := range []string{"chat", "editor", "models", "files", "help", "thinking", "message", "slash"} {
			out := app.SnapshotTab(view, size.w, size.h)
			lines := strings.Split(out, "\n")
			if len(lines) != size.h {
				t.Errorf("%s %dx%d: rendered %d rows", view, size.w, size.h, len(lines))
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != size.w {
					t.Errorf("%s %dx%d: row %d is %d columns", view, size.w, size.h, i, got)
				}
			}
		}
	}
}

func TestPromptOnlyOnFirstInputLine(t *testing.T) {
	c := newChatPane(80)
	view := promptLines(c.input.View())
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple input rows, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "❯") {
		t.Errorf("first line missing prompt: %q", lines[0])
	}
	for i := 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "❯") {
			t.Errorf("row %d repeats the prompt: %q", i, lines[i])
		}
	}
}
