package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mnemosyneos/internal/model"
)

const (
	DialogueActChat         = "chat"
	DialogueActNewTask      = "new_task"
	DialogueActAnswer       = "answer_question"
	DialogueActConfirm      = "confirm"
	DialogueActDeny         = "deny"
	DialogueActContinueTask = "continue_task"
	DialogueActClarify      = "clarify"
	DialogueActSwitchTopic  = "switch_topic"
)

var validDialogueActs = map[string]struct{}{
	DialogueActChat:         {},
	DialogueActNewTask:      {},
	DialogueActAnswer:       {},
	DialogueActConfirm:      {},
	DialogueActDeny:         {},
	DialogueActContinueTask: {},
	DialogueActClarify:      {},
	DialogueActSwitchTopic:  {},
}

type DialogueDecision struct {
	Act        string  `json:"act"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type DialogueAgent struct {
	model model.TextGateway
}

func NewDialogueAgent(textModel model.TextGateway) *DialogueAgent {
	return &DialogueAgent{model: textModel}
}

func (a *DialogueAgent) Decide(message, conversationContext string, state SessionState) DialogueDecision {
	message = strings.TrimSpace(message)
	if message == "" {
		return DialogueDecision{Act: DialogueActChat, Reason: "empty message", Confidence: 1}
	}

	if a != nil && a.model != nil {
		if decision, err := a.modelDecision(message, conversationContext, state); err == nil && decision.Act != "" {
			return decision
		}
	}
	if decision, ok := heuristicDialogueDecision(message, conversationContext, state); ok {
		return decision
	}
	return DialogueDecision{
		Act:        DialogueActNewTask,
		Reason:     "fallback to new task after ambiguity",
		Confidence: 0.55,
	}
}

func (a *DialogueAgent) modelDecision(message, conversationContext string, state SessionState) (DialogueDecision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userPrompt := "User message:\n" + message
	if strings.TrimSpace(conversationContext) != "" {
		userPrompt += "\n\nRecent conversation context:\n" + conversationContext
	}
	if strings.TrimSpace(state.PendingQuestion) != "" || strings.TrimSpace(state.FocusTaskID) != "" {
		userPrompt += "\n\nSession state:\n" + summarizeSessionState(state)
	}

	resp, err := a.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "Classify the dialogue act for an AgentOS chat turn. Return strict JSON only: {\"act\":\"chat|new_task|answer_question|confirm|deny|continue_task|clarify|switch_topic\",\"reason\":\"...\",\"confidence\":0.0-1.0}. Use session state and recent conversation context. Short replies like '需要', '继续', '好的' often confirm or continue the current thread instead of starting a new task.",
		UserPrompt:   userPrompt,
		MaxTokens:    100,
		Temperature:  0,
		Profile:      model.ProfileRouting,
	})
	if err != nil {
		return DialogueDecision{}, err
	}
	var decision DialogueDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &decision); err != nil {
		return DialogueDecision{}, err
	}
	if _, ok := validDialogueActs[decision.Act]; !ok {
		return DialogueDecision{}, err
	}
	return decision, nil
}

func heuristicDialogueDecision(message, conversationContext string, state SessionState) (DialogueDecision, bool) {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return DialogueDecision{Act: DialogueActChat, Reason: "empty message", Confidence: 1}, true
	}
	if looksLikeListChoiceReply(message) && assistantOfferedEnumeratedChoice(conversationContext) {
		return DialogueDecision{Act: DialogueActAnswer, Reason: "list choice after assistant enumerated options", Confidence: 0.91}, true
	}
	if looksLikeAffirmativeFollowup(text) && strings.TrimSpace(state.PendingQuestion) != "" {
		return DialogueDecision{Act: DialogueActConfirm, Reason: "affirmative reply to pending question", Confidence: 0.92}, true
	}
	if looksLikeNegativeFollowup(text) && strings.TrimSpace(state.PendingQuestion) != "" {
		return DialogueDecision{Act: DialogueActDeny, Reason: "negative reply to pending question", Confidence: 0.9}, true
	}
	if containsAnyMarker(text, "继续", "继续吧", "接着", "继续做", "go on", "continue") && strings.TrimSpace(state.FocusTaskID) != "" {
		return DialogueDecision{Act: DialogueActContinueTask, Reason: "continue marker with focused task", Confidence: 0.9}, true
	}
	if looksLikeSocialReply(text) {
		return DialogueDecision{Act: DialogueActChat, Reason: "social conversational reply", Confidence: 0.88}, true
	}
	if strings.TrimSpace(state.PendingQuestion) != "" && len([]rune(text)) <= 32 {
		return DialogueDecision{Act: DialogueActAnswer, Reason: "short answer while assistant question is pending", Confidence: 0.75}, true
	}
	if strings.TrimSpace(conversationContext) != "" && containsAnyMarker(text, "换个", "改成", "不要这个", "instead", "switch") {
		return DialogueDecision{Act: DialogueActSwitchTopic, Reason: "explicit topic switch marker", Confidence: 0.8}, true
	}
	if containsOperationalMarker(text) {
		return DialogueDecision{Act: DialogueActNewTask, Reason: "contains operational marker", Confidence: 0.85}, true
	}
	return DialogueDecision{}, false
}

func summarizeSessionState(state SessionState) string {
	var parts []string
	if state.Topic != "" {
		parts = append(parts, "topic="+state.Topic)
	}
	if state.FocusTaskID != "" {
		parts = append(parts, "focus_task_id="+state.FocusTaskID)
	}
	if state.PendingQuestion != "" {
		parts = append(parts, "pending_question="+state.PendingQuestion)
	}
	if state.PendingAction != "" {
		parts = append(parts, "pending_action="+state.PendingAction)
	}
	if state.LastUserAct != "" {
		parts = append(parts, "last_user_act="+state.LastUserAct)
	}
	if state.LastAssistantAct != "" {
		parts = append(parts, "last_assistant_act="+state.LastAssistantAct)
	}
	return strings.Join(parts, "\n")
}

func looksLikeNegativeFollowup(text string) bool {
	switch text {
	case "不用", "不需要", "不了", "不是", "别", "no", "nope", "not now":
		return true
	default:
		return false
	}
}

func looksLikeSocialReply(text string) bool {
	greetings := []string{"hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "早上好", "晚上好", "谢谢", "thank you", "thanks"}
	for _, value := range greetings {
		if text == value {
			return true
		}
	}
	return false
}

func containsOperationalMarker(text string) bool {
	markers := []string{
		"search", "web", "plan", "summarize", "inspect", "check", "review", "analyze",
		"read", "write", "edit", "update", "fix", "run", "execute", "create", "open",
		"task", "repo", "repository", "github", "issue", "email", "mail", "inbox",
		"shell", "command", "memory", "recall", "file", "install", "configure",
		"搜索", "查", "总结", "规划", "计划", "检查", "审查", "分析", "读取", "写入", "编辑",
		"修改", "修复", "运行", "执行", "创建", "打开", "任务", "仓库", "项目", "邮件",
		"终端", "命令", "记忆", "回忆", "文件", "安装", "配置", "帮我", "请你",
	}
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
