# MnemosyneOS

> A Filesystem-Native AgentOS for Linux and macOS
>
> Persistent agents with runtime state, OS automation, governed memory, skills, and controlled privileges.

**MnemosyneOS is a persistent AgentOS** that lets an AI agent operate a computer like a system process with dedicated memory, execution state, and controlled privileges. It is *not* a Python library, SDK, or a simple memory plugin.

* Interact through a local console and remote APIs
* Use `skills` as executable behavior modules
* Run with either normal user privileges or authorized root privileges
* Persist tasks, observations, artifacts, and memory as files
* Use multi-agent internal coordination to reduce token usage
* Start with text-model workflows and extend to multimodal operation when GUI perception is needed

Its memory model is built around:

* Structured **Memory Cards**
* Dual Graph architecture (Episodic + Semantic)
* Temporal validity & versioned facts
* Evidence-backed knowledge
* Consolidation & reconsolidation mechanisms
* Activation-driven decay
* Event-sourced journaling

MnemosyneOS is both:

* A production-oriented AgentOS for Linux/macOS systems
* A research framework for cognitive-inspired AI memory and agent runtimes

---

## Quick Start (Go Service)

### 0) One command to start

```bash
go run ./cmd/mnemosynectl
```

That's it. This single command will:
1. Auto-bootstrap the runtime if it doesn't exist
2. Launch a **background daemon** (persists after you close the chat)
3. Drop you into an interactive chat REPL

The daemon runs as a background process — you can close the chat, do other work,
then come back later with `mnemosynectl` or open the web UI anytime.

### Daemon management

```bash
mnemosynectl start          # Start the daemon in the background
mnemosynectl stop           # Stop the daemon
mnemosynectl restart        # Restart the daemon
mnemosynectl status         # Show daemon status, runtime info, tasks
mnemosynectl logs           # View daemon log output
mnemosynectl logs -n 100    # Last 100 lines
```

### Chat anytime

```bash
mnemosynectl                # Opens chat (auto-starts daemon if needed)
mnemosynectl chat           # Same as above
mnemosynectl ask "What's my schedule?"   # One-shot question
```

### Web UI

While the daemon is running, open:

```text
http://127.0.0.1:8080/ui/chat
```

### 0.5) Optional: pre-configure a model

If you want LLM-powered AgentOS behavior, configure any chat-completions
compatible model first. The LLM is the planner/controller; local work is done
by MnemosyneOS skills and, as the tool surface expands, MCP-backed tools.

```bash
go run ./cmd/mnemosynectl init \
  --provider siliconflow \
  --api-key "$SILICONFLOW_API_KEY" \
  --model "Qwen/Qwen2.5-7B-Instruct"
```

`init` writes the active model config to an ignored local file such as
`runtime/model/local.config.json` and records the path in `.env.local` via
`MNEMOSYNE_MODEL_CONFIG_PATH`. Keep the committed `runtime/model/config.json`
as a safe `provider=none` default; do not commit API keys.

For file/code operations, the selected model must support OpenAI-compatible
`tool_calls`. This is a capability requirement, not an OpenAI-only requirement:
DeepSeek, SiliconFlow, OpenAI, or any compatible endpoint can work if the
selected model exposes tool calling. Run `mnemosynectl doctor --test-model` to
check both text generation and tool-calling support.

Or configure later via the web UI at `/ui/models`.

### 1) Advanced: run the server in the foreground

If you prefer to run the server in the foreground (e.g. for debugging):

```bash
go run ./cmd/mnemosynectl serve --ui web
```

Default address: `http://127.0.0.1:8080`  
Override with env: `MNEMOSYNE_ADDR=:8090`
Optional web search provider:
`MNEMOSYNE_WEB_SEARCH_PROVIDER='serpapi'` or `MNEMOSYNE_WEB_SEARCH_PROVIDER='tavily'`
Optional web search API key:
`MNEMOSYNE_WEB_SEARCH_API_KEY='replace_me'`
Optional web search endpoint override:
`MNEMOSYNE_WEB_SEARCH_ENDPOINT='https://serpapi.com/search.json'`
Optional GitHub issue connector:
`MNEMOSYNE_GITHUB_OWNER='owner'`
`MNEMOSYNE_GITHUB_REPO='repo'`
`MNEMOSYNE_GITHUB_TOKEN='replace_me'`
Optional email connector via local Maildir:
`MNEMOSYNE_MAILDIR_ROOT='/path/to/maildir'`
`MNEMOSYNE_MAILDIR_MAILBOX='INBOX'`
Optional email connector via IMAP:
`MNEMOSYNE_EMAIL_PROVIDER='imap'`
`MNEMOSYNE_IMAP_HOST='imap.example.com'`
`MNEMOSYNE_IMAP_PORT='993'`
`MNEMOSYNE_IMAP_USERNAME='agent@example.com'`
`MNEMOSYNE_IMAP_PASSWORD='replace_me'`
Optional root approval token:
`MNEMOSYNE_ROOT_APPROVAL_TOKEN='replace_me_for_root_tests'`

