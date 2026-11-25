package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MONET_CONFIG_DIR", dir)
	if got := Path(); got != filepath.Join(dir, "config.json") {
		t.Errorf("Path() = %q", got)
	}
	cfg := Load()
	if cfg.EditorTabSize != 4 || !cfg.ShowThinking || cfg.Model != "" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MONET_CONFIG_DIR", dir)
	cfg := Load()
	cfg.SetModel("qwen2.5-coder:7b")
	cfg.SetModel("qwen3.5:4b")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	reloaded := Load()
	if reloaded.Model != "qwen3.5:4b" {
		t.Errorf("Model = %q", reloaded.Model)
	}
	if len(reloaded.RecentModels) != 2 || reloaded.RecentModels[0] != "qwen3.5:4b" {
		t.Errorf("RecentModels = %v", reloaded.RecentModels)
	}
}

func TestOllamaBaseURL(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	cfg := Default()
	if got := cfg.OllamaBaseURL(); got != "http://127.0.0.1:11434" {
		t.Errorf("default URL = %q", got)
	}
	cfg.OllamaURL = "localhost:9999/"
	if got := cfg.OllamaBaseURL(); got != "http://localhost:9999" {
		t.Errorf("normalized URL = %q", got)
	}
	t.Setenv("OLLAMA_HOST", "0.0.0.0:1234")
	if got := (Default()).OllamaBaseURL(); got != "http://0.0.0.0:1234" {
		t.Errorf("env URL = %q", got)
	}
}

func TestMalformedConfigFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MONET_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.EditorTabSize != 4 {
		t.Errorf("fallback config = %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json.bak")); err != nil {
		t.Errorf("expected a backup of the broken config: %v", err)
	}
}
