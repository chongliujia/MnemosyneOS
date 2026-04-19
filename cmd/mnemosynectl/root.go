package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mnemosyneos/internal/appcli"
)

func printRootHelp() {
	fmt.Fprint(os.Stdout, `MnemosyneOS CLI

Primary commands:
  init            Bootstrap runtime, model config, .env.local — then starts the API in the background (use --no-start to skip)
  doctor          Validate local runtime + optional model connectivity probe
  serve           Run the HTTP server in the foreground (also used by the daemon)
  start           Start background daemon (spawns serve)
  stop            Stop background daemon
  restart         Restart background daemon
  logs            Tail daemon log file
  chat            Interactive terminal chat (default when no subcommand)
  ask <message>   One-shot question

Examples:
  mnemosynectl init                         # TTY: guided prompts (or use -i / -interactive)
  mnemosynectl init --provider deepseek --api-key "$DEEPSEEK_API_KEY"
  mnemosynectl init --provider siliconflow --api-key "$SILICONFLOW_API_KEY" --model Qwen/Qwen2.5-7B-Instruct
  mnemosynectl init --provider openai-compatible --base-url https://api.example.com/v1 --api-key "$KEY" --conversation-model gpt-4.1-mini
  mnemosynectl doctor --test-model
  mnemosynectl serve --addr :8080 --ui web

After init (unless --no-start or -json), the API runs in the background like mnemosynectl start. serve/start also load .env.local (or MNEMOSYNE_DOTENV_PATH).

See GETTING_STARTED.md for the full operator flow.
`)
}

