package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/connectors"
	"mnemosyneos/internal/fsaccess"
	"mnemosyneos/internal/model"
	"mnemosyneos/internal/recall"
)

// AgentSkill describes a capability the LLM can invoke via function calling.
type AgentSkill struct {
	Name        string
	Description string
	Parameters  map[string]any
	Handler     AgentSkillHandler
}

// AgentSkillHandler executes an agent skill and returns a textual result
// that will be sent back to the LLM as a tool response.
type AgentSkillHandler func(ctx context.Context, args map[string]any) (string, error)

// AgentSkillRegistry holds all agent-callable skills and converts them to
// model.ToolDefinition for the OpenAI function-calling API.
type AgentSkillRegistry struct {
	skills []AgentSkill
	index  map[string]AgentSkillHandler
}

func NewAgentSkillRegistry() *AgentSkillRegistry {
	return &AgentSkillRegistry{
		index: make(map[string]AgentSkillHandler),
	}
}

func (r *AgentSkillRegistry) Register(skill AgentSkill) {
	if r == nil {
		return
	}
	r.skills = append(r.skills, skill)
	r.index[skill.Name] = skill.Handler
}

// ToolDefinitions returns the OpenAI-compatible tool definitions for all
// registered skills, ready to be passed to model.TextRequest.Tools.
func (r *AgentSkillRegistry) ToolDefinitions() []model.ToolDefinition {
	if r == nil {
		return nil
	}
	defs := make([]model.ToolDefinition, 0, len(r.skills))
	for _, skill := range r.skills {
		defs = append(defs, model.ToolDefinition{
			Type: "function",
			Function: model.ToolFunction{
				Name:        skill.Name,
				Description: skill.Description,
				Parameters:  skill.Parameters,
			},
		})
	}
	return defs
}

// Execute runs the named skill with the given JSON arguments.
func (r *AgentSkillRegistry) Execute(ctx context.Context, name string, rawArgs string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("agent skill registry is nil")
	}
	handler, ok := r.index[name]
	if !ok {
		return "", fmt.Errorf("unknown agent skill: %s", name)
	}
	var args map[string]any
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for %s: %w", name, err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	// Recover from panics in skill handlers so a single broken skill
	// doesn't crash the entire server.
	var (
		result string
		err    error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("skill %s panicked: %v", name, r)
			}
		}()
		result, err = handler(ctx, args)
	}()
	return result, err
}

