package chat

import (
	"context"
	"encoding/json"
	"regexp"
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

// DecideWithContext is the single entry point used by the chat service to
// classify every incoming user turn. The routing policy is deliberately
// LLM-first:
//
//  1. If a model gateway is available, we trust it. The prompt tells the
//     model to err on the side of direct_reply whenever the user's target is
//     ambiguous, partial, or just chatty; this avoids silently spinning up
//     an approval-gated task for unclear requests.
//  2. If the model call fails (network/API key dead, classifier timeout),
//     we fall back to the legacy heuristic so the agent can still work
//     offline.
//  3. If neither path produces a classification, we default to
//     direct_reply — not task_request. A missed reply is cheap; a missed
//     task can block the user in an approval loop they never asked for.
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
		Kind:       IntentKindDirect,
		Reason:     "classifier unavailable and heuristic ambiguous, defaulting to conversation",
		Confidence: 0.5,
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
		SystemPrompt: `You classify the user's chat message for an AgentOS runtime. Return STRICT JSON only (no markdown, no prose):
{"kind":"direct_reply"|"task_request","reason":"<short>","confidence":0.0-1.0}

There are only two labels:

direct_reply
  The assistant should answer in chat. Use this whenever the turn would be better served by a reply or a clarifying question than by creating a formal task+plan that needs approval. This includes:
  - Greetings, small talk, acknowledgements ("你好", "thanks", "好的")
  - Questions, even when they mention files/paths/tasks ("这个项目在哪", "what does memory do?")
  - Follow-ups to the assistant's own previous question ("需要", "2吧", "用第一个")
  - Requests whose TARGET is ambiguous, partial, or malformed — e.g. contains a space mid-word, an unknown name, a typo, or could mean several things. Ask the user to clarify instead of acting.
  - Anything where one extra turn of conversation is obviously cheaper than spinning up a task.

task_request
  The user wants the system to DO an action AND there is enough concrete signal to act. ALL of these must be true:
  (1) explicit action verb / imperative (search, run, edit, create, 帮我搜索, 帮我运行, 帮我创建, please X, etc.)
  (2) a concrete, unambiguous target (specific path, full keyword, clear file/object name, clear query)
  (3) no obvious typos, partial words, or split-token targets

If any of (1)(2)(3) is shaky → direct_reply. When in doubt → direct_reply.

Examples:
"你好" → direct_reply
"你能帮我查看一个名叫la b的目录" → direct_reply (target "la b" looks like a typo/split, ask for clarification)
"帮我查看 /Users/me/lab 目录" → task_request (clear verb + clear path)
"这个项目叫什么？" → direct_reply (question)
"帮我搜索 AgentOS 架构设计" → task_request (clear verb + clear keyword)
"搜一下" → direct_reply (no target)
"运行一下" → direct_reply (no target)
"2吧" → direct_reply (follow-up choice)`,
		UserPrompt:  userPrompt,
		MaxTokens:   80,
		Temperature: 0,
		Profile:     model.ProfileRouting,
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

var listChoiceReplyRE = regexp.MustCompile(`(?i)^(?:第?\s*[1-9]\d{0,2}\s*个|第[一二三四五六七八九十两百]+\s*个|选项\s*[1-9]\d{0,2}|option\s*[1-9]\d{0,2}|[1-9]\d{0,2})(?:吧|啊|呢|哦|呀|咯|的|这个|那个)?$`)

func stripPickerPathSuffixes(s string) string {
	out := strings.TrimSpace(s)
	for {
		prev := out
		for _, suf := range []string{"这个吧", "那个吧", "这个", "那个", "吧", "哦", "啊", "呢", "呀"} {
			if strings.HasSuffix(out, suf) {
				out = strings.TrimSpace(strings.TrimSuffix(out, suf))
				break
			}
		}
		if out == prev {
			break
		}
	}
	return out
}

// pathPickerFollowup matches turns like "/Users/me/lab 这个" where the user
// picks a concrete path after a directory listing — answer in chat, not task-plan.
func pathPickerFollowup(raw string) bool {
	s0 := strings.TrimSpace(raw)
	if s0 == "" || len([]rune(s0)) > 512 {
		return false
	}
	s := strings.TrimSpace(stripPickerPathSuffixes(s0))
outer:
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		for _, p := range []string{"在", "就", "选", "用", "到", "是"} {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(strings.TrimPrefix(s, p))
				continue outer
			}
		}
		break
	}
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || s == "~"
}

func looksLikeListChoiceReply(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" || len([]rune(msg)) > 36 {
		return false
	}
	return listChoiceReplyRE.MatchString(msg)
}

func assistantOfferedEnumeratedChoice(context string) bool {
	c := strings.TrimSpace(context)
	if c == "" {
		return false
	}
	lower := strings.ToLower(c)
	if strings.Contains(lower, "哪一个") || strings.Contains(lower, "which one") || strings.Contains(lower, "which of these") {
		return true
	}
	if strings.Contains(c, "完整路径") && strings.Contains(c, "|") {
		return true
	}
	if strings.Count(c, "|") >= 4 {
		return true
	}
	if strings.Contains(c, "1.") && strings.Contains(c, "2.") {
		return true
	}
	return false
}

