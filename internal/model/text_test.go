package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDeepSeekGatewayStreamText(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload["stream"] != true {
				t.Fatalf("expected stream=true payload, got %#v", payload["stream"])
			}
			body := strings.NewReader(
				"data: {\"model\":\"deepseek-chat\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
					"data: {\"model\":\"deepseek-chat\",\"choices\":[{\"delta\":{\"content\":\" world\"}}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n" +
					"data: [DONE]\n\n",
			)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(body),
			}, nil
		}),
	}

	gateway := &DeepSeekGateway{
		baseURL:    "https://api.deepseek.test",
		apiKey:     "test-key",
		model:      "deepseek-chat",
		httpClient: client,
	}

	var chunks []string
	resp, err := gateway.StreamText(context.Background(), TextRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
		MaxTokens:    64,
		Temperature:  0.1,
	}, func(delta TextDelta) error {
		chunks = append(chunks, delta.Text)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamText returned error: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "Hello world" {
		t.Fatalf("expected streamed chunks to join into %q, got %q", "Hello world", got)
	}
	if resp.Text != "Hello world" {
		t.Fatalf("expected response text %q, got %q", "Hello world", resp.Text)
	}
	if resp.TotalTokens != 7 {
		t.Fatalf("expected total tokens 7, got %d", resp.TotalTokens)
	}
}