// RegisterBuiltinAgentSkills registers the 7 built-in agent skills that let
// the LLM autonomously interact with the user's system.
func RegisterBuiltinAgentSkills(reg *AgentSkillRegistry, opts BuiltinSkillOpts) {
	if reg == nil {
		return
	}
	reg.Register(AgentSkill{
		Name:        "get_system_info",
		Description: "Get information about the MnemosyneOS system: workspace path, runtime root, version, server address, and current time.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: makeGetSystemInfoHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "read_file",
		Description: "Read a file under the workspace or runtime root. Returns the text content or an error.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path to read. Relative paths are resolved under the workspace; absolute paths must stay under the workspace or runtime root.",
				},
			},
			"required": []string{"path"},
		},
		Handler: makeReadFileHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "list_directory",
		Description: "List files and directories. Read-only and broad by default: any absolute path the OS lets the user read is fair game (~, ~/Projects, /Users/..., /opt, etc). If the user refers to \"this directory\" you MUST pass its absolute path. Omitting path lists the current project workspace.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute directory to list, or a workspace-relative path. ~ and ~/subdir are expanded. Empty = workspace root.",
				},
			},
		},
		Handler: makeListDirectoryHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "search_files",
		Description: "Search for files or directories by name pattern. Read-only and broad by default (any absolute path the OS lets the user read). USE THIS whenever the user asks to find / locate / 查找 / 搜索 a file or folder — do not answer from memory. Supports glob (*.go, README*) and substring match. If a first search under the project workspace returns 0 matches, RETRY with directory=\"~\" before telling the user it wasn't found.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "File or directory name / glob to match (e.g. 'config.json', '*.py', 'lab').",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Root to search under. Defaults to the current project workspace. Use \"~\" to search the user's home, or any absolute path.",
				},
				"max_depth": map[string]any{
					"type":        "integer",
					"description": "Max directory depth to recurse. Defaults to 5; bump to 8 for home-wide searches.",
				},
			},
			"required": []string{"pattern"},
		},
		Handler: makeSearchFilesHandler(opts),
	})
	if agentShellToolEnabled() {
		reg.Register(AgentSkill{
			Name: "run_command",
			Description: "Execute a shell command and return stdout/stderr. Useful for running tests, build scripts, or quick experiments " +
				"(pytest, go test, npm run, python script.py, ls, grep ...). For heavyweight or high-risk operations (rm -rf, deploy, long trainings) " +
				"prefer creating a task — it gives the user an approval gate and persistent artifacts. Default timeout is 5 minutes; set timeout_ms up to 30 minutes for slower runs. " +
				"Always report the exit code and a summary of output back to the user so they can see what really happened.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The full shell command to run (passed to /bin/sh -c).",
					},
					"workdir": map[string]any{
						"type":        "string",
						"description": "Working directory. Defaults to the project workspace. Absolute paths OK when MNEMOSYNE_FILESYSTEM_UNRESTRICTED=true or under MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS.",
					},
					"timeout_ms": map[string]any{
						"type":        "integer",
						"description": "Command timeout in milliseconds. Default 300000 (5 min). Max 1800000 (30 min). Anything longer should be a task, not an agent-loop tool call.",
					},
				},
				"required": []string{"command"},
			},
			Handler: makeRunCommandHandler(opts),
		})
	}
	reg.Register(AgentSkill{
		Name: "write_file",
		Description: "Create or overwrite a file with the given content. Use this to author scripts, write small config files, or save experiment inputs. " +
			"WARNING: this overwrites any existing file at path — prefer append_file when you want to extend. For mass edits, iterative refactors, or anything the user asked you to change in existing source code, " +
			"create a task (file-edit skill) instead so the change is audited and reversible.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Target file path. Relative = under workspace. Absolute paths must be under workspace / runtime / temp / MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS, or anywhere when MNEMOSYNE_FILESYSTEM_UNRESTRICTED=true.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The exact bytes to write. Max 256KB per call; split larger files across multiple append_file calls.",
				},
				"create_dirs": map[string]any{
					"type":        "boolean",
					"description": "Create parent directories if they don't exist. Defaults to true.",
				},
			},
			"required": []string{"path", "content"},
		},
		Handler: makeWriteFileHandler(opts),
	})
	reg.Register(AgentSkill{
		Name: "append_file",
		Description: "Append content to the end of a file. Use for iteratively building up a script, log, or experiment journal without rewriting the whole file. Creates the file if missing. Same allowed-path rules as write_file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Target file path (same rules as write_file).",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Bytes to append. Max 256KB per call.",
				},
				"create_dirs": map[string]any{
					"type":        "boolean",
					"description": "Create parent directories if they don't exist. Defaults to true.",
				},
			},
			"required": []string{"path", "content"},
		},
		Handler: makeAppendFileHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "web_search",
		Description: "Search the web for information on a topic. Returns a list of results with titles, URLs, and snippets.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query.",
				},
			},
			"required": []string{"query"},
		},
		Handler: makeWebSearchHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "recall_memory",
		Description: "Search long-term memory for relevant facts, procedures, or past events. Returns matching memory cards.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "What to recall from memory.",
				},
			},
			"required": []string{"query"},
		},
		Handler: makeRecallMemoryHandler(opts),
	})
	reg.Register(AgentSkill{
		Name:        "list_tasks",
		Description: "List recent tasks in the runtime. Returns task IDs, titles, states, and selected skills.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max number of tasks to return. Defaults to 10.",
				},
			},
		},
		Handler: makeListTasksHandler(opts),
	})
}