### 2) Use local console

With the API service running:

```bash
go run ./cmd/mnemosynectl doctor
go run ./cmd/mnemosynectl chat
go run ./cmd/mnemosynectl status
go run ./cmd/mnemosynectl ask "你好"
go run ./cmd/mnemosynectl ask "Plan the next repository step"
go run ./cmd/mnemosynectl tasks
go run ./cmd/mnemosynectl run <task-id>
go run ./cmd/mnemosynectl recall "approval agentos"
go run ./cmd/mnemosynectl approvals
go run ./cmd/mnemosynectl approve-action <approval-id>
```

`mnemosynectl ask` now sends a real chat turn to `/chat` and prints the assistant reply.
`mnemosynectl chat` opens an interactive REPL over the same session-aware chat API.

Default API base: `http://127.0.0.1:8080`  
Override with env: `MNEMOSYNE_API_BASE=http://127.0.0.1:8090`

### 3) Use the web console

With the API service running, open:

```text
http://127.0.0.1:8080/dashboard
```

Current pages:

- `/dashboard` for runtime status, recent tasks, approvals, and actions
- `/ui/chat` for a conversation-first, session-aware AgentOS chat surface with SSE updates, AJAX send, session rename/archive, collapsed memory/task context, and streamed model replies
- `/ui/tasks` for task creation, inspection, and rerun
- `/ui/approvals` for root action review and approval context
- `/ui/recall` for unified memory recall
- `/ui/memory` for the latest connector-backed cards plus card/edge detail
- `/ui/models` for runtime model provider, preset, per-profile model selection, max token budgets, temperature tuning, token budgets, and connectivity testing
- `/ui/artifacts/view` for artifact page/raw/download access

### 4) Run the harness

```bash
go run ./cmd/mnemosyne-harness
go run ./cmd/mnemosyne-harness -scenario ./scenarios/email_inbox_summary
go run ./cmd/mnemosyne-harness -tags chat,memory
go run ./cmd/mnemosyne-harness -lane smoke
go run ./cmd/mnemosyne-harness -lane regression -tags memory
go run ./cmd/mnemosyne-harness -report-a ./runs/<run-a> -report-b ./runs/<run-b>
go run ./cmd/mnemosyne-harness -rollup ./runs
go run ./cmd/mnemosyne-harness -rollup ./runs -lane regression
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -tags execution
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -lane smoke
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -tags email
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -lane regression
./scripts/ci-harness.sh smoke
./scripts/ci-harness.sh regression
./scripts/refresh-harness-baselines.sh
make ci
```

CI discipline:

- `make test-go` runs the Go test suite.
- `./scripts/ci-harness.sh <lane>` runs a harness lane, emits a rollup, and fails on baseline drift.
- `./scripts/refresh-harness-baselines.sh` intentionally regenerates committed harness baselines.
- `.github/workflows/ci.yml` gates pull requests on `go test`, `smoke`, and `regression`.

### 6) Background service (macOS launchd, openclaw-style)

One-command background install (runs on login, auto-restart on crash):

```bash
make build-ctl
make service-install-macos
# or directly:
# ./scripts/install-macos-service.sh
```

Defaults:
- Address: `:8080` → `http://127.0.0.1:8080`
- Runtime root: `~/.mnemosyneos/runtime`
- Binary installed to: `~/.mnemosyneos/bin/mnemosynectl`

Verify and use:
```bash
curl http://127.0.0.1:8080/health
go run ./cmd/mnemosynectl chat
open http://127.0.0.1:8080/ui/chat
```

