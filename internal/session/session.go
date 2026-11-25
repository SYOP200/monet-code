// Package session persists Monet Code conversations so a TUI restart can
// resume where the user left off.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/syop200/monet-code/internal/config"
	"github.com/syop200/monet-code/internal/ollama"
)

// Session is one saved conversation.
type Session struct {
	ID      string           `json:"id"`
	Created time.Time        `json:"created"`
	Updated time.Time        `json:"updated"`
	Root    string           `json:"root"`
	Model   string           `json:"model"`
	History []ollama.Message `json:"history"`
}

// Dir returns the sessions directory next to the shared config.
func Dir() string { return filepath.Join(config.Dir(), "sessions") }

// NewID returns a sortable session identifier.
func NewID() string { return time.Now().UTC().Format("20060102-150405") }

// Save writes a session atomically and prunes old sessions.
func Save(id, root, model string, history []ollama.Message) (Session, error) {
	if id == "" {
		id = NewID()
	}
	id = filepath.Base(strings.TrimSpace(id))
	now := time.Now().UTC()
	s := Session{ID: id, Created: now, Updated: now, Root: root, Model: model}
	s.History = make([]ollama.Message, len(history))
	copy(s.History, history)
	if old, err := Load(id); err == nil && !old.Created.IsZero() {
		s.Created = old.Created
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return s, err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return s, err
	}
	path := filepath.Join(Dir(), id+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return s, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return s, err
	}
	prune(50)
	return s, nil
}

// List returns saved sessions newest first.
func List() []Session {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		return nil
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// Load reads one session by id.
func Load(id string) (Session, error) {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." {
		return Session{}, fmt.Errorf("session id is required")
	}
	data, err := os.ReadFile(filepath.Join(Dir(), id+".json"))
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Latest returns the most recently updated session.
func Latest() (Session, error) {
	list := List()
	if len(list) == 0 {
		return Session{}, fmt.Errorf("no saved sessions yet")
	}
	return list[0], nil
}

func prune(keep int) {
	list := List()
	for i := keep; i < len(list); i++ {
		_ = os.Remove(filepath.Join(Dir(), list[i].ID+".json"))
	}
}

// Message is a note passed between two running Monet sessions.
type Message struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Text string    `json:"text"`
	Time time.Time `json:"time"`
}

// MessagesDir stores per-session inboxes.
func MessagesDir() string { return filepath.Join(config.Dir(), "messages") }

// SendMessage appends a note to another session's inbox.
func SendMessage(from, to, text string) (Message, error) {
	to = filepath.Base(strings.TrimSpace(to))
	text = strings.TrimSpace(text)
	if to == "" || to == "." || text == "" {
		return Message{}, fmt.Errorf("usage: /msg <session> <text>")
	}
	msg := Message{From: from, To: to, Text: text, Time: time.Now().UTC()}
	if err := os.MkdirAll(MessagesDir(), 0o755); err != nil {
		return msg, err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return msg, err
	}
	path := filepath.Join(MessagesDir(), to+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return msg, err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return msg, err
	}
	if err := f.Close(); err != nil {
		return msg, err
	}
	return msg, nil
}

// Inbox reads pending notes for a session and removes them when clear is true.
func Inbox(id string, clear bool) ([]Message, error) {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." {
		return nil, fmt.Errorf("session id is required")
	}
	path := filepath.Join(MessagesDir(), id+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Message
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m Message
		if err := json.Unmarshal([]byte(line), &m); err == nil && strings.TrimSpace(m.Text) != "" {
			out = append(out, m)
		}
	}
	if clear {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return out, err
		}
	}
	return out, nil
}
