package chat

import (
	"strings"
	"testing"
)

func TestHeuristicRouteDecisionForChineseSearchRequest(t *testing.T) {
	decision := heuristicRouteDecision("帮我搜索一下 OpenClaw 的 memory 设计", "", SessionState{})
	if decision.IntentKind != IntentKindTask {
		t.Fatalf("expected task_request, got %#v", decision)
	}
	if decision.Skill != SkillWebSearch {
		t.Fatalf("expected %s, got %#v", SkillWebSearch, decision)
	}
	if decision.TargetScope != "new_task" {
		t.Fatalf("expected new_task scope, got %#v", decision)
	}
	if decision.RiskLevel != "medium" {
		t.Fatalf("expected medium risk, got %#v", decision)
	}
}

func TestHeuristicRouteDecisionForFocusedContinuation(t *testing.T) {
	decision := heuristicRouteDecision("继续展开", "assistant: 已经完成搜索并可继续总结", SessionState{
		FocusTaskID: "task-123",
	})
	if decision.TargetScope != "focused_task" {
		t.Fatalf("expected focused_task scope, got %#v", decision)
	}
	if decision.NeedsConfirmation {
		t.Fatalf("expected focused followup to skip confirmation, got %#v", decision)
	}
	if len(decision.CandidateSkills) == 0 {
		t.Fatalf("expected candidate skills, got %#v", decision)
	}
}

func TestHeuristicRouteDecisionForHighRiskEditNeedsConfirmation(t *testing.T) {
	decision := heuristicRouteDecision("edit README.md", "", SessionState{})
	if decision.RiskLevel != "high" {
		t.Fatalf("expected high risk, got %#v", decision)
	}
	if !decision.NeedsConfirmation {
		t.Fatalf("expected confirmation requirement, got %#v", decision)
	}
}

func TestHeuristicRouteReadOnlyDirLookupIsDirectNoTaskSkill(t *testing.T) {
	t.Parallel()
	decision := heuristicRouteDecision("查看一个叫lab的目录位置", "", SessionState{})
	if decision.IntentKind != IntentKindDirect {
		t.Fatalf("expected direct intent, got %#v", decision)
	}
	if strings.TrimSpace(decision.Skill) != "" {
		t.Fatalf("expected empty skill for direct reply, got %q", decision.Skill)
	}
}

func TestHeuristicRoutePathPickerFollowupIsDirect(t *testing.T) {
	t.Parallel()
	decision := heuristicRouteDecision("/Users/j/foo/lab 这个吧", "", SessionState{})
	if decision.IntentKind != IntentKindDirect {
		t.Fatalf("expected direct intent, got %#v", decision)
	}
	if strings.TrimSpace(decision.Skill) != "" {
		t.Fatalf("expected empty skill for direct reply, got %q", decision.Skill)
	}
}

func TestHeuristicRouteExplicitCreateFileSkipsConfirmation(t *testing.T) {
	decision := heuristicRouteDecision("创建一个名叫testlab的文件", "", SessionState{})
	if decision.IntentKind != IntentKindTask {
		t.Fatalf("expected task, got %#v", decision)
	}
	if decision.NeedsConfirmation {
		t.Fatalf("expected explicit create-file request to skip confirmation, got %#v", decision)
	}
}
