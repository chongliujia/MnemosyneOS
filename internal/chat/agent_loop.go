package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mnemosyneos/internal/model"
	"mnemosyneos/internal/skills"
)

const maxAgentTurns = 6

// AgentLoopEvent is emitted during the agent loop so callers (CLI, Web)
// can show real-time progress.
type AgentLoopEvent struct {
	Kind     string `json:"kind"`
	ToolName string `json:"tool_name,omitempty"`
	ToolArgs string `json:"tool_args,omitempty"`
	Result   string `json:"result,omitempty"`
	Text     string `json:"text,omitempty"`
	Error    string `json:"error,omitempty"`
}

// AgentLoop runs a multi-turn function-calling conversation loop.
// It sends the user message + available tools to the LLM, and if the LLM
// responds with tool_calls, executes them and feeds the results back,
// repeating until the LLM produces a final text reply or the turn limit is
// reached.
type AgentLoop struct {
	textModel model.TextGateway
	skills    *skills.AgentSkillRegistry
}

func NewAgentLoop(textModel model.TextGateway, skillRegistry *skills.AgentSkillRegistry) *AgentLoop {
	return &AgentLoop{
		textModel: textModel,
		skills:    skillRegistry,
	}
}

// Run executes the agent loop and returns the final assistant text.
// onEvent is called for each intermediate step so the UI can show progress.
// turnLocale should be the locale of the user's actual message (e.g. from
// detectTurnLocale on the raw user text); when empty it is inferred from userMessage.
func (a *AgentLoop) Run(ctx context.Context, systemPrompt, userMessage, turnLocale string, onEvent func(AgentLoopEvent)) (string, error) {
	if a == nil || a.textModel == nil {
		return "", fmt.Errorf("agent loop is not configured")
	}

	loc := strings.TrimSpace(turnLocale)
	if loc == "" {
		loc = detectTurnLocale(userMessage)
	}

	tools := a.skills.ToolDefinitions()
	messages := []model.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	for turn := 0; turn < maxAgentTurns; turn++ {
		req := model.TextRequest{
			Messages:    messages,
			Tools:       tools,
			MaxTokens:   2048,
			Temperature: 0.2,
			Profile:     model.ProfileConversation,
		}

		turnCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		resp, err := a.textModel.GenerateText(turnCtx, req)
		cancel()

		if err != nil {
			return "", fmt.Errorf("agent loop LLM call failed: %w", err)
		}

		// If the LLM returned text without tool calls, we're done.
		if len(resp.ToolCalls) == 0 {
			text := strings.TrimSpace(resp.Text)
			if text == "" {
				text = "我已完成处理，但没有产生文本回复。"
			}
			if onEvent != nil {
				onEvent(AgentLoopEvent{Kind: "final_reply", Text: text})
			}
			return text, nil
		}

		// Append the assistant message with its tool calls (content may
		// also contain "thinking" text from the model).
		assistantMsg := model.ChatMessage{
			Role:      "assistant",
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call and append the results.
		for _, tc := range resp.ToolCalls {
			if onEvent != nil {
				onEvent(AgentLoopEvent{
					Kind:     "tool_call",
					ToolName: tc.Function.Name,
					ToolArgs: tc.Function.Arguments,
				})
			}

			toolArgs := withTurnLocale(tc.Function.Arguments, loc)
			toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
			result, execErr := a.skills.Execute(toolCtx, tc.Function.Name, toolArgs)
			toolCancel()

			if execErr != nil {
				result = fmt.Sprintf("Error: %s", execErr.Error())
			}

			if onEvent != nil {
				onEvent(AgentLoopEvent{
					Kind:     "tool_result",
					ToolName: tc.Function.Name,
					Result:   truncateResult(result, 1500),
				})
			}

			messages = append(messages, model.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	// Hit the turn limit — synthesize what we have.
	if onEvent != nil {
		onEvent(AgentLoopEvent{Kind: "turn_limit", Text: "达到了最大工具调用轮次。"})
	}
	return "我已经使用了多个工具来处理你的请求。如果还需要更多信息，请继续告诉我。", nil
}

func withTurnLocale(rawArgs, locale string) string {
	rawArgs = strings.TrimSpace(rawArgs)
	if rawArgs == "" {
		payload, _ := json.Marshal(map[string]any{"_locale": locale})
		return string(payload)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
		return rawArgs
	}
	if parsed == nil {
		parsed = map[string]any{}
	}
	parsed["_locale"] = locale
	updated, err := json.Marshal(parsed)
	if err != nil {
		return rawArgs
	}
	return string(updated)
}

func truncateResult(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}
