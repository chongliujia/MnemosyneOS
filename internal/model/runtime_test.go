package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeGatewayGenerateTextAcceptsArrayContent(t *testing.T) {
	store := &ConfigStore{
		cfg: Config{
			Provider: "openai-compatible",
			Preset:   "custom",
			BaseURL:  "https://api.siliconflow.test/v1",
			APIKey:   "test-key",
			Conversation: ProfileConfig{
				Model:       "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens:   1200,
				Temperature: 0.2,
			},
			Routing: ProfileConfig{
				Model:     "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens: 220,
			},
			Skills: ProfileConfig{
				Model:       "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens:   640,
				Temperature: 0.2,
			},
		},
	}
	gateway := NewRuntimeGateway(store).(*RuntimeGateway)
	gateway.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			body := `{"model":"Pro/MiniMaxAI/MiniMax-M2.5","choices":[{"message":{"content":[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}]}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	resp, err := gateway.GenerateText(context.Background(), TextRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Profile:      ProfileConversation,
	})
	if err != nil {
		t.Fatalf("GenerateText returned error: %v", err)
	}
	if !strings.Contains(resp.Text, "第一段") || !strings.Contains(resp.Text, "第二段") {
		t.Fatalf("expected concatenated array content, got %q", resp.Text)
	}
}

func TestRuntimeGatewayGenerateTextReportsNonCompatibleBody(t *testing.T) {
	store := &ConfigStore{
		cfg: Config{
			Provider: "openai-compatible",
			Preset:   "custom",
			BaseURL:  "https://api.siliconflow.test/v1",
			APIKey:   "test-key",
			Conversation: ProfileConfig{
				Model:       "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens:   1200,
				Temperature: 0.2,
			},
			Routing: ProfileConfig{
				Model:     "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens: 220,
			},
			Skills: ProfileConfig{
				Model:       "Pro/MiniMaxAI/MiniMax-M2.5",
				MaxTokens:   640,
				Temperature: 0.2,
			},
		},
	}
	gateway := NewRuntimeGateway(store).(*RuntimeGateway)
	gateway.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`404`)),
			}, nil
		}),
	}

	_, err := gateway.GenerateText(context.Background(), TextRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Profile:      ProfileConversation,
	})
	if err == nil {
		t.Fatalf("expected compatibility error")
	}
	if !strings.Contains(err.Error(), "non-compatible response") {
		t.Fatalf("expected compatibility hint, got %v", err)
	}
}

func TestCompletionEndpointAcceptsFullPath(t *testing.T) {
	if got := completionEndpoint("https://api.siliconflow.cn/v1"); got != "https://api.siliconflow.cn/v1/chat/completions" {
		t.Fatalf("unexpected endpoint: %s", got)
	}
	if got := completionEndpoint("https://api.siliconflow.cn/v1/chat/completions"); got != "https://api.siliconflow.cn/v1/chat/completions" {
		t.Fatalf("unexpected full endpoint passthrough: %s", got)
	}
}

func TestConfigStorePersistsToDisk(t *testing.T) {
	runtimeRoot := t.TempDir()
	store, err := NewConfigStore(runtimeRoot)
	if err != nil {
		t.Fatalf("NewConfigStore returned error: %v", err)
	}
	cfg := Config{
		Provider: "openai-compatible",
		Preset:   "custom",
		BaseURL:  "https://persisted.example/v1",
		APIKey:   "persisted-key",
		Conversation: ProfileConfig{
			Model:     "conv-model",
			MaxTokens: 1200,
		},
		Routing: ProfileConfig{
			Model:     "route-model",
			MaxTokens: 200,
		},
		Skills: ProfileConfig{
			Model:     "skill-model",
			MaxTokens: 640,
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(runtimeRoot, "model", "config.json"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var stored Config
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if stored.APIKey != "" {
		t.Fatalf("expected default model config path to scrub API key, got %q", stored.APIKey)
	}
	if stored.BaseURL != "https://persisted.example/v1" || stored.Skills.Model != "skill-model" {
		t.Fatalf("unexpected persisted config: %+v", stored)
	}

	reloaded, err := NewConfigStore(runtimeRoot)
	if err != nil {
		t.Fatalf("NewConfigStore reload returned error: %v", err)
	}
	got := reloaded.Get()
	if got.BaseURL != "https://persisted.example/v1" || got.Skills.Model != "skill-model" {
		t.Fatalf("expected reloaded config from disk, got %+v", got)
	}
}

func TestConfigStorePrivatePathPersistsAPIKey(t *testing.T) {
	runtimeRoot := t.TempDir()
	configPath := filepath.Join(runtimeRoot, "model", "local.config.json")
	store, err := NewConfigStoreAtPath(runtimeRoot, configPath)
	if err != nil {
		t.Fatalf("NewConfigStoreAtPath returned error: %v", err)
	}
	cfg := Config{
		Provider: "openai",
		APIKey:   "private-key",
		Conversation: ProfileConfig{
			Model: "gpt-4o-mini",
		},
		Routing: ProfileConfig{
			Model: "gpt-4o-mini",
		},
		Skills: ProfileConfig{
			Model: "gpt-4o-mini",
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var stored Config
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if stored.APIKey != "private-key" {
		t.Fatalf("expected private config path to persist API key, got %q", stored.APIKey)
	}
}

func TestConfigStoreOverlaysProviderAPIKeyFromEnv(t *testing.T) {
	runtimeRoot := t.TempDir()
	t.Setenv("OPENAI_API_KEY", "env-key")
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "model"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	raw := []byte(`{
  "provider": "openai",
  "base_url": "https://api.openai.com/v1",
  "conversation": {"model": "gpt-4o-mini", "max_tokens": 1600},
  "routing": {"model": "gpt-4o-mini", "max_tokens": 220},
  "skills": {"model": "gpt-4o-mini", "max_tokens": 1200}
}`)
	if err := os.WriteFile(filepath.Join(runtimeRoot, "model", "config.json"), raw, 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	store, err := NewConfigStore(runtimeRoot)
	if err != nil {
		t.Fatalf("NewConfigStore returned error: %v", err)
	}
	if got := store.Get().APIKey; got != "env-key" {
		t.Fatalf("expected env API key overlay, got %q", got)
	}
}
