package airuntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrTaskNotFound        = errors.New("task not found")
	ErrRuntimeInvalidInput = errors.New("invalid runtime input")
)

type Store struct {
	mu        sync.Mutex
	rootDir   string
	statePath string
}

func NewStore(rootDir string) *Store {
	return &Store{
		rootDir:   rootDir,
		statePath: filepath.Join(rootDir, "state", "runtime.json"),
	}
}

func (s *Store) RootDir() string {
	return s.rootDir
}

func (s *Store) LoadState() (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStateLocked()
}

func (s *Store) SaveState(state RuntimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = time.Now().UTC()
	return s.writeJSONFile(s.statePath, state)
}

func (s *Store) CreateTask(req CreateTaskRequest) (Task, error) {
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Goal) == "" {
		return Task{}, fmt.Errorf("%w: title and goal are required", ErrRuntimeInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	task := Task{
		TaskID:           fmt.Sprintf("task-%d", now.UnixNano()),
		Title:            strings.TrimSpace(req.Title),
		Goal:             strings.TrimSpace(req.Goal),
		State:            TaskStateInbox,
		CreatedAt:        now,
		UpdatedAt:        now,
		RequestedBy:      strings.TrimSpace(req.RequestedBy),
		Source:           strings.TrimSpace(req.Source),
		ExecutionProfile: firstNonEmpty(req.ExecutionProfile, "user"),
		RequiresApproval: req.RequiresApproval,
		SelectedSkill:    strings.TrimSpace(req.SelectedSkill),
		Metadata:         req.Metadata,
	}
	if task.Metadata == nil {
		task.Metadata = map[string]string{}
	}

	if err := s.writeTaskLocked(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Store) GetTask(taskID string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getTaskLocked(taskID)
}

func (s *Store) ListTasks() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskDirs := []string{
		TaskStateInbox,
		TaskStatePlanned,
		TaskStateActive,
		TaskStateBlocked,
		TaskStateAwaitingApproval,
		TaskStateDone,
		TaskStateFailed,
	}

	tasks := make([]Task, 0)
	for _, dirName := range taskDirs {
		dirPath := filepath.Join(s.rootDir, "tasks", dirName)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			taskPath := filepath.Join(dirPath, entry.Name())
			var task Task
			if err := s.readJSONFile(taskPath, &task); err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	return tasks, nil
}

func (s *Store) MoveTask(taskID, nextState string, mutate func(*Task)) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, path, err := s.findTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if mutate != nil {
		mutate(&task)
	}
	task.State = nextState
	task.UpdatedAt = time.Now().UTC()

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Task{}, err
	}
	if err := s.writeTaskLocked(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Store) loadStateLocked() (RuntimeState, error) {
	var state RuntimeState
	if err := s.readJSONFile(s.statePath, &state); err != nil {
		return RuntimeState{}, err
	}
	return state, nil
}

func (s *Store) getTaskLocked(taskID string) (Task, error) {
	task, _, err := s.findTaskLocked(taskID)
	return task, err
}

func (s *Store) findTaskLocked(taskID string) (Task, string, error) {
	taskDirs := []string{
		TaskStateInbox,
		TaskStatePlanned,
		TaskStateActive,
		TaskStateBlocked,
		TaskStateAwaitingApproval,
		TaskStateDone,
		TaskStateFailed,
		TaskStateArchived,
	}

	for _, dirName := range taskDirs {
		taskPath := filepath.Join(s.rootDir, "tasks", dirName, taskID+".json")
		if _, err := os.Stat(taskPath); err == nil {
			var task Task
			if err := s.readJSONFile(taskPath, &task); err != nil {
				return Task{}, "", err
			}
			return task, taskPath, nil
		}
	}
	return Task{}, "", ErrTaskNotFound
}

func (s *Store) writeTaskLocked(task Task) error {
	taskPath := filepath.Join(s.rootDir, "tasks", task.State, task.TaskID+".json")
	return s.writeJSONFile(taskPath, task)
}

func (s *Store) writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