Customize (optional):
```bash
./scripts/install-macos-service.sh --addr :8090 --workspace-root "$(pwd)" --runtime-root "$HOME/.mnemosyneos/runtime"
```

Uninstall:
```bash
make service-uninstall-macos
# or:
# ./scripts/uninstall-macos-service.sh
```

Current non-GitHub baseline scenarios:

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

Model settings:

- committed `runtime/model/config.json` is the safe default and should not contain secrets
- real local model credentials live in the ignored path selected by `MNEMOSYNE_MODEL_CONFIG_PATH`
- supported providers in this version:
  - `deepseek`
  - `siliconflow`
  - `openai`
  - `openai-compatible`
- the UI now splits configuration into three profiles:
  - `conversation model`
  - `routing model`
  - `skills/summary model`
- each profile has its own model name and max token budget
- conversation and skills/summary profiles expose independent temperature settings
- routing remains deterministic at temperature `0`
- the `Test Connection` action verifies provider, key, base URL, and selected routing model without restarting the server
- `mnemosynectl doctor --test-model` also verifies tool-calling support, which is required for AgentOS skills/MCP-style work

Current web search MVP behavior:

- search/web goals map to the `web-search` skill
- the skill uses a configured search API provider
- missing search configuration blocks the task cleanly
- search results are deduplicated into canonical result cards and a summary card

Current connector/runtime MVP behavior:

- `web-search` uses Tavily or SerpAPI through the connector runtime
- `github-issue-search` uses the configured GitHub repository connector
- `email-inbox` uses the configured Maildir or IMAP read-only connector
- `shell-command`, `file-read`, and `file-edit` run through the execution plane
- `recall` provides a unified read path over web/email/github memory cards

Current root approval MVP behavior:

- `root` execution no longer runs directly from task metadata
- root tasks first enter `awaiting_approval` and create an approval request
- approve with `mnemosynectl approve-action <approval-id>` and rerun the task
- this is still a control-plane authorization gate only; it does not yet provide real OS-level privilege escalation

### 5) Project references

- Getting started: [`GETTING_STARTED.md`](./GETTING_STARTED.md)
- Project direction: [`PROJECT_DIRECTION.md`](./PROJECT_DIRECTION.md)
- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- Tech stack decisions: [`TECH_STACK.md`](./TECH_STACK.md)
- Test strategy: [`TEST_STRATEGY.md`](./TEST_STRATEGY.md)
- Harness architecture: [`HARNESS_ARCHITECTURE.md`](./HARNESS_ARCHITECTURE.md)
- Memory architecture: [`docs/memory/MEMORY_ARCHITECTURE.md`](./docs/memory/MEMORY_ARCHITECTURE.md)
- Memory strategy: [`docs/memory/MEMORY_STRATEGY.md`](./docs/memory/MEMORY_STRATEGY.md)
- Working memory spec: [`docs/memory/WORKING_MEMORY_SPEC.md`](./docs/memory/WORKING_MEMORY_SPEC.md)
- Memory orchestration spec: [`docs/memory/MEMORY_ORCHESTRATION_SPEC.md`](./docs/memory/MEMORY_ORCHESTRATION_SPEC.md)
- Memory consolidation spec: [`docs/memory/MEMORY_CONSOLIDATION_SPEC.md`](./docs/memory/MEMORY_CONSOLIDATION_SPEC.md)
- AI user runtime: [`AI_USER_RUNTIME.md`](./AI_USER_RUNTIME.md)
- Skill system: [`SKILL_SYSTEM.md`](./SKILL_SYSTEM.md)
- Interfaces: [`INTERFACES.md`](./INTERFACES.md)
- Language decision: [`LANGUAGE_DECISION.md`](./LANGUAGE_DECISION.md)
- Model gateway: [`MODEL_GATEWAY.md`](./MODEL_GATEWAY.md)
- Web search runtime: [`WEB_SEARCH_RUNTIME.md`](./WEB_SEARCH_RUNTIME.md)
- Execution plane: [`EXECUTION_PLANE.md`](./EXECUTION_PLANE.md)
- Testbed architecture: [`TESTBED_ARCHITECTURE.md`](./TESTBED_ARCHITECTURE.md)
- Harness: [`HARNESS.md`](./HARNESS.md)

---

## Why MnemosyneOS?

Most AI memory systems today:

