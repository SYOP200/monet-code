// Package config reads and writes the shared Monet Code configuration.
//
// The Go TUI and the Python CLI both use the same JSON file so that
// `monet models use qwen2.5-coder:7b` is reflected the next time the TUI
// starts. Location: $MONET_CONFIG_DIR/config.json, falling back to
// ~/.config/monet/config.json (on every platform, so the two runtimes agree).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is the persisted user configuration.
type Config struct {
	Model           string   `json:"model"`
	RecentModels    []string `json:"recent_models,omitempty"`
	RecentDirs      []string `json:"recent_directories,omitempty"`
	EditorTabSize   int      `json:"editor_tab_size,omitempty"`
	OllamaURL       string   `json:"ollama_url,omitempty"`
	SystemPrompt    string   `json:"system_prompt,omitempty"`
	ShowThinking    bool     `json:"show_thinking"`
	OrganizeFolders bool     `json:"organize_folders,omitempty"`
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		Model:           "",
		EditorTabSize:   4,
		ShowThinking:    true,
		OrganizeFolders: true,
	}
}

// Dir returns the directory that holds the configuration file.
func Dir() string {
	if d := os.Getenv("MONET_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".monet"
	}
	return filepath.Join(home, ".config", "monet")
}

// Path returns the full path of config.json.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load reads the config file, returning defaults if it does not exist or is
// unreadable. Malformed files are backed up rather than silently overwritten.
func Load() Config {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		_ = os.WriteFile(Path()+".bak", data, 0o644)
		return Default()
	}
	if cfg.EditorTabSize <= 0 {
		cfg.EditorTabSize = 4
	}
	return cfg
}

// Save writes the config atomically.
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// SetModel records the active model and keeps a short most-recent list.
func (c *Config) SetModel(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c.Model = name
	recent := []string{name}
	for _, m := range c.RecentModels {
		if m != name && len(recent) < 8 {
			recent = append(recent, m)
		}
	}
	c.RecentModels = recent
}

// OllamaBaseURL resolves the Ollama endpoint from an explicit override,
// the config file, or OLLAMA_HOST.
func (c Config) OllamaBaseURL() string {
	raw := strings.TrimSpace(c.OllamaURL)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	if raw == "" {
		raw = "http://127.0.0.1:11434"
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}
