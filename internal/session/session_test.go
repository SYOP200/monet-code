package session

import (
	"path/filepath"
	"testing"

	"github.com/syop200/monet-code/internal/ollama"
)

func TestSaveListLoad(t *testing.T) {
	t.Setenv("MONET_CONFIG_DIR", t.TempDir())
	history := []ollama.Message{
		{Role: ollama.RoleUser, Content: "hello"},
		{Role: ollama.RoleAssistant, Content: "hi", Thinking: "brief thought"},
	}
	if _, err := Save("20260101-120000", "/tmp/project", "qwen3-coder:30b", history); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load("20260101-120000")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "qwen3-coder:30b" || len(loaded.History) != 2 {
		t.Fatalf("unexpected session: %+v", loaded)
	}
	if loaded.History[1].Thinking != "brief thought" {
		t.Fatalf("thinking was not persisted: %+v", loaded.History[1])
	}
	list := List()
	if len(list) != 1 || list[0].ID != "20260101-120000" {
		t.Fatalf("unexpected session list: %+v", list)
	}
	latest, err := Latest()
	if err != nil || latest.ID != "20260101-120000" {
		t.Fatalf("latest = %+v, err = %v", latest, err)
	}
}

func TestSavePrunesOldSessions(t *testing.T) {
	t.Setenv("MONET_CONFIG_DIR", t.TempDir())
	for i := 0; i < 52; i++ {
		id := "20260101-1200" + string(rune('A'+i%26)) + string(rune('A'+i/26))
		if _, err := Save(id, "/tmp/project", "m", []ollama.Message{{Role: ollama.RoleUser, Content: "x"}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(List()); got > 50 {
		t.Fatalf("pruned to %d sessions, want at most 50", got)
	}
}

func TestSessionMessages(t *testing.T) {
	t.Setenv("MONET_CONFIG_DIR", t.TempDir())
	if _, err := SendMessage("a", "b", "please review internal/ui"); err != nil {
		t.Fatal(err)
	}
	if _, err := SendMessage("a", "b", "second note"); err != nil {
		t.Fatal(err)
	}
	inbox, err := Inbox("b", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 || inbox[0].From != "a" || inbox[1].Text != "second note" {
		t.Fatalf("unexpected inbox: %+v", inbox)
	}
	testPath := filepath.Join(MessagesDir(), "b.jsonl")
	if testPath == "" {
		t.Fatal("messages dir missing")
	}
	if _, err := Inbox("b", true); err != nil {
		t.Fatal(err)
	}
	inbox, err = Inbox("b", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 0 {
		t.Fatalf("inbox was not cleared: %+v", inbox)
	}
	if _, err := SendMessage("a", "", "x"); err == nil {
		t.Fatal("expected empty recipient to fail")
	}
}