* Store text chunks in vector databases
* Lack version control
* Cannot handle fact evolution
* Do not model temporal validity
* Struggle with contamination and correction
* Have no long-term governance layer
* Are not designed to act as persistent operating-system users

MnemosyneOS treats memory as part of a larger AgentOS: a structured, evolving cognitive system that can observe, plan, act, remember, and resume work across time.

## Agent Skills System

MnemosyneOS includes a built-in **Agent Skills System** that enables the LLM to autonomously call tools during conversations. This is the AgentOS contract: the LLM reasons and selects tools; skills and MCP-style adapters perform local work under runtime policy.

### How it works

When you chat with MnemosyneOS, the LLM decides whether to:
- **Reply directly** — for greetings, simple questions, conversation
- **Call tools** — when it needs real data (files, directories, web search, etc.)
- **Chain multiple tools** — e.g., first get system info, then list a directory, then read a file

The agent loop runs up to 6 tool-calling rounds before producing a final response.

### Built-in Skills

| Skill | Description |
|-------|-------------|
| `get_system_info` | Workspace path, runtime root, version, server address, current time |
| `read_file` | Read file contents (auto-truncates large files) |
| `list_directory` | List files and directories at a given path |
| `run_command` | Execute shell commands (git, system info, builds, etc.) |
| `web_search` | Search the web via configured search provider |
| `recall_memory` | Search long-term memory for relevant facts and procedures |
| `list_tasks` | List recent tasks in the runtime |

### Example interaction

```
❯ 我的项目在哪里？有哪些文件？

  [调用 get_system_info]
  [调用 list_directory]

  你的 MnemosyneOS 项目位于 /Users/you/My-Github-Project/MnemosyneOS。
  主要文件包括：cmd/, internal/, runtime/, README.md, Makefile, go.mod ...
```

The LLM automatically determines which tools to use and synthesizes a natural-language response from the results.

## Platform Direction

MnemosyneOS is not just a memory plugin. It is designed as:

* A persistent AgentOS for Linux/macOS
* A filesystem-first automation runtime
* A multi-agent operating loop with memory-aware skills
* A text-first MVP with API-first external retrieval
* A platform developed locally first, then validated with Docker, VMs, and dedicated hardware

---

## Core Architecture

### 1. Journal Layer ✅

Append-only JSONL event log (`memory/journal.jsonl`).
Every card create/update/touch/decay is recorded with timestamp, event type, entity ID, and version.
Used for auditing and future crash-recovery replay.

### 2. Card Layer ✅

Structured memory units with versioned history and disk persistence:

* Event Cards (episodic memory)
* Fact Cards (semantic memory)
* Preference Cards
* Plan/Commitment Cards
* Self/Goal Kernel Cards
* Procedure Cards (extracted from repeated task runs)

Cards are persisted as JSON files under `<runtime>/memory/cards/` and survive restarts.

### 3. Dual Graph Layer ✅ (partial)

* Edges with weight, confidence, temporal validity, evidence refs
* Edges persisted as JSON under `<runtime>/memory/edges/`
* `edgesByCard` index for efficient traversal
* Episodic / Semantic graph distinction (not yet enforced, edges are untyped beyond `edge_type`)

### 4. Governance Layer ✅

* **Background scheduler**: `GovernanceScheduler` runs every hour (configurable):
  - Activation-based decay with per-card policies (`session_use` 5× faster, `durable` 5× slower, `never`)
  - Active → Stale → Archived lifecycle based on score thresholds
  - **Event → Fact upgrade** (automatic episodic-to-semantic consolidation)
  - **Conflict detection** (finds contradicting facts, logs warnings)
* Consolidator promotes candidates, supersedes old cards
* `RebuildProjections()` for disaster recovery from journal
* `VerifyIntegrity()` for snapshot-vs-journal consistency checks

### 5. Chat → Memory Bridge ✅

* Every substantive chat exchange automatically creates an episodic **event card**
* Event cards carry: topic, summary, user query, session ID, skill used
* Cards use `session_use` decay policy (fast decay for session-scoped ephemeral memory)
* Governance scheduler periodically upgrades recurring event patterns into **fact cards**
* Memory recall results are injected into LLM prompts (working notes, semantic hits, procedure guidance)

---

