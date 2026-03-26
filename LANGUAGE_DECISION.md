# MnemosyneOS Language Decision

## Decision

Keep the platform runtime in `Go`, keep the SDK and experimentation layer in `Python`, and reserve `Rust` for future safety-critical or performance-critical subsystems.

## Why not switch the whole platform to Rust now

The current project risks are dominated by:

- runtime architecture
- task and skill orchestration
- memory design
- browser integration
- permission control
- testbed automation

These are product and systems-integration problems more than low-level performance problems.

Switching the entire platform to Rust now would slow down:

- API iteration
- runtime prototyping
- connector development
- skill integration
- VM and Raspberry Pi validation

## Why Go remains the best platform language right now

Go fits the current needs well:

- long-running services
- HTTP APIs
- process orchestration
- filesystem-heavy tooling
- concurrent runtime coordination
- operational simplicity on Linux/macOS

It is a strong fit for the platform control plane.

## Why Python stays in the stack

Python remains useful for:

- SDKs
- experiments
- skill prototypes
- connector prototypes
- model-provider glue

Python lowers iteration cost without forcing the core runtime to become less structured.

## Where Rust does make sense later

Rust should be introduced only where it provides a clear benefit:

- privileged executor
- sandbox runner
- low-level OS bridge
- high-performance journal or index components
- security-sensitive local agents

## Recommended language split

### Go

Use for:

- AI user runtime
- control plane
- task scheduler
- memory orchestration
- policy engine
- connector manager
- browser/session coordinator
- local console and runtime API

### Python

Use for:

- SDK
- experimental skills
- research pipelines
- integration scripts
- model and tool experimentation

### Rust

Use only when needed for:

- execution plane hardening
- root-capable executor
- sandbox isolation
- low-level performance hotspots

## Architectural rule

Do not let future Rust adoption force a rewrite of the platform control plane.

Instead:

- define interfaces now
- keep the execution plane replaceable
- allow Go implementations first
- upgrade selected components to Rust only when justified

## Final position

The project should remain:

- `Go` for the platform runtime
- `Python` for SDK and experimentation
- `Rust` as an optional future execution-core language
