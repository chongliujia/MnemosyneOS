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

### Start the runtime

Create local config first:

```bash
cp .env.example .env.local
```

Then run the API:

```bash
go run ./cmd/mnemosyne-api
```

Open the web console:

```text
http://127.0.0.1:8080/ui/chat
```

Recommended first checks:

1. Open `/ui/models` and verify provider, model, token budgets, and connectivity.
2. Open `/ui/chat` and send a simple direct reply message.
3. Try one task request such as web search or file inspection.
4. Review `/ui/tasks`, `/ui/approvals`, `/ui/recall`, and `/ui/memory`.

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
```

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
- `mnemosynectl` when driving the runtime directly
- `mnemosyne-harness` when validating the system as an engineering platform

## Related Documents

- [README.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/README.md)
- [HARNESS.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/HARNESS.md)
- [ARCHITECTURE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/ARCHITECTURE.md)
- [TECH_STACK.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/TECH_STACK.md)
- [EXECUTION_PLANE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/EXECUTION_PLANE.md)
