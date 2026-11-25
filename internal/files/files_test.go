package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	for name, want := range map[string]bool{
		".git":                  true,
		"node_modules":          true,
		".venv":                 true,
		"monet_code.egg-info":   true,
		"some_package.egg-info": true,
		"internal":              false,
		"src":                   false,
	} {
		if got := ShouldSkipDir(name); got != want {
			t.Errorf("ShouldSkipDir(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCategoryFor(t *testing.T) {
	cases := map[string]string{
		"notes.md":        "Documents",
		"photo.HEIC":      "Images",
		"clip.mov":        "Video",
		"track.flac":      "Audio",
		"backup.tar.gz":   "Archives",
		"main.go":         "Code",
		"rows.csv":        "Data",
		"LICENSE":         "Other",
		"weird.extension": "Other",
	}
	for name, want := range cases {
		if got := CategoryFor(name).Name; got != want {
			t.Errorf("CategoryFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAnalyzeIgnoresHiddenAndDirectories(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "notes.md"), "hello")
	mustWrite(t, filepath.Join(root, "photo.png"), "png")
	mustWrite(t, filepath.Join(root, ".secret"), "hidden")
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FileCount != 2 {
		t.Fatalf("FileCount = %d, want 2", rep.FileCount)
	}
	if len(rep.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2", len(rep.Groups))
	}
	if rep.Groups[0].Category.Name != "Documents" || rep.Groups[0].Files[0] != "notes.md" {
		t.Errorf("unexpected first group: %+v", rep.Groups[0])
	}
}

func TestPlanIsSideEffectFreeAndApplyMoves(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.md"), "doc")
	mustWrite(t, filepath.Join(root, "b.py"), "print(1)")
	mustWrite(t, filepath.Join(root, ".env"), "secret")

	rep, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	moves, err := Plan(rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 2 {
		t.Fatalf("len(moves) = %d, want 2", len(moves))
	}
	for _, dir := range []string{"Documents", "Code"} {
		if _, err := os.Stat(filepath.Join(root, dir)); !os.IsNotExist(err) {
			t.Fatalf("Plan created %s before Apply", dir)
		}
	}
	for _, m := range moves {
		if m.RelTo == "" || m.Category == "" {
			t.Errorf("incomplete move: %+v", m)
		}
	}

	moved, errs := Apply(moves)
	if moved != 2 || len(errs) != 0 {
		t.Fatalf("Apply = %d moved, %v", moved, errs)
	}
	mustExist(t, filepath.Join(root, "Documents", "a.md"))
	mustExist(t, filepath.Join(root, "Code", "b.py"))
	mustExist(t, filepath.Join(root, ".env"))
}

func TestPlanAvoidsCollisions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "notes.md"), "loose")
	mustWrite(t, filepath.Join(root, "Documents", "notes.md"), "already filed")

	rep, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	moves, err := Plan(rep)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 1 {
		t.Fatalf("len(moves) = %d, want 1", len(moves))
	}
	if moves[0].RelTo != filepath.ToSlash(filepath.Join("Documents", "notes (1).md")) {
		t.Errorf("RelTo = %q", moves[0].RelTo)
	}
}

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()
	if _, err := SafeJoin(root, "a/b.txt"); err != nil {
		t.Errorf("inside path rejected: %v", err)
	}
	if _, err := SafeJoin(root, "."); err != nil {
		t.Errorf("root rejected: %v", err)
	}
	for _, bad := range []string{"../outside", "a/../../outside", "/etc/passwd"} {
		if _, err := SafeJoin(root, bad); err == nil {
			t.Errorf("SafeJoin(%q) should fail", bad)
		}
	}
}

func TestReadTextRejectsBinary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "binary.dat")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadText(path, 1<<20); err == nil {
		t.Fatal("expected binary file to be rejected")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}