```plaintext
mnemosyneos/
│
├── README.md
├── ARCHITECTURE.md
├── ROADMAP.md
├── BENCHMARK.md
│
├── docs/
│   ├── research/
│   ├── design/
│   └── benchmarks/
│
├── core/
│   ├── journal/
│   ├── cards/
│   ├── graph/
│   ├── activation/
│   ├── consolidation/
│   └── governance/
│
├── api/
│   └── memory_vfs/
│
├── benchmarks/
│   ├── temporal_correctness/
│   ├── contamination_tests/
│   ├── narrative_tests/
│   └── stability_tests/
│
└── examples/
    └── agent_integration/
```

---

## Research Goals

MnemosyneOS aims to explore:

* Cognitive memory modeling for AI
* Temporal fact evolution
* Memory contamination resistance
* Dual-channel retrieval (reliable + narrative)
* Long-running AI memory benchmarking

---

## Engineering Goals

* 24/7 long-running memory core
* Persistent AgentOS runtime
* Multi-agent support
* Skill-based behavior execution
* Local console and remote connector support
* Provider-agnostic model API integration
* Optional multimodal perception for screenshots and desktop interaction
* Versioned knowledge graph
* Evidence-backed retrieval
* Minimal explanation subgraph extraction
* Rebuildable indexing
* Horizontal scalability (future)

## Test Environments

The current test strategy is:

* Primary development loop: local development machine
* Constrained integration: Docker
* VM/system validation: `QEMU`, `UTM`, `libvirt + virt-install + virt-manager`
* Real deployment validation: `Raspberry Pi 5 / ARM64 Linux`

---

# 四、ROADMAP

## Phase 0 – Concept Stabilization ✅

* ✅ Define Card schema (`memory.Card`, versioned, temporal validity)
* ✅ Define Edge types (`memory.Edge`, weighted, evidence-backed)
* ✅ Define Dual Graph structure (cards + edges with `edgesByCard` index)
* Define Memory-VFS API

## Phase 1 – Minimal Memory Core ✅

* ✅ Event journal (append-only JSONL at `memory/journal.jsonl`)
* ✅ Card creation, update, touch (version chain, activation tracking)
* ✅ Graph linking (`CreateEdge` with referential integrity)
* ✅ Basic querying (by ID, type, scope, status)
* ✅ As-of temporal queries (`resolveAsOf` with `ValidFrom`/`ValidTo`)
* ✅ **Disk persistence** — cards & edges saved as JSON files, survive restarts
* ✅ **Journal layer** — every mutation appended to `journal.jsonl` for audit trail

## Phase 2 – Consolidation Engine ✅

* ✅ Candidate → Active promotion (`Consolidator.PromoteCandidates`)
* ✅ Supersession chain (superseded cards archived)
* ✅ Version chain support (each update appends a new version)
* ✅ Procedural extraction from repeated task runs (`BuildProcedureCandidates`)
* ✅ **Journal replay** — `ReplayFromJournal()` rebuilds complete state from journal alone (crash recovery)
* ✅ **Integrity verification** — `VerifyIntegrity()` compares live state vs journal replay
* ✅ **Event → Fact upgrade** — `UpgradeEventsToFacts()` clusters event cards by topic, creates fact candidates with evidence refs and edges
* ✅ **Conflict detection** — `DetectConflicts()` finds contradicting fact cards on same topic

## Phase 3 – Governance & Stability ✅

* ✅ Activation model (score, decay policy: `session_use`/`durable`/`never`)
* ✅ Decay & compaction (`DecayAndCompact` with configurable thresholds)
* ✅ **Background governance scheduler** (periodic decay goroutine, auto-started)
* ✅ **Rebuildable projections** — `RebuildProjections()` wipes snapshots, replays journal, rewrites all files
* ✅ **Soak test** — 100 cards, 50 edges, 200 updates, 100 touches, decay + rebuild + integrity verified

## Phase 4 – Benchmark Suite ✅

* ✅ **Temporal correctness** — as-of queries return correct version across validity windows (4/4)
* ✅ **Version chain integrity** — 20-version chain with prev_version links verified
* ✅ **Evidence integrity** — all evidence_refs resolve, all edges point to real cards (15/15)
* ✅ **Memory contamination resistance** — false low-confidence info isolated, majority truth promoted (3/3)
* ✅ **Narrative coherence** — temporal event chains maintain order, edges form correct DAG (20/20)
