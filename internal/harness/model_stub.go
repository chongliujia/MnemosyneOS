package harness

import (
	"context"
	"encoding/json"
	"strings"

	"mnemosyneos/internal/model"
)

type stubTextGateway struct{}

func (g stubTextGateway) GenerateText(_ context.Context, req model.TextRequest) (model.TextResponse, error) {
	text := stubResponse(req)
	return model.TextResponse{
		Provider: "harness",
		Model:    "stub",
		Text:     text,
	}, nil
}

func (g stubTextGateway) StreamText(ctx context.Context, req model.TextRequest, onDelta func(model.TextDelta) error) (model.TextResponse, error) {
	resp, err := g.GenerateText(ctx, req)
	if err != nil {
		return model.TextResponse{}, err
	}
	for _, chunk := range chunkText(resp.Text, 32) {
		if err := onDelta(model.TextDelta{Text: chunk}); err != nil {
			return model.TextResponse{}, err
		}
	}
	return resp, nil
}

func stubResponse(req model.TextRequest) string {
	switch req.Profile {
	case model.ProfileRouting:
		return stubRoutingResponse(req.UserPrompt)
	case model.ProfileConversation:
		return stubConversationResponse(req.UserPrompt)
	case model.ProfileSkills:
		return stubSkillsResponse(req.UserPrompt)
	default:
		return fallbackSection(req.UserPrompt, "Fallback reply:", "Fallback answer:", "Deterministic fallback summary:")
	}
}

func stubRoutingResponse(prompt string) string {
	userMessage := strings.ToLower(extractPromptBlock(prompt, "User message:"))
	response := map[string]any{
		"dialogue_act": "chat",
		"intent_kind":  "direct_reply",
		"skill":        "",
		"reason":       "stub default direct reply",
		"confidence":   0.9,
	}

	switch {
	case containsAnyText(userMessage, "继续", "展开", "详细", "继续展开") &&
		(strings.Contains(strings.ToLower(prompt), "pending_action=summarize_focus_task") ||
			strings.Contains(strings.ToLower(prompt), "pending_question=")):
		response["dialogue_act"] = "confirm"
		response["intent_kind"] = "direct_reply"
		response["reason"] = "follow-up to pending question"
		response["confidence"] = 0.97
	case containsAnyText(userMessage, "邮件", "邮箱", "inbox", "email", "mail"):
		response["dialogue_act"] = "new_task"
		response["intent_kind"] = "task_request"
		response["skill"] = "email-inbox"
		response["reason"] = "email request"
		response["confidence"] = 0.94
	case containsAnyText(userMessage, "搜索", "search", "web"):
		response["dialogue_act"] = "new_task"
		response["intent_kind"] = "task_request"
		response["skill"] = "web-search"
		response["reason"] = "search request"
		response["confidence"] = 0.96
	case strings.Contains(userMessage, "github") && containsAnyText(userMessage, "issue", "问题"):
		response["dialogue_act"] = "new_task"
		response["intent_kind"] = "task_request"
		response["skill"] = "github-issue-search"
		response["reason"] = "github issue request"
		response["confidence"] = 0.94
	case looksLikeSocialReplyText(userMessage):
		response["dialogue_act"] = "chat"
		response["intent_kind"] = "direct_reply"
		response["reason"] = "social greeting"
		response["confidence"] = 0.95
	default:
		response["dialogue_act"] = "new_task"
		response["intent_kind"] = "task_request"
		response["skill"] = "task-plan"
		response["reason"] = "generic task request"
		response["confidence"] = 0.75
	}

	raw, _ := json.Marshal(response)
	return string(raw)
}

func stubConversationResponse(prompt string) string {
	if strings.Contains(prompt, "Artifact excerpts:") {
		switch {
		case strings.Contains(prompt, "prod.example.internal"):
			return "重点邮件提到 deployment window，并要求在 rollout 后验证 prod.example.internal。"
		case strings.Contains(prompt, "OpenClaw"):
			return "OpenClaw 的 memory 设计核心是文件化记忆、混合检索和按需展开完整内容。"
		}
	}
	if answer := extractPromptBlock(prompt, "Fallback answer:"); strings.TrimSpace(answer) != "" {
		return strings.TrimSpace(answer)
	}
	if reply := extractPromptBlock(prompt, "Fallback reply:"); strings.TrimSpace(reply) != "" {
		return strings.TrimSpace(reply)
	}
	return "我在这里，可以继续当前会话。"
}

func stubSkillsResponse(prompt string) string {
	switch {
	case strings.Contains(prompt, "Selected skill: web-search"):
		return "搜索已完成。我已经整理了 OpenClaw memory 设计的要点，包括文件化记忆、BM25 与向量混合检索，以及时间衰减。需要我继续展开这些结果吗？"
	case strings.Contains(prompt, "Selected skill: github-issue-search"):
		return "GitHub issue 搜索已完成，相关问题和状态已经整理好。需要我继续归纳重点吗？"
	case strings.Contains(prompt, "Selected skill: email-inbox"):
		return "收件箱扫描已完成，邮件线程已经整理完毕。需要我继续总结要点吗？"
	case strings.Contains(prompt, "Task state: awaiting_approval"):
		return "任务正在等待审批。批准后重新运行即可继续。"
	case strings.Contains(prompt, "Selected skill: task-plan"):
		return "计划已生成，下一步可以检查 artifact 或继续执行计划。"
	default:
		if fallback := extractPromptBlock(prompt, "Deterministic fallback summary:"); strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		return "任务已完成。"
	}
}

func extractPromptBlock(prompt, label string) string {
	idx := strings.Index(prompt, label)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(prompt[idx+len(label):])
	return rest
}

func fallbackSection(prompt string, labels ...string) string {
	for _, label := range labels {
		if value := extractPromptBlock(prompt, label); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func chunkText(text string, size int) []string {
	if size <= 0 || len(text) <= size {
		return []string{text}
	}
	chunks := make([]string, 0, (len(text)/size)+1)
	runes := []rune(text)
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func containsAnyText(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func looksLikeSocialReplyText(text string) bool {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "早上好", "晚上好", "谢谢", "thank you", "thanks":
		return true
	default:
		return false
	}
}
