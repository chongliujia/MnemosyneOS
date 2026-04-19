package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileStoreSaveListApplyDelete(t *testing.T) {
	root := t.TempDir()
	store, err := NewProfileStore(root)
	if err != nil {
		t.Fatalf("NewProfileStore: %v", err)
	}
	cfg := Config{
		Provider: "openai-compatible",
		BaseURL:  "https://example.test/v1",
		APIKey:   "secret-key",
		Conversation: ProfileConfig{
			Model:       "m-conv",
			MaxTokens:   800,
			Temperature: 0.2,
		},
		Routing: ProfileConfig{
			Model:     "m-route",
			MaxTokens: 200,
		},
		Skills: ProfileConfig{
			Model:       "m-skill",
			MaxTokens:   600,
			Temperature: 0.2,
		},
	}
	if err := store.Save("work", cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	names := store.ListNames()
	if len(names) != 1 || names[0] != "work" {
		t.Fatalf("ListNames: %#v", names)
	}
	got, ok := store.Get("work")
	if !ok || got.BaseURL != "https://example.test/v1" || got.Routing.Model != "m-route" {
		t.Fatalf("Get: ok=%v cfg=%+v", ok, got)
	}
	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.Get("work"); ok {
		t.Fatalf("expected profile removed")
	}
	raw, err := os.ReadFile(filepath.Join(root, "model", "profiles.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected profiles file to exist")
	}
}

func TestProfileStoreRejectInvalidName(t *testing.T) {
	root := t.TempDir()
	store, err := NewProfileStore(root)
	if err != nil {
		t.Fatalf("NewProfileStore: %v", err)
	}
	cfg := Config{
		Provider: "openai-compatible",
		BaseURL:  "https://example.test/v1",
		APIKey:   "k",
		Conversation: ProfileConfig{
			Model:     "m",
			MaxTokens: 100,
		},
		Routing: ProfileConfig{
			Model:     "m",
			MaxTokens: 50,
		},
		Skills: ProfileConfig{
			Model:     "m",
			MaxTokens: 80,
		},
	}
	if err := store.Save("bad name", cfg); err == nil {
		t.Fatalf("expected error for invalid profile name")
	}
}