// BuiltinSkillOpts holds the dependencies needed by built-in agent skills.
type BuiltinSkillOpts struct {
	WorkspaceRoot string
	RuntimeRoot   string
	Version       string
	Addr          string
	Connectors    *connectors.Runtime
	RecallService *recall.Service
	RuntimeStore  *airuntime.Store
}

// Handlers

func makeGetSystemInfoHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		extra := fsaccess.ExtraRootsFromEnv()
		extraJoined := strings.Join(extra, ", ")
		if extraJoined == "" {
			extraJoined = "(none)"
		}
		info := map[string]string{
			"workspace_root":              opts.WorkspaceRoot,
			"runtime_root":                opts.RuntimeRoot,
			"version":                     opts.Version,
			"server_address":              opts.Addr,
			"current_time":                time.Now().Format(time.RFC3339),
			"os":                          runtime.GOOS,
			"filesystem_unrestricted":     fmt.Sprintf("%t", fsaccess.UnrestrictedFromEnv()),
			"filesystem_unrestricted_env": "MNEMOSYNE_FILESYSTEM_UNRESTRICTED",
			"filesystem_extra_roots_env":  "MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS",
			"filesystem_extra_roots":      extraJoined,
		}
		raw, _ := json.MarshalIndent(info, "", "  ")
		return string(raw), nil
	}
}

func makeReadFileHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		path := stringArg(args, "path")
		if path == "" {
			return "", fmt.Errorf("path is required")
		}
		resolved, err := resolveAgentReadPath(path, opts)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", resolved, err)
		}
		content := string(data)
		if len(content) > 8000 {
			content = content[:8000] + "\n\n... (truncated, file is larger)"
		}
		return content, nil
	}
}

func makeListDirectoryHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		path := stringArg(args, "path")
		if path == "" {
			path = opts.WorkspaceRoot
		}
		resolved, err := resolveAgentReadPath(path, opts)
		if err != nil {
			return "", err
		}
		entries, err := os.ReadDir(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot list %s: %w", resolved, err)
		}
		var b strings.Builder
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			b.WriteString(name)
			b.WriteByte('\n')
		}
		return strings.TrimSpace(b.String()), nil
	}
}

func makeSearchFilesHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		locale := localeFromArgs(args)
		pattern := stringArg(args, "pattern")
		if pattern == "" {
			return "", fmt.Errorf("pattern is required")
		}
		dir := stringArg(args, "directory")
		if dir == "" {
			dir = opts.WorkspaceRoot
		}
		resolvedDir, err := resolveAgentReadPath(dir, opts)
		if err != nil {
			return "", err
		}

		maxDepth := intArg(args, "max_depth", 5)
		if maxDepth < 1 {
			maxDepth = 1
		}
		if maxDepth > 10 {
			maxDepth = 10
		}

		const maxResults = 30
		var matches []string
		baseDepth := strings.Count(filepath.Clean(resolvedDir), string(filepath.Separator))

		_ = filepath.WalkDir(resolvedDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - baseDepth
			if d.IsDir() {
				if depth > maxDepth {
					return filepath.SkipDir
				}
				if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "__pycache__" || d.Name() == ".venv" {
					return filepath.SkipDir
				}
				return nil
			}
			matched, _ := filepath.Match(pattern, d.Name())
			if !matched {
				matched = strings.Contains(strings.ToLower(d.Name()), strings.ToLower(pattern))
			}
			if matched {
				matches = append(matches, path)
				if len(matches) >= maxResults {
					return fmt.Errorf("limit reached")
				}
			}
			return nil
		})

		if len(matches) == 0 {
			return localize(locale, fmt.Sprintf("在 %s 下没有找到匹配 \"%s\" 的文件（搜索深度 %d 层）。", resolvedDir, pattern, maxDepth), fmt.Sprintf("No files matching %q were found under %s (max depth: %d).", pattern, resolvedDir, maxDepth)), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, localize(locale, "找到 %d 个匹配文件：\n\n", "Found %d matching files:\n\n"), len(matches))
		for i, m := range matches {
			fmt.Fprintf(&b, "%d. %s\n", i+1, m)
		}
		if len(matches) >= maxResults {
			b.WriteString(localize(locale, "\n（结果已截断，可能还有更多匹配）", "\n(Results truncated; more matches may exist.)"))
		}
		return b.String(), nil
	}
}

func makeRunCommandHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		command := stringArg(args, "command")
		if command == "" {
			return "", fmt.Errorf("command is required")
		}
		workdir := stringArg(args, "workdir")
		if workdir == "" {
			workdir = opts.WorkspaceRoot
		}
		resolvedWorkdir, err := resolveAgentPath(workdir, opts)
		if err != nil {
			return "", err
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(timeoutCtx, "sh", "-c", command)
		cmd.Dir = resolvedWorkdir
		output, err := cmd.CombinedOutput()
		result := strings.TrimSpace(string(output))
		if len(result) > 4000 {
			result = result[:4000] + "\n\n... (truncated)"
		}
		if err != nil {
			return fmt.Sprintf("Exit error: %s\nOutput:\n%s", err.Error(), result), nil
		}
		return result, nil
	}
}

func makeWebSearchHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		locale := localeFromArgs(args)
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		if opts.Connectors == nil {
			return webSearchNotConfiguredMessage(locale), nil
		}
		searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		resp, err := opts.Connectors.Search(searchCtx, connectors.SearchRequest{
			Query: query,
			Limit: 5,
		})
		if err != nil {
			if errors.Is(err, connectors.ErrConnectorUnavailable) {
				return webSearchNotConfiguredMessage(locale), nil
			}
			return localize(locale, fmt.Sprintf("搜索出错: %s", err.Error()), fmt.Sprintf("Search failed: %s", err.Error())), nil
		}
		if len(resp.Results) == 0 {
			return localize(locale, "没有找到相关结果。", "No relevant results found."), nil
		}
		var b strings.Builder
		for i, r := range resp.Results {
			fmt.Fprintf(&b, "%d. %s\n   URL: %s\n", i+1, r.Title, r.URL)
			if r.Snippet != "" {
				snippet := r.Snippet
				if len(snippet) > 300 {
					snippet = snippet[:300] + "..."
				}
				fmt.Fprintf(&b, "   %s\n", snippet)
			}
			b.WriteByte('\n')
		}
		return strings.TrimSpace(b.String()), nil
	}
}

func webSearchNotConfiguredMessage(locale string) string {
	return localize(locale, `[功能未配置] 网络搜索功能尚未启用。

请告诉用户：网络搜索需要配置 API 密钥才能使用。配置方法：
1. 在项目根目录的 .env.local 文件中添加：
   MNEMOSYNE_WEB_SEARCH_PROVIDER=tavily
   MNEMOSYNE_WEB_SEARCH_API_KEY=你的Tavily密钥
2. 重启 MnemosyneOS（mnemosynectl restart）

推荐使用 Tavily（https://tavily.com），也支持 SerpAPI（https://serpapi.com）。`, `[Feature not configured] Web search is not enabled.

Tell the user web search requires an API key:
1. Add to .env.local in the project root:
   MNEMOSYNE_WEB_SEARCH_PROVIDER=tavily
   MNEMOSYNE_WEB_SEARCH_API_KEY=your_tavily_key
2. Restart MnemosyneOS (mnemosynectl restart)

Recommended provider: Tavily (https://tavily.com), SerpAPI also supported (https://serpapi.com).`)
}

func makeRecallMemoryHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		locale := localeFromArgs(args)
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		if opts.RecallService == nil {
			return localize(locale, "记忆系统暂未就绪，目前还没有可检索的记忆内容。你可以先正常使用，系统会自动积累记忆。", "Memory is not ready yet, so there is nothing to recall. Keep using the system and memories will accumulate."), nil
		}
		resp := opts.RecallService.Recall(recall.Request{
			Query: query,
			Limit: 5,
		})
		if len(resp.Hits) == 0 {
			return localize(locale, "没有找到相关记忆。", "No relevant memories found."), nil
		}
		var b strings.Builder
		for i, hit := range resp.Hits {
			fmt.Fprintf(&b, "%d. [%s] %s\n   %s\n\n", i+1, hit.CardType, hit.CardID, hit.Snippet)
		}
		return strings.TrimSpace(b.String()), nil
	}
}

func makeListTasksHandler(opts BuiltinSkillOpts) AgentSkillHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		locale := localeFromArgs(args)
		if opts.RuntimeStore == nil {
			return localize(locale, "任务系统暂未就绪。", "Task runtime is not ready."), nil
		}
		limit := intArg(args, "limit", 10)
		tasks, err := opts.RuntimeStore.ListTasks()
		if err != nil {
			return localize(locale, fmt.Sprintf("任务列表读取失败：%s", err.Error()), fmt.Sprintf("Failed to list tasks: %s", err.Error())), nil
		}
		if len(tasks) == 0 {
			return localize(locale, "当前没有任务。", "No tasks found."), nil
		}
		if limit > 0 && len(tasks) > limit {
			tasks = tasks[:limit]
		}
		var b strings.Builder
		for i, t := range tasks {
			fmt.Fprintf(&b, "%d. [%s] %s (skill: %s, id: %s)\n",
				i+1, t.State, t.Title,
				firstNonEmpty(t.SelectedSkill, "none"), t.TaskID)
		}
		return strings.TrimSpace(b.String()), nil
	}
}

// Helpers

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func intArg(args map[string]any, key string, fallback int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return fallback
}

// resolveAgentPath is used by write / exec tools (file-edit, run_command).
// Strict by default: workspace, runtime, temp, MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS
// or MNEMOSYNE_FILESYSTEM_UNRESTRICTED=true.
func resolveAgentPath(path string, opts BuiltinSkillOpts) (string, error) {
	return resolveAgentPathWithPolicy(path, opts, false)
}

// resolveAgentReadPath is used by read-only tools (list_directory,
// search_files, read_file). It still does ~ expansion + symlink resolution,
// but falls back to IsReadPathAllowed so any absolute path the OS lets the
// process read is accepted. This matches the OpenClaw-like personal agent
// model: reads are bounded by OS permissions, writes remain gated.
func resolveAgentReadPath(path string, opts BuiltinSkillOpts) (string, error) {
	return resolveAgentPathWithPolicy(path, opts, true)
}

func resolveAgentPathWithPolicy(path string, opts BuiltinSkillOpts, readOnly bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = opts.WorkspaceRoot
	}
	var err error
	path, err = fsaccess.ExpandHomeDir(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(opts.WorkspaceRoot, path)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// EvalSymlinks only works for existing paths. For read tools this is
	// fine (we're about to stat/read), and for write tools we already assume
	// an existing parent (the original behaviour).
	if eval, evalErr := filepath.EvalSymlinks(resolved); evalErr == nil {
		resolved = eval
	} else if !readOnly {
		return "", evalErr
	}
	allowed := fsaccess.IsPathAllowed(resolved, opts.WorkspaceRoot, opts.RuntimeRoot)
	if !allowed && readOnly {
		allowed = fsaccess.IsReadPathAllowed(resolved, opts.WorkspaceRoot, opts.RuntimeRoot)
	}
	if !allowed {
		return "", fmt.Errorf("path %s is outside allowed roots", resolved)
	}
	return resolved, nil
}

func localeFromArgs(args map[string]any) string {
	raw := stringArg(args, "_locale")
	if strings.EqualFold(raw, "en") {
		return "en"
	}
	return "zh"
}

func localize(locale, zh, en string) string {
	if strings.EqualFold(strings.TrimSpace(locale), "en") {
		return en
	}
	return zh
}
