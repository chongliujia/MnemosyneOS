package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func initNeedsInteractivePrompt(opts initOptions, forceWizard bool) bool {
	if forceWizard {
		return true
	}
	p := strings.ToLower(strings.TrimSpace(opts.Provider))
	if p == "" {
		return true
	}
	if p == "none" {
		return false
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return true
	}
	if p == "openai-compatible" && strings.TrimSpace(opts.BaseURL) == "" {
		return true
	}
	return false
}

func runInitInteractive(out io.Writer, in io.Reader, opts *initOptions, full bool) {
	rd := bufio.NewReader(in)

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "MnemosyneOS — guided setup")
	if full {
		fmt.Fprintln(out, "Full wizard: model first, then optional paths. Press Enter for [defaults]. Ctrl+C to cancel.")
	} else {
		fmt.Fprintln(out, "We only configure your model here. Runtime data defaults to ./runtime (or $MNEMOSYNE_RUNTIME_ROOT); you are not required to pick a folder now.")
		fmt.Fprintln(out, "To change paths, run: mnemosynectl init -i     Press Enter for [defaults]. Ctrl+C to cancel.")
	}
	fmt.Fprintln(out, "")

	// Model first — avoids leading with paths, which most users never need to think about.
	if full || strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = promptProvider(out, rd, strings.TrimSpace(opts.Provider))
	}

	p := strings.ToLower(strings.TrimSpace(opts.Provider))
	if p == "none" || p == "" {
		opts.Provider = "none"
		fmt.Fprintln(out, "\nSkipping model API details (provider none).")
		if full {
			promptAdvancedPathsAndEnv(out, rd, opts)
		}
		return
	}

	if full || (p == "openai-compatible" && strings.TrimSpace(opts.BaseURL) == "") {
		def := strings.TrimSpace(opts.BaseURL)
		if def == "" {
			def = "https://api.example.com/v1"
		}
		fmt.Fprintln(out, "\nOpenAI-compatible endpoints usually end with /v1 (example: https://api.siliconflow.cn/v1).")
		opts.BaseURL = promptLine(out, rd, "Model base URL", def)
	}

	if full || strings.TrimSpace(opts.APIKey) == "" {
		fmt.Fprintln(out, "\nModel API key (typed characters will be visible — prefer pasting in a private terminal).")
		opts.APIKey = promptLine(out, rd, "API key", strings.TrimSpace(opts.APIKey))
	}

	if full || (strings.TrimSpace(opts.ConversationModel) == "" && strings.TrimSpace(opts.RoutingModel) == "" && strings.TrimSpace(opts.SkillsModel) == "") {
		hint := defaultModelHint(p)
		if hint != "" {
			fmt.Fprintf(out, "\nOptional: one model id for conversation + routing + skills.\nLeave blank to use the provider default (e.g. %s).\n", hint)
		} else {
			fmt.Fprintln(out, "\nOptional: one model id for all profiles (leave blank for provider defaults).")
		}
		m := promptLine(out, rd, "Model id", "")
		if m != "" {
			opts.ConversationModel = m
			opts.RoutingModel = m
			opts.SkillsModel = m
		}
	}

	if full {
		promptAdvancedPathsAndEnv(out, rd, opts)
	} else {
		fmt.Fprintln(out, "\nOptional (saved to your env file for `serve` / `chat` / `doctor`):")
		ws := promptLine(out, rd, "MNEMOSYNE_WORKSPACE_ROOT — project directory for file tools (blank = do not write)", "")
		if strings.TrimSpace(ws) != "" {
			opts.WorkspaceRoot = strings.TrimSpace(ws)
		}
		fsLine := promptLine(out, rd, "Allow unrestricted filesystem for tools? y/N (blank = do not change env)", "N")
		if parseYes(fsLine) {
			opts.FilesystemUnrestricted = "true"
		}
	}

	fmt.Fprintln(out, "\nApplying configuration…")
}

