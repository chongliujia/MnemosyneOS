package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mnemosyneos/internal/model"
)

const (
	SkillTaskPlan          = "task-plan"
	SkillWebSearch         = "web-search"
	SkillGitHubIssueSearch = "github-issue-search"
	SkillEmailInbox        = "email-inbox"
	SkillMemoryConsolidate = "memory-consolidate"
	SkillFileRead          = "file-read"
	SkillFileEdit          = "file-edit"
	SkillShellCommand      = "shell-command"
)

var validSkills = map[string]struct{}{
	SkillTaskPlan:          {},
	SkillWebSearch:         {},
	SkillGitHubIssueSearch: {},
	SkillEmailInbox:        {},
	SkillMemoryConsolidate: {},
	SkillFileRead:          {},
	SkillFileEdit:          {},
	SkillShellCommand:      {},
}

type SkillDecision struct {
	Skill      string  `json:"skill"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type SkillAgent struct {
	model model.TextGateway
}

func NewSkillAgent(textModel model.TextGateway) *SkillAgent {
	return &SkillAgent{model: textModel}
}

func (a *SkillAgent) Decide(message string) SkillDecision {
	return a.DecideWithContext(message, "")
}

func (a *SkillAgent) DecideWithContext(message, conversationContext string) SkillDecision {
	message = strings.TrimSpace(message)
	if message == "" {
		return SkillDecision{Skill: SkillTaskPlan, Reason: "empty message", Confidence: 1}
	}

	if a != nil && a.model != nil {
		if decision, err := a.modelSkillDecision(message, conversationContext); err == nil && decision.Skill != "" {
			return decision
		}
	}

	if decision, ok := heuristicSkillDecision(message, conversationContext); ok {
		return decision
	}

	return SkillDecision{
		Skill:      SkillTaskPlan,
		Reason:     "fallback to task planning after model/heuristic ambiguity",
		Confidence: 0.55,
	}
}

func (a *SkillAgent) modelSkillDecision(message, conversationContext string) (SkillDecision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	userPrompt := "User message:\n" + message
	if strings.TrimSpace(conversationContext) != "" {
		userPrompt += "\n\nRecent conversation context:\n" + conversationContext
	}

	resp, err := a.model.GenerateText(ctx, model.TextRequest{
		SystemPrompt: "Route the user request to exactly one AgentOS skill. Return strict JSON only: {\"skill\":\"task-plan|web-search|github-issue-search|email-inbox|memory-consolidate|file-read|file-edit|shell-command\",\"reason\":\"...\",\"confidence\":0.0-1.0}. Use web-search for external information requests like searching the web or researching another project. Use memory-consolidate only when the user explicitly asks to consolidate, store, or summarize memory itself.",
		UserPrompt:   userPrompt,
		MaxTokens:    120,
		Temperature:  0,
		Profile:      model.ProfileRouting,
	})
	if err != nil {
		return SkillDecision{}, err
	}

	var decision SkillDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &decision); err != nil {
		return SkillDecision{}, err
	}
	if _, ok := validSkills[decision.Skill]; !ok {
		return SkillDecision{}, err
	}
	return decision, nil
}

func heuristicSkillDecision(message, conversationContext string) (SkillDecision, bool) {
	text := strings.ToLower(strings.TrimSpace(message))
	context := strings.ToLower(strings.TrimSpace(conversationContext))

	if looksLikeAffirmativeFollowup(text) && containsAnyMarker(context, "搜索", "search", "资料", "sources", "总结", "summary", "memory design") {
		return SkillDecision{Skill: SkillWebSearch, Reason: "follow-up accepted on recent search/research thread", Confidence: 0.82}, true
	}

	switch {
	case containsAnyMarker(text, "搜索", "查一下", "查找", "research", "search", "web", "look up", "google"):
		return SkillDecision{Skill: SkillWebSearch, Reason: "contains search or research marker", Confidence: 0.92}, true
	case containsAnyMarker(text, "github", "issue", "issues", "仓库问题", "工单", "议题"):
		return SkillDecision{Skill: SkillGitHubIssueSearch, Reason: "contains GitHub issue marker", Confidence: 0.9}, true
	case containsAnyMarker(text, "email", "mail", "inbox", "邮件", "邮箱", "收件箱"):
		return SkillDecision{Skill: SkillEmailInbox, Reason: "contains email marker", Confidence: 0.9}, true
	case containsAnyMarker(text, "consolidate memory", "memory consolidate", "记忆整理", "整合记忆", "memory summary"):
		return SkillDecision{Skill: SkillMemoryConsolidate, Reason: "explicit memory consolidation request", Confidence: 0.88}, true
	case containsAnyMarker(text, "read", "读取", "查看") && containsAnyMarker(text, "file", "readme", "文件", "文档", "note"):
		return SkillDecision{Skill: SkillFileRead, Reason: "contains file read marker", Confidence: 0.86}, true
	case containsAnyMarker(text, "edit", "write", "update", "修改", "编辑", "写入", "更新") && containsAnyMarker(text, "file", "readme", "文件", "文档", "note"):
		return SkillDecision{Skill: SkillFileEdit, Reason: "contains file edit marker", Confidence: 0.86}, true
	case containsAnyMarker(text, "shell", "command", "run", "execute", "终端", "命令", "运行", "执行"):
		return SkillDecision{Skill: SkillShellCommand, Reason: "contains shell command marker", Confidence: 0.84}, true
	default:
		return SkillDecision{}, false
	}
}

func containsAnyMarker(text string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
