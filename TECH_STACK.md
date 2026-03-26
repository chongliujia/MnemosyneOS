# MnemosyneOS Tech Stack (Phase 1)

## Decision

- Core runtime and memory engine: **Go**
- Client ecosystem and experimentation: **Python SDK**
- Platform model: **filesystem-native AgentOS runtime**
- Privilege model: **user mode + authorized root mode**
- Model strategy: **text-first MVP + optional multimodal extension**

## Why this combination

- Go gives a stable, fast 24/7 runtime for long-lived AgentOS control-plane work.
- Python SDK lowers integration cost for agent tooling, skills, and experiments.
- The split keeps stateful execution in one runtime while preserving flexibility for integrations.
- Filesystem-first source-of-truth keeps the platform auditable, replayable, and token-efficient.
- A text-first default keeps the MVP simpler and cheaper, while optional vision support unlocks GUI-native automation.

## Backend (Go)

- Language: Go 1.22+
- API: `net/http` + JSON
- Runtime model: sync write ACK after journal append, async projections
- System role:
  - AgentOS runtime
  - memory engine
  - task/execution coordinator
  - policy and privilege gate
- Initial storage:
  - In-memory reference implementation (current)
  - Next step: filesystem-backed journal + SQLite (WAL) projections and indexes
- Observability target: OpenTelemetry + Prometheus

## SDK (Python)

- Python: 3.10+
- Transport: HTTP/JSON against Memory VFS API
- Models: Pydantic v2
- HTTP client: httpx (sync + async)

## Model Integration

- Integration mode: provider-agnostic API gateway
- Default MVP path: text-only model workflows
- Extended path: multimodal perception for screenshots, browser pages, and desktop state
- Optional providers:
  - OpenAI-compatible APIs
  - Anthropic
  - Gemini
  - local inference backends such as Ollama or vLLM

## Model Roles

- `text model`:
  - planning
  - memory consolidation
  - shell/file reasoning
  - summarization
  - review and safety checks
- `vision model`:
  - screenshot understanding
  - GUI state inspection
  - browser page visual interpretation
  - desktop workflow perception
- `utility models`:
  - embeddings
  - reranking
  - OCR/ASR post-processing

## Runtime Principles

- Source of truth: files first
- Acceleration layer: SQLite / BM25 / vector indexes
- Identity model: one persistent agent runtime, internally coordinated by multiple agents
- Behavior model: skills as executable action modules
- Interaction model:
  - local console
  - runtime API
  - remote connectors such as mail/webhooks

## Privilege Profiles

- `user mode`: default, normal-user OS automation
- `root mode`: explicitly authorized, auditable privileged execution

## API Contract (current)

- `GET /health`
- `GET /runtime/state`
- `GET /tasks`
- `POST /tasks`
- `GET /tasks/{id}`
- `POST /tasks/{id}/approve`
- `POST /tasks/{id}/deny`
- `POST /tasks/{id}/run`
- `POST /actions/shell`
- `POST /actions/file-read`
- `POST /actions/file-write`
- `GET /actions/{id}`
- `PUT /cards`
- `PATCH /cards/{id}`
- `POST /edges`
- `GET /query`

## Test Strategy

- Primary development loop: local development machine
- Constrained integration: Docker
- VM/system validation: `QEMU`, `UTM`, `libvirt + virt-install + virt-manager`
- Real deployment validation: `Raspberry Pi 5 / ARM64 Linux`

## Near-term upgrades

1. Replace in-memory store with filesystem-backed journal and replayable SQLite projections.
2. Define AI user identity, privilege profiles, and skill contracts.
3. Add a provider-agnostic model gateway with text and vision routing.
4. Add local console, runtime API, and connector interfaces.
5. Add idempotency key and request tracing headers.
6. Add API versioning (`/v1`) and OpenAPI spec.
7. Add Python SDK retries/backoff and typed exceptions.
