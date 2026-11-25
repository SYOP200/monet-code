package catalog

import "testing"

func TestAllIsNotEmpty(t *testing.T) {
	entries := All()
	if len(entries) < 10 {
		t.Fatalf("catalog has %d entries", len(entries))
	}
	for _, e := range entries {
		if e.Name == "" || e.Title == "" || e.Description == "" {
			t.Errorf("incomplete entry: %+v", e)
		}
	}
}

func TestFindIsTagInsensitive(t *testing.T) {
	if _, ok := Find("qwen3-coder:30b"); !ok {
		t.Fatal("expected qwen3-coder:30b in the catalog")
	}
	entry := Entry{Name: "example:latest", Title: "Example", Tags: []string{"code", "test"}}
	if entry.TagString() != "code · test" {
		t.Errorf("TagString = %q", entry.TagString())
	}
}

func TestSortedIsAlphabetical(t *testing.T) {
	entries := Sorted()
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Name > entries[i].Name {
			t.Fatalf("not sorted at %d: %q > %q", i, entries[i-1].Name, entries[i].Name)
		}
	}
}