func cmdInitCLI(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.SetOutput(os.Stderr)
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory (default $MNEMOSYNE_RUNTIME_ROOT or ./runtime)")
	apiBase := fs.String("api-base", "", "MNEMOSYNE_API_BASE value to write (default http://127.0.0.1:8080)")
	envFile := fs.String("env-file", "", "env file to upsert (default .env.local)")
	provider := fs.String("provider", "", "model provider (deepseek|siliconflow|openai|openai-compatible|none)")
	preset := fs.String("preset", "", "optional model preset label")
	baseURL := fs.String("base-url", "", "model API base URL")
	apiKey := fs.String("api-key", "", "model API key")
	modelAll := fs.String("model", "", "if set, applies to conversation, routing, and skills models")
	conversationModel := fs.String("conversation-model", "", "conversation profile model id")
	routingModel := fs.String("routing-model", "", "routing profile model id")
	skillsModel := fs.String("skills-model", "", "skills profile model id")
	workspaceRoot := fs.String("workspace-root", "", "if set, writes MNEMOSYNE_WORKSPACE_ROOT to the env file (project dir for file tools)")
	filesystemUnrestricted := fs.String("filesystem-unrestricted", "", "if true or false, writes MNEMOSYNE_FILESYSTEM_UNRESTRICTED to the env file (omit to leave unchanged)")
	asJSON := fs.Bool("json", false, "print machine-readable JSON result")
	noStart := fs.Bool("no-start", false, "after init, do not start the background API (for scripts or when you prefer mnemosynectl serve)")
	var interactive bool
	fs.BoolVar(&interactive, "interactive", false, "run the full guided setup in a terminal (also triggered when key flags are missing on a TTY)")
	fs.BoolVar(&interactive, "i", false, "shorthand for -interactive")
	_ = fs.Parse(args)

	opts := initOptions{
		RuntimeRoot:            *runtimeRoot,
		APIBase:                *apiBase,
		EnvFile:                *envFile,
		Provider:               *provider,
		Preset:                 *preset,
		BaseURL:                *baseURL,
		APIKey:                 *apiKey,
		ConversationModel:      *conversationModel,
		RoutingModel:           *routingModel,
		SkillsModel:            *skillsModel,
		WorkspaceRoot:          *workspaceRoot,
		FilesystemUnrestricted: *filesystemUnrestricted,
	}
	if m := *modelAll; m != "" {
		opts.ConversationModel = m
		opts.RoutingModel = m
		opts.SkillsModel = m
	}

	needPrompt := initNeedsInteractivePrompt(opts, interactive)
	if *asJSON && needPrompt {
		fmt.Fprintln(os.Stderr, "init: -json cannot be combined with interactive setup; pass all flags explicitly.")
		os.Exit(2)
	}
	if needPrompt && !isStdinTTY() {
		fmt.Fprintln(os.Stderr, "init: missing required flags, and this process is not attached to an interactive terminal.")
		fmt.Fprintln(os.Stderr, "  Pass --provider, --api-key, and (for openai-compatible) --base-url, or run init from a normal terminal for guided prompts.")
		os.Exit(2)
	}
	if needPrompt && isStdinTTY() {
		runInitInteractive(os.Stdout, os.Stdin, &opts, interactive)
	}

	if err := validateInitOptionsAfterPrompt(opts); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(2)
	}

	result, err := runInit(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		writeJSONTo(os.Stdout, redactedInitResult(result))
		return
	}
	fmt.Fprintf(os.Stdout, "runtime root:     %s\n", result.RuntimeRoot)
	fmt.Fprintf(os.Stdout, "model config:     %s\n", result.ModelConfigPath)
	fmt.Fprintf(os.Stdout, "runtime state:    %s\n", result.RuntimeStatePath)
	fmt.Fprintf(os.Stdout, "env file:         %s\n", result.EnvFile)
	if result.CreatedEnvFile {
		fmt.Fprintln(os.Stdout, "env file:         created")
	}
	if len(result.UpdatedEnvKeys) > 0 {
		fmt.Fprintf(os.Stdout, "env keys updated: %v\n", result.UpdatedEnvKeys)
	}
	if !*asJSON && !*noStart {
		if result.EnvFile != "" && filepath.Clean(result.EnvFile) != ".env.local" {
			if abs, err := filepath.Abs(result.EnvFile); err == nil {
				_ = os.Setenv("MNEMOSYNE_DOTENV_PATH", abs)
			}
		}
		autoStartAfterInit(os.Stderr, result.RuntimeRoot)
		fmt.Fprintln(os.Stdout, "\nNext: run `mnemosynectl` or `mnemosynectl chat` in this directory (CLI talks to the local API).")
		fmt.Fprintln(os.Stdout, "Stop the background API with: mnemosynectl stop   Foreground dev server: mnemosynectl serve")
	} else {
		fmt.Fprintln(os.Stdout, "\nNext: run `mnemosynectl start` (background API) or `mnemosynectl serve` (foreground) from this directory.")
	}
	fmt.Fprintln(os.Stdout, "serve/start load .env.local (or MNEMOSYNE_DOTENV_PATH) so MNEMOSYNE_* settings apply.")
	if result.EnvFile != "" {
		if abs, err := filepath.Abs(result.EnvFile); err == nil && filepath.Clean(result.EnvFile) != ".env.local" {
			fmt.Fprintf(os.Stdout, "Custom env file %q — in a new shell run: export MNEMOSYNE_DOTENV_PATH=%s\n", result.EnvFile, abs)
		}
	}
}

func cmdDoctorCLI(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.SetOutput(os.Stderr)
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	apiBase := fs.String("api-base", "", "API base URL for live health checks")
	envFile := fs.String("env-file", "", "env file path (informational)")
	testModel := fs.Bool("test-model", false, "probe the configured LLM with a tiny completion")
	asJSON := fs.Bool("json", false, "print machine-readable JSON report")
	_ = fs.Parse(args)

	report := runDoctor(doctorOptions{
		RuntimeRoot: *runtimeRoot,
		APIBase:     *apiBase,
		EnvFile:     *envFile,
		TestModel:   *testModel,
	})
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "doctor: encode: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintf(os.Stdout, "status: %s\n", report.Status)
	for _, c := range report.Checks {
		fmt.Fprintf(os.Stdout, "  [%s] %s", c.Status, c.Name)
		if c.Details != "" {
			fmt.Fprintf(os.Stdout, " — %s", c.Details)
		}
		fmt.Fprintln(os.Stdout)
	}
	if report.Status == "fail" {
		os.Exit(1)
	}
}

func cmdServeCLI(args []string) {
	if err := appcli.RunServe(args); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}
