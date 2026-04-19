package airuntime

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Orchestrator struct {
	store *Store
}

func NewOrchestrator(store *Store) *Orchestrator {
	return &Orchestrator{store: store}
}

func (o *Orchestrator) Recover() error {
	state, err := o.store.LoadState()
	if err != nil {
		return err
	}

	if state.ActiveTaskID == nil {
		if state.Status != "idle" {
			state.Status = "idle"
			return o.store.SaveState(state)
		}
		return nil
	}

	taskID := *state.ActiveTaskID
	task, err := o.store.GetTask(taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			state.ActiveTaskID = nil
			state.Status = "idle"
			return o.store.SaveState(state)
		}
		return err
	}

	if task.State == TaskStateActive {
		if _, err := o.store.MoveTask(taskID, TaskStatePlanned, func(t *Task) {
			if t.Metadata == nil {
				t.Metadata = map[string]string{}
			}
			t.Metadata["recovered_from_active"] = "true"
			t.Metadata["recovered_at"] = time.Now().UTC().Format(time.RFC3339)
			t.NextAction = "recovered for re-execution"
		}); err != nil {
			return err
		}
	}

	state.ActiveTaskID = nil
	state.Status = "idle"
	return o.store.SaveState(state)
}

func (o *Orchestrator) SubmitTask(req CreateTaskRequest) (Task, error) {
	if _, err := o.store.CreateTask(req); err != nil {
		return Task{}, err
	}
	return o.Advance()
}

func (o *Orchestrator) Advance() (Task, error) {
	state, err := o.store.LoadState()
	if err != nil {
		return Task{}, err
	}

	if state.ActiveTaskID != nil {
		activeTask, err := o.store.GetTask(*state.ActiveTaskID)
		if err == nil && activeTask.State == TaskStateActive {
			return activeTask, nil
		}
		if err != nil && !errors.Is(err, ErrTaskNotFound) {
			return Task{}, err
		}
		state.ActiveTaskID = nil
		state.Status = "idle"
		if err := o.store.SaveState(state); err != nil {
			return Task{}, err
		}
	}

	tasks, err := o.store.ListTasks()
	if err != nil {
		return Task{}, err
	}

	for _, task := range tasks {
		switch task.State {
		case TaskStateInbox:
			plannedTask, err := o.planTask(task.TaskID)
			if err != nil {
				return Task{}, err
			}
			if plannedTask.State == TaskStatePlanned {
				return o.activateTask(plannedTask.TaskID)
			}
			return plannedTask, nil
		case TaskStatePlanned:
			return o.activateTask(task.TaskID)
		case TaskStateAwaitingApproval, TaskStateActive:
			return task, nil
		}
	}

	return Task{}, ErrTaskNotFound
}

func (o *Orchestrator) ApproveTask(taskID, approvedBy string) (Task, error) {
	task, err := o.RequeueTask(taskID, "approval_granted", func(t *Task) {
		t.RequiresApproval = false
		t.Metadata["approved_by"] = approvedBy
		t.NextAction = "approved for execution"
	})
	if err != nil {
		return Task{}, err
	}
	return o.activateTask(task.TaskID)
}

func (o *Orchestrator) DenyTask(taskID, deniedBy, reason string) (Task, error) {
	return o.store.MoveTask(taskID, TaskStateFailed, func(t *Task) {
		t.FailureReason = firstNonEmpty(reason, "denied")
		t.NextAction = "task denied"
		t.Metadata["denied_by"] = deniedBy
	})
}

func (o *Orchestrator) RequeueTask(taskID, trigger string, mutate func(*Task)) (Task, error) {
	task, err := o.store.GetTask(taskID)
	if err != nil {
		return Task{}, err
	}
	switch task.State {
	case TaskStatePlanned, TaskStateActive, TaskStateBlocked, TaskStateFailed, TaskStateAwaitingApproval:
	default:
		return Task{}, fmt.Errorf("task %s cannot be requeued from state %s", taskID, task.State)
	}

	state, err := o.store.LoadState()
	if err != nil {
		return Task{}, err
	}
	if state.ActiveTaskID != nil && *state.ActiveTaskID == taskID {
		state.ActiveTaskID = nil
		state.Status = "idle"
		if err := o.store.SaveState(state); err != nil {
			return Task{}, err
		}
	}

	return o.store.MoveTask(taskID, TaskStatePlanned, func(t *Task) {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		delete(t.Metadata, "approval_token")
		delete(t.Metadata, "last_action_id")
		t.Metadata["rerun_count"] = incrementStringCounter(t.Metadata["rerun_count"])
		t.Metadata["last_rerun_trigger"] = firstNonEmpty(trigger, "manual")
		t.Metadata["last_rerun_from_state"] = task.State
		t.Metadata["last_rerun_at"] = time.Now().UTC().Format(time.RFC3339)
		t.FailureReason = ""
		t.NextAction = "rerun requested"
		if mutate != nil {
			mutate(t)
		}
	})
}

