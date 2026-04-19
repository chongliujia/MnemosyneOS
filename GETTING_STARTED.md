# Getting Started

MnemosyneOS should be used in three different ways depending on what you are trying to achieve.

Do not think of it only as a chat app.
The more useful mental model is:

- a local AgentOS runtime
- a harness-driven evaluation system
- an extensible open-source core

## Mode 1: Use It As A Local AgentOS

Use this mode when you want a persistent local operator that can:

- chat with you
- search the web
- inspect files
- triage email
- continue tasks across turns
- record artifacts, observations, and memory

### Start

One command does everything:

```bash
go run ./cmd/mnemosynectl
```

This auto-bootstraps the runtime, launches a **background daemon**, and opens the
interactive chat. The daemon keeps running after you exit — like a system service.

```bash
mnemosynectl stop        # Stop the daemon
mnemosynectl start       # Start it again (without entering chat)
mnemosynectl status      # Check if it's running
mnemosynectl logs        # View daemon output
```

The web console is available while the daemon runs:

```text
http://127.0.0.1:8080/ui/chat
```

Optional: pre-configure any chat-completions compatible model. The provider is
the reasoning layer; local work is done by MnemosyneOS skills and, as the tool
surface expands, MCP-backed tools.

```bash
go run ./cmd/mnemosynectl init \
  --provider siliconflow \
  --api-key "$SILICONFLOW_API_KEY" \
  --model "Qwen/Qwen2.5-7B-Instruct"
```

Model credentials are stored in an ignored local config selected by
`MNEMOSYNE_MODEL_CONFIG_PATH`; the committed `runtime/model/config.json` should
stay as a safe `provider=none` default.

For file/code operations, choose a model endpoint that supports
OpenAI-compatible `tool_calls`. `mnemosynectl doctor --test-model` checks both
plain text generation and tool-calling support.

Recommended first checks:

1. Run `go run ./cmd/mnemosynectl doctor --test-model`.
2. Run `go run ./cmd/mnemosynectl chat` and send a simple direct reply message.
3. Try `go run ./cmd/mnemosynectl ask "Plan the next repository step"`.
4. Try `go run ./cmd/mnemosynectl tasks` to list current tasks.
5. Try `go run ./cmd/mnemosynectl recall "approval agentos"` to search memory.
6. Open `/ui/chat` and verify the same session-aware runtime is reachable through the web surface.
7. Review `/ui/tasks`, `/ui/approvals`, `/ui/recall`, and `/ui/memory`.

### What this mode is for

This mode is best for:

- local operator workflows
- iterative development of the runtime
- validating chat continuity and task execution
- checking memory and approval behavior by hand

### What this mode is not for

This mode is not enough by itself to prove industrial reliability.

For that, use the harness.

## Mode 2: Use It As A Harness Platform

Use this mode when you want to validate runtime behavior with repeatable scenarios.

This is the most important mode for industrial development.

### Run the full scenario suite

```bash
go run ./cmd/mnemosyne-harness
```

### Run a single scenario

```bash
go run ./cmd/mnemosyne-harness -scenario ./scenarios/web_search_summary
```

### Run by tags

```bash
go run ./cmd/mnemosyne-harness -tags chat,memory
go run ./cmd/mnemosyne-harness -tags execution
go run ./cmd/mnemosyne-harness -tags email
```

### Roll up results

```bash
go run ./cmd/mnemosyne-harness -rollup ./runs
go run ./cmd/mnemosyne-harness -rollup ./runs -tags chat,memory
```

### Save and check baselines

```bash
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness
./scripts/ci-harness.sh smoke
./scripts/ci-harness.sh regression
./scripts/refresh-harness-baselines.sh
make ci
```

`./scripts/ci-harness.sh` is the local equivalent of the GitHub CI harness gate: it runs the selected lane, writes a rollup JSON, and fails on baseline drift.

### Diff two runs

```bash
go run ./cmd/mnemosyne-harness -report-a ./runs/<run-a> -report-b ./runs/<run-b>
```

### What this mode is for

This mode is best for:

- regression testing
- scenario replay
- benchmark iteration
- approval and execution validation
- comparing runtime behavior across code changes

### Current baseline scenarios

Current non-GitHub baseline scenarios include:

- `web_search_summary`
- `root_approval_flow`
- `chat_followup_continuity`
- `email_inbox_summary`
- `email_followup_continuity`
- `file_read_roundtrip`
- `shell_failure_observability`
- `working_memory_followup`
- `memory_write_recall_roundtrip`
- `approval_memory_boundary`
- `session_recovery_continuity`

See [HARNESS.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/HARNESS.md) for details.

## Mode 3: Use It As An Open-Source Core

Use this mode when you want to build on top of MnemosyneOS instead of only using the default UI.

The project is structured so that contributors can extend:

- connectors
- skills
- execution adapters
- scenarios
- policies
- model routing

### The core layers to understand

Start from these areas:

- runtime orchestration:
  [internal/airuntime](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/airuntime)
- chat and session runtime:
  [internal/chat](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/chat)
- execution plane:
  [internal/execution](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/execution)
- memory and recall:
  [internal/memory](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/memory)
  [internal/recall](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/recall)
- connector runtime:
  [internal/connectors](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/connectors)
- harness:
  [internal/harness](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness)

### Recommended extension path

If you want to extend the system, use this order:

1. Add or modify a scenario in `scenarios/`.
2. Add the required connector, skill, or runtime behavior.
3. Add assertions in the harness.
4. Save a baseline.
5. Compare future runs against that baseline.

This keeps feature work tied to evaluation instead of letting the runtime drift.

## Recommended Development Flow

For core development, this is the most productive loop:

1. Reproduce the behavior in `/ui/chat` or `/ui/tasks`.
2. Turn it into a harness scenario.
3. Add assertions for the expected result.
4. Fix the runtime until the scenario passes.
5. Save or update the baseline intentionally.

This is the workflow that moves MnemosyneOS toward an industrial AgentOS instead of a UI-heavy demo.

## Use The Right Entry Point

Use the right entry point for the job:

- `/ui/chat` when exploring operator behavior
- `mnemosynectl chat` / `mnemosynectl ask` when driving chat from the terminal
- `mnemosynectl tasks` / `mnemosynectl run` when managing tasks
- `mnemosynectl recall` when searching memory
- `mnemosynectl approvals` / `mnemosynectl approve-action` when reviewing root actions
- `mnemosynectl status` when checking overall runtime health
- `mnemosyne-harness` when validating the system as an engineering platform

## Background Service (macOS)

To run MnemosyneOS as a persistent background service on macOS:

```bash
make service-install-macos
```

This builds the binary, installs a launchd plist, and starts the service. Verify:

```bash
curl http://127.0.0.1:8080/health
go run ./cmd/mnemosynectl status
open http://127.0.0.1:8080/ui/chat
```

Uninstall:

```bash
make service-uninstall-macos
```

## Related Documents

- [README.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/README.md)
- [HARNESS.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/HARNESS.md)
- [ARCHITECTURE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/ARCHITECTURE.md)
- [TECH_STACK.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/TECH_STACK.md)
- [EXECUTION_PLANE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/EXECUTION_PLANE.md)