// looksLikeReadOnlyFilesystemLookup matches “where is this folder / 查看…目录…位置”
// style requests. Answer via the agent (list_directory, search_files, etc.),
// not task-plan + approval, so the user gets concrete paths in-chat.
func looksLikeReadOnlyFilesystemLookup(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return false
	}
	if pathPickerFollowup(s) {
		return true
	}
	lower := strings.ToLower(s)

	if (strings.Contains(lower, "where ") || strings.Contains(lower, "which ") ||
		strings.Contains(lower, "location of") || strings.Contains(lower, "path to")) &&
		(strings.Contains(lower, "directory") || strings.Contains(lower, "folder") ||
			strings.Contains(lower, "file")) {
		return true
	}
	if strings.Contains(lower, "find ") && (strings.Contains(lower, "folder") || strings.Contains(lower, "directory")) {
		return true
	}

	viewOps := strings.Contains(s, "查看") || strings.Contains(s, "显示") || strings.Contains(s, "列出") ||
		strings.Contains(s, "找找") || strings.Contains(s, "看一下") || strings.Contains(s, "看下")
	dirNoun := strings.Contains(s, "目录") || strings.Contains(s, "文件夹")
	if dirNoun && (strings.Contains(s, "在哪") || strings.Contains(s, "在哪儿") || strings.Contains(s, "位置") ||
		strings.Contains(s, "路径")) {
		return true
	}
	if viewOps && dirNoun {
		return true
	}
	if viewOps && strings.Contains(s, "文件") &&
		(strings.Contains(s, "在哪") || strings.Contains(s, "位置") || strings.Contains(s, "路径")) {
		return true
	}
	return false
}

func heuristicIntentDecision(message, conversationContext string) (IntentDecision, bool) {
	text := strings.ToLower(strings.TrimSpace(message))
	context := strings.ToLower(strings.TrimSpace(conversationContext))

	if looksLikeReadOnlyFilesystemLookup(message) {
		return IntentDecision{
			Kind:       IntentKindDirect,
			Reason:     "read-only filesystem lookup (answer with workspace tools, not task-plan)",
			Confidence: 0.93,
		}, true
	}

	if looksLikeListChoiceReply(message) && assistantOfferedEnumeratedChoice(conversationContext) {
		return IntentDecision{
			Kind:       IntentKindDirect,
			Reason:     "short list/table choice after assistant enumerated options",
			Confidence: 0.92,
		}, true
	}

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

	isQuestion := looksLikeQuestion(text)

	// Strong operational markers: explicit action verbs that almost always
	// mean the user wants the system to DO something.
	strongMarkers := []string{
		"search", "summarize", "execute", "install", "configure",
		"搜索", "总结", "执行", "安装", "配置",
		"帮我搜", "帮我查", "帮我找", "帮我写", "帮我读", "帮我运行",
		"帮我编辑", "帮我修改", "帮我修复", "帮我创建", "帮我打开",
		"帮我检查", "帮我分析", "帮我规划",
	}
	for _, marker := range strongMarkers {
		if strings.Contains(text, marker) {
			return IntentDecision{
				Kind:       IntentKindTask,
				Reason:     "contains strong operational marker: " + marker,
				Confidence: 0.92,
			}, true
		}
	}

	// Action verbs that are task-like when they appear as commands (not in
	// questions). If the message looks like a question, these are treated
	// as informational — the user is asking ABOUT these topics.
	actionVerbs := []string{
		"plan", "inspect", "review", "analyze", "read", "write",
		"edit", "update", "fix", "run", "create", "open", "check",
		"规划", "计划", "检查", "审查", "分析", "读取", "写入", "编辑",
		"修改", "修复", "运行", "创建", "打开",
		"帮我", "请你",
	}
	if !isQuestion {
		for _, verb := range actionVerbs {
			if strings.Contains(text, verb) {
				return IntentDecision{
					Kind:       IntentKindTask,
					Reason:     "contains action verb in non-question context: " + verb,
					Confidence: 0.88,
				}, true
			}
		}
	}

	// Topic nouns: these words describe system concepts but can appear in
	// casual questions ("what is my project?", "where are the files?").
	// Only route to task when combined with an action verb AND not a question.
	topicNouns := []string{
		"repo", "repository", "github", "issue", "email", "mail", "inbox",
		"shell", "command", "memory", "recall", "file", "browser",
		"task", "web",
		"仓库", "项目", "邮件", "终端", "命令", "记忆", "回忆",
		"文件", "任务",
	}
	if !isQuestion {
		for _, noun := range topicNouns {
			if strings.Contains(text, noun) {
				return IntentDecision{
					Kind:       IntentKindTask,
					Reason:     "contains topic noun in imperative context: " + noun,
					Confidence: 0.78,
				}, true
			}
		}
	}

	if strings.ContainsAny(text, "/\\") && !isQuestion {
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

	// If the message is a question and wasn't caught by strong markers,
	// treat it as conversational.
	if isQuestion {
		return IntentDecision{
			Kind:       IntentKindDirect,
			Reason:     "informational question without strong action marker",
			Confidence: 0.85,
		}, true
	}

	return IntentDecision{}, false
}

// looksLikeQuestion returns true when the message uses question syntax
// (question marks, interrogative words, or common question patterns).
func looksLikeQuestion(text string) bool {
	if strings.ContainsAny(text, "?？") {
		return true
	}
	questionPatterns := []string{
		"what ", "where ", "when ", "which ", "who ", "how ", "why ",
		"is it", "is there", "are there", "can you tell", "do you know",
		"could you tell", "would you",
		"什么", "哪里", "哪个", "哪些", "在哪", "怎么", "怎样",
		"为什么", "如何", "多少", "几个", "是什么", "是哪",
		"能不能", "可不可以", "有没有", "是不是",
		"告訴我", "告诉我", "知道吗", "对吗", "吗",
		"嗎", "呢",
	}
	for _, p := range questionPatterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
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
