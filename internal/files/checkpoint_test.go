package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointRestoresExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Snapshot(root, "main.go", "write_file")
	if err != nil {
		t.Fatal(err)
	}
	if !cp.Existed {
		t.Fatal("expected the checkpoint to record an existing file")
	}
	if err := os.WriteFile(path, []byte("package broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Rewind(root, cp.ID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("restored %q, want original content", data)
	}
}

func TestCheckpointRemovesCreatedFile(t *testing.T) {
	root := t.TempDir()
	cp, err := Snapshot(root, "new/file.txt", "write_file")
	if err != nil {
		t.Fatal(err)
	}
	if cp.Existed {
		t.Fatal("expected the checkpoint to record a missing file")
	}
	path := filepath.Join(root, "new", "file.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RewindLast(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected rewound creation to be removed, stat err = %v", err)
	}
}

func TestCheckpointRefusesMonetState(t *testing.T) {
	root := t.TempDir()
	if _, err := Snapshot(root, ".monet/memory.md", "write_file"); err == nil {
		t.Fatal("expected .monet to be refused")
	}
	if _, err := Snapshot(root, "../outside.txt", "write_file"); err == nil {
		t.Fatal("expected an escaping path to be refused")
	}
	list, err := ListCheckpoints(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no checkpoints, got %d", len(list))
	}
}

func TestSafeJoinContainment(t *testing.T) {
	root := t.TempDir()
	if _, err := SafeJoin(root, "a/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeJoin(root, "../escape.txt"); err == nil {
		t.Fatal("expected ../ to escape and be refused")
	}
	if _, err := SafeJoin(root, filepath.Join(root, "ok.txt")); err != nil {
		t.Fatalf("absolute path inside root should be allowed: %v", err)
	}
}

func TestCheckpointLabelsAreVisible(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Snapshot(root, "notes.md", "write_file"); err != nil {
		t.Fatal(err)
	}
	list, err := ListCheckpoints(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Rel != "notes.md" || list[0].Label != "write_file" {
		t.Fatalf("unexpected checkpoint list: %+v", list)
	}
	if list[0].ID == "" {
		t.Fatalf("checkpoint id missing: %+v", list[0])
	}
}
