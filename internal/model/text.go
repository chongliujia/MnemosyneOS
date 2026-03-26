package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type TextGateway interface {
	GenerateText(ctx context.Context, req TextRequest) (TextResponse, error)
	StreamText(ctx context.Context, req TextRequest, onDelta func(TextDelta) error) (TextResponse, error)
}

type TextRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
	Profile      string
}

type TextResponse struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Text          string `json:"text"`
	InputTokens   int    `json:"input_tokens,omitempty"`
	OutputTokens  int    `json:"output_tokens,omitempty"`
	TotalTokens   int    `json:"total_tokens,omitempty"`
	LatencyMillis int64  `json:"latency_ms,omitempty"`
}

type TextDelta struct {
	Text string `json:"text"`
}

type DeepSeekGateway struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewTextGatewayFromEnv() TextGateway {
	provider := strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_PROVIDER"))
	switch strings.ToLower(provider) {
	case "", "none":
		return nil
	case "deepseek":
		apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
		if apiKey == "" {
			return nil
		}
		baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "/")
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		modelName := strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_NAME"))
		if modelName == "" {
			modelName = "deepseek-chat"
		}
		return &DeepSeekGateway{
			baseURL: baseURL,
			apiKey:  apiKey,
			model:   modelName,
			httpClient: &http.Client{
				Timeout: 45 * time.Second,
			},
		}
	default:
		return nil
	}
}

func (g *DeepSeekGateway) GenerateText(ctx context.Context, req TextRequest) (TextResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}

	payload := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"stream":      false,
		"max_tokens":  maxTokens,
		"temperature": req.Temperature,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TextResponse{}, err
	}

	started := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return TextResponse{}, err
	}
	defer resp.Body.Close()

	var raw struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return TextResponse{}, err
	}
	if resp.StatusCode >= 400 {
		if raw.Error != nil && raw.Error.Message != "" {
			return TextResponse{}, fmt.Errorf("deepseek API error: %s", raw.Error.Message)
		}
		return TextResponse{}, fmt.Errorf("deepseek API returned status %d", resp.StatusCode)
	}
	if len(raw.Choices) == 0 {
		return TextResponse{}, fmt.Errorf("deepseek API returned no choices")
	}

	return TextResponse{
		Provider:      "deepseek",
		Model:         firstNonEmpty(raw.Model, g.model),
		Text:          strings.TrimSpace(raw.Choices[0].Message.Content),
		InputTokens:   raw.Usage.PromptTokens,
		OutputTokens:  raw.Usage.CompletionTokens,
		TotalTokens:   raw.Usage.TotalTokens,
		LatencyMillis: time.Since(started).Milliseconds(),
	}, nil
}

func (g *DeepSeekGateway) StreamText(ctx context.Context, req TextRequest, onDelta func(TextDelta) error) (TextResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}

	payload := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"stream":      true,
		"max_tokens":  maxTokens,
		"temperature": req.Temperature,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TextResponse{}, err
	}

	started := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return TextResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Error != nil && apiErr.Error.Message != "" {
			return TextResponse{}, fmt.Errorf("deepseek API error: %s", apiErr.Error.Message)
		}
		return TextResponse{}, fmt.Errorf("deepseek API returned status %d", resp.StatusCode)
	}

	var (
		builder strings.Builder
		modelID string
		usage   struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return TextResponse{}, err
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return TextResponse{}, fmt.Errorf("deepseek API error: %s", chunk.Error.Message)
		}
		if chunk.Model != "" {
			modelID = chunk.Model
		}
		if chunk.Usage.TotalTokens > 0 {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if strings.TrimSpace(delta) == "" && delta == "" {
			continue
		}
		builder.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(TextDelta{Text: delta}); err != nil {
				return TextResponse{}, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return TextResponse{}, err
	}

	text := strings.TrimSpace(builder.String())
	if text == "" {
		return TextResponse{}, fmt.Errorf("deepseek API returned no streamed content")
	}
	return TextResponse{
		Provider:      "deepseek",
		Model:         firstNonEmpty(modelID, g.model),
		Text:          text,
		InputTokens:   usage.PromptTokens,
		OutputTokens:  usage.CompletionTokens,
		TotalTokens:   usage.TotalTokens,
		LatencyMillis: time.Since(started).Milliseconds(),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
