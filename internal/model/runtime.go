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
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type completionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
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

type streamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content json.RawMessage `json:"content"`
		} `json:"delta"`
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

const (
	ProfileConversation = "conversation"
	ProfileRouting      = "routing"
	ProfileSkills       = "skills"
)

type ProfileConfig struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature,omitempty"`
}

type Config struct {
	Provider     string        `json:"provider"`
	Preset       string        `json:"preset,omitempty"`
	BaseURL      string        `json:"base_url"`
	APIKey       string        `json:"api_key,omitempty"`
	Conversation ProfileConfig `json:"conversation"`
	Routing      ProfileConfig `json:"routing"`
	Skills       ProfileConfig `json:"skills"`
}

type ConfigStore struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

type RuntimeGateway struct {
	store      *ConfigStore
	httpClient *http.Client
}

func NewConfigStore(runtimeRoot string) (*ConfigStore, error) {
	path := filepath.Join(runtimeRoot, "model", "config.json")
	store := &ConfigStore{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if raw, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		store.cfg = normalizeConfig(cfg)
		return store, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	store.cfg = normalizeConfig(configFromEnv())
	if err := store.Save(store.cfg); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *ConfigStore) Get() Config {
	if s == nil {
		return Config{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *ConfigStore) Save(cfg Config) error {
	if s == nil {
		return fmt.Errorf("model config store is not configured")
	}
	cfg = normalizeConfig(cfg)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

func NewRuntimeGateway(store *ConfigStore) TextGateway {
	return &RuntimeGateway{
		store: store,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (g *RuntimeGateway) GenerateText(ctx context.Context, req TextRequest) (TextResponse, error) {
	cfg := g.currentConfig()
	profile, resolvedReq, err := g.resolveRequest(cfg, req)
	if err != nil {
		return TextResponse{}, err
	}
	return g.generateWithProfile(ctx, cfg, profile, resolvedReq)
}

func (g *RuntimeGateway) StreamText(ctx context.Context, req TextRequest, onDelta func(TextDelta) error) (TextResponse, error) {
	cfg := g.currentConfig()
	profile, resolvedReq, err := g.resolveRequest(cfg, req)
	if err != nil {
		return TextResponse{}, err
	}
	return g.streamWithProfile(ctx, cfg, profile, resolvedReq, onDelta)
}

func (g *RuntimeGateway) TestConfig(ctx context.Context, cfg Config) (TextResponse, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg); err != nil {
		return TextResponse{}, err
	}
	req := TextRequest{
		SystemPrompt: "Reply with exactly: model connection ok",
		UserPrompt:   "Test the model connection.",
		MaxTokens:    24,
		Temperature:  0,
		Profile:      ProfileRouting,
	}
	profile, resolvedReq, err := g.resolveRequest(cfg, req)
	if err != nil {
		return TextResponse{}, err
	}
	return g.generateWithProfile(ctx, cfg, profile, resolvedReq)
}

func (g *RuntimeGateway) currentConfig() Config {
	if g == nil || g.store == nil {
		return Config{}
	}
	return g.store.Get()
}

func (g *RuntimeGateway) resolveRequest(cfg Config, req TextRequest) (ProfileConfig, TextRequest, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg); err != nil {
		return ProfileConfig{}, TextRequest{}, err
	}
	profile := cfg.profile(req.Profile)
	if strings.TrimSpace(profile.Model) == "" {
		return ProfileConfig{}, TextRequest{}, fmt.Errorf("model profile %q is not configured", firstNonEmpty(req.Profile, ProfileConversation))
	}
	if profile.MaxTokens > 0 && (req.MaxTokens <= 0 || req.MaxTokens > profile.MaxTokens) {
		req.MaxTokens = profile.MaxTokens
	}
	req.Temperature = applyTemperature(req.Profile, req.Temperature, profile.Temperature)
	return profile, req, nil
}

func (g *RuntimeGateway) generateWithProfile(ctx context.Context, cfg Config, profile ProfileConfig, req TextRequest) (TextResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	payload := map[string]any{
		"model": profile.Model,
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completionEndpoint(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return TextResponse{}, err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TextResponse{}, err
	}
	raw, err := decodeCompletionResponse(rawBody)
	if err != nil {
		return TextResponse{}, fmt.Errorf("%s API returned a non-compatible response: %s", cfg.Provider, err.Error())
	}
	if resp.StatusCode >= 400 {
		if raw.Error != nil && raw.Error.Message != "" {
			return TextResponse{}, fmt.Errorf("%s API error: %s", cfg.Provider, raw.Error.Message)
		}
		return TextResponse{}, fmt.Errorf("%s API returned status %d: %s", cfg.Provider, resp.StatusCode, previewTextBody(rawBody, 180))
	}
	if len(raw.Choices) == 0 {
		return TextResponse{}, fmt.Errorf("%s API returned no choices", cfg.Provider)
	}
	text := normalizeAssistantTextContent(raw.Choices[0].Message.Content)
	if strings.TrimSpace(text) == "" {
		return TextResponse{}, fmt.Errorf("%s API returned an empty message content", cfg.Provider)
	}
	return TextResponse{
		Provider:      cfg.Provider,
		Model:         firstNonEmpty(raw.Model, profile.Model),
		Text:          strings.TrimSpace(text),
		InputTokens:   raw.Usage.PromptTokens,
		OutputTokens:  raw.Usage.CompletionTokens,
		TotalTokens:   raw.Usage.TotalTokens,
		LatencyMillis: time.Since(started).Milliseconds(),
	}, nil
}

func (g *RuntimeGateway) streamWithProfile(ctx context.Context, cfg Config, profile ProfileConfig, req TextRequest, onDelta func(TextDelta) error) (TextResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	payload := map[string]any{
		"model": profile.Model,
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completionEndpoint(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
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
			return TextResponse{}, fmt.Errorf("%s API error: %s", cfg.Provider, apiErr.Error.Message)
		}
		return TextResponse{}, fmt.Errorf("%s API returned status %d", cfg.Provider, resp.StatusCode)
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
		chunk, err := decodeStreamChunk([]byte(payload))
		if err != nil {
			return TextResponse{}, fmt.Errorf("%s streamed a non-compatible event: %s", cfg.Provider, err.Error())
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return TextResponse{}, fmt.Errorf("%s API error: %s", cfg.Provider, chunk.Error.Message)
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
		delta := normalizeAssistantDeltaContent(chunk.Choices[0].Delta.Content)
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
		return TextResponse{}, fmt.Errorf("%s API returned no streamed content", cfg.Provider)
	}
	return TextResponse{
		Provider:      cfg.Provider,
		Model:         firstNonEmpty(modelID, profile.Model),
		Text:          text,
		InputTokens:   usage.PromptTokens,
		OutputTokens:  usage.CompletionTokens,
		TotalTokens:   usage.TotalTokens,
		LatencyMillis: time.Since(started).Milliseconds(),
	}, nil
}

func decodeCompletionResponse(raw []byte) (completionResponse, error) {
	var out completionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return completionResponse{}, fmt.Errorf("%v; body=%s", err, previewTextBody(raw, 180))
	}
	return out, nil
}

func decodeStreamChunk(raw []byte) (streamChunk, error) {
	var out streamChunk
	if err := json.Unmarshal(raw, &out); err != nil {
		return streamChunk{}, fmt.Errorf("%v; body=%s", err, previewTextBody(raw, 180))
	}
	return out, nil
}

func normalizeAssistantTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var list []any
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.TrimSpace(extractTextFromAny(list))
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return strings.TrimSpace(extractTextFromAny(obj))
	}
	return strings.TrimSpace(string(raw))
}

func normalizeAssistantDeltaContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var list []any
	if err := json.Unmarshal(raw, &list); err == nil {
		return extractDeltaTextFromAny(list)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return extractDeltaTextFromAny(obj)
	}
	return string(raw)
}

func extractTextFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(extractTextFromAny(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "output_text", "reasoning_content"} {
			if nested, ok := v[key]; ok {
				if text := strings.TrimSpace(extractTextFromAny(nested)); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func extractDeltaTextFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var builder strings.Builder
		for _, item := range v {
			builder.WriteString(extractDeltaTextFromAny(item))
		}
		return builder.String()
	case map[string]any:
		for _, key := range []string{"text", "content", "output_text", "reasoning_content"} {
			if nested, ok := v[key]; ok {
				if text := extractDeltaTextFromAny(nested); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func previewTextBody(raw []byte, max int) string {
	text := strings.TrimSpace(string(raw))
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	if len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func completionEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "/chat/completions"
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(lower, "/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/chat/completions"
}

func (c Config) profile(name string) ProfileConfig {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ProfileRouting:
		return c.Routing
	case ProfileSkills:
		return c.Skills
	default:
		return c.Conversation
	}
}

func configFromEnv() Config {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_PROVIDER")))
	cfg := Config{
		Provider: "none",
		Conversation: ProfileConfig{
			MaxTokens:   1600,
			Temperature: 0.2,
		},
		Routing: ProfileConfig{
			MaxTokens:   220,
			Temperature: 0,
		},
		Skills: ProfileConfig{
			MaxTokens:   1200,
			Temperature: 0.2,
		},
	}
	switch provider {
	case "deepseek":
		cfg.Provider = "deepseek"
		cfg.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "https://api.deepseek.com")
		cfg.APIKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
		name := firstNonEmpty(strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_NAME")), "deepseek-chat")
		cfg.Conversation.Model = name
		cfg.Routing.Model = name
		cfg.Skills.Model = name
	case "siliconflow":
		cfg.Provider = "siliconflow"
		cfg.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("SILICONFLOW_BASE_URL")), "https://api.siliconflow.cn/v1")
		cfg.APIKey = strings.TrimSpace(os.Getenv("SILICONFLOW_API_KEY"))
		name := firstNonEmpty(strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_NAME")), "Qwen/Qwen2.5-7B-Instruct")
		cfg.Conversation.Model = name
		cfg.Routing.Model = name
		cfg.Skills.Model = name
	case "openai":
		cfg.Provider = "openai"
		cfg.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), "https://api.openai.com/v1")
		cfg.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		name := firstNonEmpty(strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_NAME")), "gpt-4o-mini")
		cfg.Conversation.Model = name
		cfg.Routing.Model = name
		cfg.Skills.Model = name
	case "openai-compatible":
		cfg.Provider = "openai-compatible"
		cfg.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_COMPAT_BASE_URL")), "")
		cfg.APIKey = strings.TrimSpace(os.Getenv("OPENAI_COMPAT_API_KEY"))
		name := firstNonEmpty(strings.TrimSpace(os.Getenv("MNEMOSYNE_MODEL_NAME")), "")
		cfg.Conversation.Model = name
		cfg.Routing.Model = name
		cfg.Skills.Model = name
	default:
		cfg.Provider = "none"
	}
	return normalizeConfig(cfg)
}

func normalizeConfig(cfg Config) Config {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.Preset = strings.ToLower(strings.TrimSpace(cfg.Preset))
	if cfg.Provider == "" || cfg.Provider == "none" {
		cfg.Provider = "none"
		cfg.Preset = ""
		cfg.BaseURL = ""
		cfg.Conversation = ProfileConfig{}
		cfg.Routing = ProfileConfig{}
		cfg.Skills = ProfileConfig{}
		return cfg
	}

	switch cfg.Provider {
	case "deepseek":
		cfg.BaseURL = firstNonEmpty(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), "https://api.deepseek.com")
		cfg.Conversation.Model = firstNonEmpty(strings.TrimSpace(cfg.Conversation.Model), "deepseek-chat")
	case "siliconflow":
		cfg.BaseURL = firstNonEmpty(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), "https://api.siliconflow.cn/v1")
		cfg.Conversation.Model = firstNonEmpty(strings.TrimSpace(cfg.Conversation.Model), "Qwen/Qwen2.5-7B-Instruct")
	case "openai":
		cfg.BaseURL = firstNonEmpty(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), "https://api.openai.com/v1")
		cfg.Conversation.Model = firstNonEmpty(strings.TrimSpace(cfg.Conversation.Model), "gpt-4o-mini")
	case "openai-compatible":
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		cfg.Conversation.Model = strings.TrimSpace(cfg.Conversation.Model)
	default:
		cfg.Provider = "openai-compatible"
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		cfg.Conversation.Model = strings.TrimSpace(cfg.Conversation.Model)
	}
	cfg.Routing.Model = firstNonEmpty(strings.TrimSpace(cfg.Routing.Model), cfg.Conversation.Model)
	cfg.Skills.Model = firstNonEmpty(strings.TrimSpace(cfg.Skills.Model), cfg.Conversation.Model)

	cfg.Conversation.MaxTokens = normalizeMaxTokens(cfg.Conversation.MaxTokens, 1600)
	cfg.Routing.MaxTokens = normalizeMaxTokens(cfg.Routing.MaxTokens, 220)
	cfg.Skills.MaxTokens = normalizeMaxTokens(cfg.Skills.MaxTokens, 1200)
	cfg.Conversation.Temperature = normalizeTemperature(cfg.Conversation.Temperature, 0.2)
	cfg.Routing.Temperature = 0
	cfg.Skills.Temperature = normalizeTemperature(cfg.Skills.Temperature, 0.2)
	return cfg
}

func normalizeMaxTokens(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	if value > 8192 {
		return 8192
	}
	return value
}

func normalizeTemperature(value, fallback float64) float64 {
	if value < 0 {
		return fallback
	}
	if value > 2 {
		return 2
	}
	if value == 0 {
		return fallback
	}
	return value
}

func applyTemperature(profile string, requested, configured float64) float64 {
	_ = profile
	if requested == 0 {
		return 0
	}
	if configured > 0 {
		return configured
	}
	return requested
}

func validateConfig(cfg Config) error {
	cfg = normalizeConfig(cfg)
	if cfg.Provider == "none" {
		return fmt.Errorf("model provider is not configured")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("model API key is not configured")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("model base URL is not configured")
	}
	if strings.TrimSpace(cfg.Conversation.Model) == "" {
		return fmt.Errorf("conversation model is not configured")
	}
	if strings.TrimSpace(cfg.Routing.Model) == "" {
		return fmt.Errorf("routing model is not configured")
	}
	if strings.TrimSpace(cfg.Skills.Model) == "" {
		return fmt.Errorf("skills model is not configured")
	}
	return nil
}