func promptAdvancedPathsAndEnv(out io.Writer, rd *bufio.Reader, opts *initOptions) {
	fmt.Fprintln(out, "\n— Advanced: paths and env file —")
	fmt.Fprintln(out, "Defaults work for most setups; change only if you know you need to.")
	defRT := firstNonEmpty(strings.TrimSpace(opts.RuntimeRoot), resolveRuntimeRoot(""))
	opts.RuntimeRoot = promptLine(out, rd, "Runtime root (tasks, memory, model config on disk)", defRT)
	defAPI := firstNonEmpty(strings.TrimSpace(opts.APIBase), resolveAPIBase(""))
	opts.APIBase = promptLine(out, rd, "MNEMOSYNE_API_BASE for mnemosynectl chat/ask (after serve is up)", defAPI)
	defEnv := firstNonEmpty(strings.TrimSpace(opts.EnvFile), ".env.local")
	opts.EnvFile = promptLine(out, rd, "Env file to update", defEnv)

	cwd, _ := os.Getwd()
	defWS := strings.TrimSpace(opts.WorkspaceRoot)
	if defWS == "" && cwd != "" {
		defWS = cwd
	}
	opts.WorkspaceRoot = promptLine(out, rd, "Project/workspace for file tools (MNEMOSYNE_WORKSPACE_ROOT)", defWS)

	fsLine := promptLine(out, rd, "Unrestricted filesystem (tools may use any absolute path)? y/N", "N")
	if parseYes(fsLine) {
		opts.FilesystemUnrestricted = "true"
	}
}

func parseYes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes" || s == "1" || s == "true" || s == "on"
}

func defaultModelHint(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "deepseek":
		return "deepseek-chat"
	case "siliconflow":
		return "Qwen/Qwen2.5-7B-Instruct (example; pick your SiliconFlow model id)"
	case "openai":
		return "gpt-4o-mini"
	default:
		return ""
	}
}

func promptProvider(out io.Writer, rd *bufio.Reader, current string) string {
	if strings.TrimSpace(current) != "" {
		fmt.Fprintf(out, "Model provider [%s]\n", current)
		fmt.Fprintln(out, "  1) deepseek  2) siliconflow  3) openai  4) openai-compatible  5) none (no LLM)")
		line := promptLine(out, rd, "Choose 1–5 or type provider name", current)
		if p, ok := normalizeProviderChoice(line); ok {
			return p
		}
		return strings.TrimSpace(current)
	}
	fmt.Fprintln(out, "Model provider:")
	fmt.Fprintln(out, "  1) deepseek            — DeepSeek official API")
	fmt.Fprintln(out, "  2) siliconflow         — SiliconFlow (OpenAI-compatible)")
	fmt.Fprintln(out, "  3) openai              — OpenAI")
	fmt.Fprintln(out, "  4) openai-compatible   — Any chat-completions compatible URL")
	fmt.Fprintln(out, "  5) none                — Skip LLM for now (chat uses fallbacks)")
	line := promptLine(out, rd, "Choose 1–5", "1")
	if p, ok := normalizeProviderChoice(line); ok {
		return p
	}
	return "deepseek"
}

func normalizeProviderChoice(line string) (string, bool) {
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return "", false
	}
	switch line {
	case "1", "deepseek":
		return "deepseek", true
	case "2", "siliconflow", "silicon":
		return "siliconflow", true
	case "3", "openai":
		return "openai", true
	case "4", "openai-compatible", "compatible", "custom":
		return "openai-compatible", true
	case "5", "none", "skip", "no":
		return "none", true
	}
	switch line {
	case "deepseek", "siliconflow", "openai", "openai-compatible", "none":
		return line, true
	default:
		return "", false
	}
}

func validateInitOptionsAfterPrompt(opts initOptions) error {
	p := strings.ToLower(strings.TrimSpace(opts.Provider))
	if p == "" {
		return fmt.Errorf("model provider is required (e.g. deepseek, siliconflow, openai, openai-compatible, none)")
	}
	switch p {
	case "none", "deepseek", "siliconflow", "openai", "openai-compatible":
	default:
		return fmt.Errorf("unknown provider %q", opts.Provider)
	}
	if p == "none" {
		return nil
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return fmt.Errorf("model API key is required for provider %q", p)
	}
	if p == "openai-compatible" && strings.TrimSpace(opts.BaseURL) == "" {
		return fmt.Errorf("model base URL is required for provider openai-compatible")
	}
	return nil
}

func promptLine(out io.Writer, rd *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := rd.ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "\n(read error: %v)\n", err)
		return def
	}
	line = strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if line == "" {
		return def
	}
	return line
}
