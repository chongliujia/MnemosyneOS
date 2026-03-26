package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mnemosyneos/internal/model"
)

const (
	IntentKindDirect = "direct_reply"
	IntentKindTask   = "task_request"
)

type IntentDecision struct {
	Kind       string  `json:"kind"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type IntentAgent struct {
	model model.TextGateway
}

func NewIntentAgent(textModel model.TextGateway) *IntentAgent {
	return &IntentAgent{model: textModel}
}

func (a *IntentAgent) Decide(message string) IntentDecision {
	return a.DecideWithContext(message, "")
}

func (a *IntentAgent) DecideWithContext(message, conversationContext string) IntentDecision {
	message = strings.TrimSpace(message)
	if message == "" {
		return IntentDecision{Kind: IntentKindDirect, Reason: "empty message", Confidence: 1}
	}

	if a != nil && a.model != nil {
		if decision, err := a.modelIntentDecision(message, conversationContext); err == nil && decision.Kind != "" {
			return decision
		}
	}

	if decision, ok := heuristicIntentDecision(message, conversationContext); ok {
		return decision
	}

	return IntentDecision{
		Kind:       IntentKindTask,
		Reason:     "fallback to operational path after model/heuristic ambiguity",
		Confidence: 0.55,
	}
}

func (a *IntentAgent) modelIntentDecision(message, conversationContext string) (IntentDecision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	userPrompt := "User message:\n" + message
	if strings.TrimSpace(conversationContext) != "" {
		userPrompt += "\n\nRecent conversation context:\n" + conversationContext
	}

	resp, err := a.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "Classify the user message for an AgentOS chat surface. Return strict JSON only: {\"kind\":\"direct_reply\"|\"task_request\",\"reason\":\"...\",\"confidence\":0.0-1.0}. direct_reply means ordinary conversation that should not start a task. task_request means the message clearly asks the system to do work. Use recent conversation context when the latest user message is short or ambiguous. If the user is giving a short affirmative follow-up like '需要', '继续', or '好的' in response to the assistant asking whether to continue work, prefer direct_reply unless the user explicitly requests a new operation.",
		UserPrompt:   userPrompt,
		MaxTokens:    80,
		Temperature:  0,
		Profile:      model.ProfileRouting,
	})
	if err != nil {
		return IntentDecision{}, err
	}

	var decision IntentDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &decision); err != nil {
		return IntentDecision{}, err
	}
	if decision.Kind != IntentKindDirect && decision.Kind != IntentKindTask {
		return IntentDecision{}, err
	}
	return decision, nil
}

func heuristicIntentDecision(message, conversationContext string) (IntentDecision, bool) {
	text := strings.ToLower(strings.TrimSpace(message))
	context := strings.ToLower(strings.TrimSpace(conversationContext))

	if looksLikeAffirmativeFollowup(text) && suggestsAssistantOfferedContinuation(context) {
		return IntentDecision{
			Kind:       IntentKindDirect,
			Reason:     "short affirmative follow-up to assistant continuation question",
			Confidence: 0.9,
		}, true
	}

	greetings := []string{"hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "早上好", "晚上好"}
	for _, greeting := range greetings {
		if text == greeting {
			return IntentDecision{
				Kind:       IntentKindDirect,
				Reason:     "greeting-only message",
				Confidence: 0.99,
			}, true
		}
	}

	social := []string{"谢谢", "thank you", "thanks", "好的", "ok", "okay", "行", "明白了"}
	for _, phrase := range social {
		if text == phrase {
			return IntentDecision{
				Kind:       IntentKindDirect,
				Reason:     "short conversational acknowledgement",
				Confidence: 0.95,
			}, true
		}
	}

	operationalMarkers := []string{
		"search", "web", "plan", "summarize", "inspect", "check", "review", "analyze",
		"read", "write", "edit", "update", "fix", "run", "execute", "create", "open",
		"task", "repo", "repository", "github", "issue", "email", "mail", "inbox",
		"shell", "command", "memory", "recall", "file", "browser", "install", "configure",
		"搜索", "查", "总结", "规划", "计划", "检查", "审查", "分析", "读取", "写入", "编辑",
		"修改", "修复", "运行", "执行", "创建", "打开", "任务", "仓库", "项目", "邮件",
		"终端", "命令", "记忆", "回忆", "文件", "安装", "配置", "帮我", "请你",
	}
	for _, marker := range operationalMarkers {
		if strings.Contains(text, marker) {
			return IntentDecision{
				Kind:       IntentKindTask,
				Reason:     "contains operational marker: " + marker,
				Confidence: 0.9,
			}, true
		}
	}

	if strings.ContainsAny(text, "/\\") {
		return IntentDecision{
			Kind:       IntentKindTask,
			Reason:     "contains filesystem-like path marker",
			Confidence: 0.82,
		}, true
	}

	if len([]rune(text)) <= 8 {
		return IntentDecision{
			Kind:       IntentKindDirect,
			Reason:     "very short non-operational message",
			Confidence: 0.72,
		}, true
	}

	return IntentDecision{}, false
}

func looksLikeAffirmativeFollowup(text string) bool {
	switch text {
	case "需要", "要", "好的", "好", "可以", "行", "继续", "继续吧", "是的", "要的", "嗯", "嗯嗯", "yes", "ok", "okay", "sure":
		return true
	default:
		return false
	}
}

func suggestsAssistantOfferedContinuation(context string) bool {
	if context == "" {
		return false
	}
	markers := []string{
		"需要我", "要我", "继续", "总结", "详细", "展开", "接着", "是否", "要不要", "would you like", "do you want", "need me to",
	}
	for _, marker := range markers {
		if strings.Contains(context, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
