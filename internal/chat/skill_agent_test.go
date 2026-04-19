package chat

import "testing"

func TestHeuristicSkillDecisionPrefersWebSearchForChineseResearchRequest(t *testing.T) {
	decision, ok := heuristicSkillDecision("帮我搜索一下 OpenClaw 的 memory 设计", "")
	if !ok {
		t.Fatalf("expected heuristic decision")
	}
	if decision.Skill != SkillWebSearch {
		t.Fatalf("expected %s, got %s", SkillWebSearch, decision.Skill)
	}
}

func TestHeuristicSkillDecisionChineseCreateFileUsesFileEdit(t *testing.T) {
	decision, ok := heuristicSkillDecision("在 lab 里创建一个名为 foo.txt 的文件", "")
	if !ok {
		t.Fatalf("expected heuristic decision")
	}
	if decision.Skill != SkillFileEdit {
		t.Fatalf("expected %s, got %s", SkillFileEdit, decision.Skill)
	}
}

func TestHeuristicSkillDecisionChineseCreateDirectoryUsesShell(t *testing.T) {
	decision, ok := heuristicSkillDecision("在 lab 里创建一个名为 testLab 的目录", "")
	if !ok {
		t.Fatalf("expected heuristic decision")
	}
	if decision.Skill != SkillShellCommand {
		t.Fatalf("expected %s, got %s", SkillShellCommand, decision.Skill)
	}
}
