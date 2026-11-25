// Package catalog embeds Monet Code's curated catalog of recommended Ollama
// models. The same internal/catalog/models.json file is read by the Python CLI.
package catalog

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed models.json
var raw []byte

// Entry is a single recommended model.
type Entry struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Family      string   `json:"family"`
	Params      string   `json:"params"`
	SizeGB      float64  `json:"size_gb"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
}

type file struct {
	Updated string  `json:"updated"`
	Models  []Entry `json:"models"`
}

// All returns the curated catalog, ordered as authored.
func All() []Entry {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	return f.Models
}

// Sorted returns the catalog alphabetically by name.
func Sorted() []Entry {
	entries := All()
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries
}

// Find looks up an entry by exact model name (tag-insensitive for ":latest").
func Find(name string) (Entry, bool) {
	for _, e := range All() {
		if e.Name == name || strings.TrimSuffix(e.Name, ":latest") == strings.TrimSuffix(name, ":latest") {
			return e, true
		}
	}
	return Entry{}, false
}

// Tags renders the tag list as a compact string.
func (e Entry) TagString() string { return strings.Join(e.Tags, " · ") }
