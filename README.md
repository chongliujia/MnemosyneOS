# MnemosyneOS

> A Filesystem-Native AgentOS for Linux and macOS
>
> Persistent agents with runtime state, OS automation, governed memory, skills, and controlled privileges.


MnemosyneOS is an AgentOS that lets an agent operate a computer like a persistent system process with memory and execution state:

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

* A research framework for cognitive-inspired AI memory and agent runtimes
* A production-oriented AgentOS for Linux/macOS systems

---

## Quick Start (Go Service + Python SDK)

### 0) Local config

Create a local config file from the example:

```bash
cp .env.example .env.local
```

Both `mnemosyne-api` and `mnemosynectl` automatically load `.env.local` if present.

### 1) Run API service

```bash
go run ./cmd/mnemosyne-api
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

### 2) Use Python SDK

```bash
cd python
pip install -e .
```

```python
from mnemosyne_sdk import CreateCardRequest, MnemosyneClient

client = MnemosyneClient(base_url="http://127.0.0.1:8080")
card = client.create_card(
    CreateCardRequest(
        card_id="evt-1",
        card_type="event",
        content={"text": "User likes black coffee"},
    )
)
print(card.card_id, card.version)
client.close()
```

### 3) Use local console

With the API service running:

```bash
go run ./cmd/mnemosynectl status
go run ./cmd/mnemosynectl ask "Plan the next repository step"
go run ./cmd/mnemosynectl tasks
go run ./cmd/mnemosynectl run <task-id>
go run ./cmd/mnemosynectl recall "approval agentos"
go run ./cmd/mnemosynectl approvals
go run ./cmd/mnemosynectl approve-action <approval-id>
```

Default API base: `http://127.0.0.1:8080`  
Override with env: `MNEMOSYNE_API_BASE=http://127.0.0.1:8090`

### 4) Use the web console

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

### 5) Run the harness

```bash
go run ./cmd/mnemosyne-harness
go run ./cmd/mnemosyne-harness -scenario ./scenarios/email_inbox_summary
go run ./cmd/mnemosyne-harness -tags chat,memory
go run ./cmd/mnemosyne-harness -report-a ./runs/<run-a> -report-b ./runs/<run-b>
go run ./cmd/mnemosyne-harness -rollup ./runs
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -tags execution
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -tags email
```

Current non-GitHub baseline scenarios:

- `web_search_summary`
- `root_approval_flow`
- `chat_followup_continuity`
- `email_inbox_summary`
- `email_followup_continuity`
- `file_read_roundtrip`
- `shell_failure_observability`

Model settings:

- runtime model configuration is persisted at `runtime/model/config.json`
- supported provider shape in this version:
  - `openai-compatible`
- supported presets:
  - `deepseek`
  - `openai`
  - `custom`
- the UI now splits configuration into three profiles:
  - `conversation model`
  - `routing model`
  - `skills/summary model`
- each profile has its own model name and max token budget
- conversation and skills/summary profiles expose independent temperature settings
- routing remains deterministic at temperature `0`
- the `Test Connection` action verifies provider, key, base URL, and selected routing model without restarting the server

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

### 6) Project references

- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- Tech stack decisions: [`TECH_STACK.md`](./TECH_STACK.md)
- Test strategy: [`TEST_STRATEGY.md`](./TEST_STRATEGY.md)
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

## Platform Direction

MnemosyneOS is not just a memory plugin. It is designed as:

* A persistent AgentOS for Linux/macOS
* A filesystem-first automation runtime
* A multi-agent operating loop with memory-aware skills
* A text-first MVP with API-first external retrieval
* A platform developed locally first, then validated with Docker, VMs, and dedicated hardware

---

## Core Architecture

### 1. Journal Layer

Append-only event log
Crash-recoverable
Replayable

### 2. Card Layer

Structured memory units:

* Event Cards (episodic memory)
* Fact Cards (semantic memory)
* Preference Cards
* Plan/Commitment Cards
* Self/Goal Kernel Cards

### 3. Dual Graph Layer

* Episodic Graph (narrative continuity)
* Semantic Graph (knowledge structure)
* Evidence Bridges between them

### 4. Governance Layer

* Consolidation daemon
* Reconsolidation (conflict handling)
* Activation-based decay
* Compaction & log rotation
* Projection rebuild & verification

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
--


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

## Phase 0 – Concept Stabilization

* Define Card schema
* Define Edge types
* Define Dual Graph structure
* Define Memory-VFS API

## Phase 1 – Minimal Memory Core

* Event journal
* Card creation
* Graph linking
* Basic querying
* As-of temporal queries

## Phase 2 – Consolidation Engine

* Replay mechanism
* Event → Fact upgrade rules
* Version chain support
* Conflict detection

## Phase 3 – Governance & Stability

* Activation model
* Decay & compaction
* Rebuildable projections
* Soak testing (24h+)

## Phase 4 – Benchmark Suite

* Temporal correctness benchmark
* Evidence integrity benchmark
* Memory contamination benchmark
* Narrative coherence benchmark

---
