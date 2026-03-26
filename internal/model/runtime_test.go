package model

import (
	"context"
	"io"
	"net/http"
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