func (o *Orchestrator) planTask(taskID string) (Task, error) {
	if _, err := o.store.GetTask(taskID); err != nil {
		return Task{}, err
	}
	// Always promote inbox → planned without an orchestrator-level “plan approval”
	// gate. Root / risky work is still gated inside the skill runner (e.g. root
	// file writes) where real approvals belong.
	return o.store.MoveTask(taskID, TaskStatePlanned, func(t *Task) {
		if strings.TrimSpace(t.SelectedSkill) == "" {
			t.SelectedSkill = suggestSkill(*t)
		}
		t.NextAction = "ready for execution"
	})
}

func (o *Orchestrator) activateTask(taskID string) (Task, error) {
	task, err := o.store.MoveTask(taskID, TaskStateActive, func(t *Task) {
		t.NextAction = "execute via selected skill"
	})
	if err != nil {
		return Task{}, err
	}

	state, err := o.store.LoadState()
	if err != nil {
		return Task{}, err
	}
	state.Status = "busy"
	state.ActiveTaskID = &task.TaskID
	if err := o.store.SaveState(state); err != nil {
		return Task{}, err
	}
	return task, nil
}

func suggestSkill(task Task) string {
	switch {
	case containsAny(task.Goal, "搜索", "查一下", "查找", "联网", "网页", "网站", "资料"):
		return "web-search"
	case containsAny(task.Goal, "github", "GitHub") && containsAny(task.Goal, "issue", "issues", "问题", "工单"):
		return "github-issue-search"
	case containsAny(task.Goal, "邮件", "邮箱", "收件箱"):
		return "email-inbox"
	case containsWord(task.Goal, "email") || containsWord(task.Goal, "mail") || containsWord(task.Goal, "inbox"):
		return "email-inbox"
	case containsWord(task.Goal, "github") && containsWord(task.Goal, "issue"):
		return "github-issue-search"
	case containsWord(task.Goal, "search"), containsWord(task.Goal, "web"):
		return "web-search"
	case containsWord(task.Goal, "shell"), containsWord(task.Goal, "command"), containsWord(task.Goal, "run"):
		return "shell-command"
	case containsWord(task.Goal, "memory"), containsWord(task.Goal, "recall"):
		return "memory-consolidate"
	case looksLikeCreateDirectoryRequest(task.Goal):
		return "shell-command"
	case containsWord(task.Goal, "read") && referencesFileObject(task.Goal):
		return "file-read"
	case referencesFileObject(task.Goal) && (containsWord(task.Goal, "edit") || containsWord(task.Goal, "write") || containsWord(task.Goal, "update") || containsAny(task.Goal, "创建", "新建", "生成")):
		return "file-edit"
	default:
		return "task-plan"
	}
}

func containsWord(text, word string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(word))
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if containsWord(text, value) {
			return true
		}
	}
	return false
}

func referencesFileObject(text string) bool {
	return containsWord(text, "file") ||
		containsWord(text, "document") ||
		containsWord(text, "doc") ||
		containsWord(text, "readme") ||
		containsWord(text, "note") ||
		strings.Contains(text, "文件") ||
		strings.Contains(text, "文档") ||
		strings.Contains(text, "笔记")
}

// looksLikeCreateDirectoryRequest matches Chinese/short phrases for making a
// folder (not creating a file inside a path that happens to contain "目录").
func looksLikeCreateDirectoryRequest(goal string) bool {
	goal = strings.TrimSpace(goal)
	if strings.Contains(goal, "文件") {
		return false
	}
	if !strings.Contains(goal, "目录") && !strings.Contains(goal, "文件夹") {
		return false
	}
	return containsAny(goal, "创建", "新建", "建立")
}

func incrementStringCounter(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "1"
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil || value < 0 {
		return "1"
	}
	return fmt.Sprintf("%d", value+1)
}
