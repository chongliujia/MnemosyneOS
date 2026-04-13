# Project Direction

MnemosyneOS should not be developed as a clone of OpenClaw.

OpenClaw is useful as a reference point for:

- gateway-oriented product shape
- multi-channel interaction
- memory-aware assistant workflows
- integrated developer-facing UX

But MnemosyneOS should optimize for a different outcome:

- industrial-grade agent runtime
- persistent execution state
- auditable actions
- governed memory
- replayable and evaluable behavior

This is the distinction that matters.

## What We Are Not Building

MnemosyneOS is not trying to be:

- a generic chat UI with tools attached
- a workspace clone with more connectors
- a memory plugin with a nicer shell
- a product that wins by feature count alone

If a feature only improves appearance or convenience, but does not improve:

- reliability
- auditability
- replayability
- recoverability
- evaluability
- controllability

then it should usually be treated as lower priority.

## What We Are Actually Building

MnemosyneOS is an industrial AgentOS runtime.

The project should be understood as four stacked layers:

1. `Conversation Layer`
2. `Runtime Layer`
3. `Execution Layer`
4. `Harness + Evaluation Layer`

### 1. Conversation Layer

This layer handles:

- sessions
- dialogue continuity
- routing
- follow-up handling
- model-backed direct reply
- human operator interaction

This layer is important, but it is not the system core.

It is the entry surface.

### 2. Runtime Layer

This layer handles:

- task state machines
- skill orchestration
- approvals
- retries
- recovery
- working memory
- task/result envelopes

This is the operational center of AgentOS.

### 3. Execution Layer

This layer handles:

- shell execution
- file actions
- connector calls
- future sandbox/root execution
- action audit records
- side effects in the environment

This is where industrial reliability matters most.

### 4. Harness + Evaluation Layer

This layer handles:

- scenario execution
- assertions
- replay
- diff
- rollup
- baseline checks
- benchmark lanes

This is the layer that turns the system from a demo into an engineering platform.

## Why We Should Not Copy OpenClaw

If MnemosyneOS follows OpenClaw too closely, the likely failure mode is:

- strong product shell
- weak runtime guarantees
- fragmented execution reliability
- weak regression discipline
- insufficient harness coverage

That would produce a nicer interface, but not a better system.

The project should instead push hardest on the places where industrial systems are judged:

- can it be repeated
- can it be audited
- can it be recovered
- can it be replayed
- can it be evaluated
- can it be controlled

## Core Engineering Principles

These principles should shape implementation decisions.

### 1. Files Are The Durable Truth

Tasks, approvals, actions, artifacts, observations, sessions, and memory should stay inspectable and durable.

Databases and indexes can accelerate the system, but they should not become the only source of truth.

### 2. Execution Must Be Explicit

Side effects should be visible, attributable, and controllable.

That means:

- approval gates
- action records
- runtime traces
- explicit state transitions

### 3. Conversation And Task Execution Must Stay Distinct

Fast conversational interaction and heavy task execution are not the same problem.

They should work together, but they should not collapse into one giant path.

### 4. Memory Must Support Operations, Not Just Recall

Memory is not just for answering questions.

It must support:

- continuity across turns
- continuity across tasks
- evidence-backed recall
- durable summaries
- benchmarkable behavior

### 5. Every Important Capability Should Be Harnessable

If a runtime feature cannot be turned into a scenario with assertions, it is not yet an industrial capability.

## What “Industrial-Grade” Means Here

For this project, industrial-grade means:

- deterministic enough to evaluate
- observable enough to debug
- strict enough to audit
- modular enough to evolve
- reliable enough to run continuously

This implies concrete requirements:

- run records
- replayable scenarios
- baselines
- diffable outputs
- approval paths
- controlled execution contracts
- provider independence

## Product Strategy

The right strategy is:

- build a general runtime core
- enter through strong operator workflows

That means:

- general core
- opinionated first-party workflows
- scenario packs that prove value

The project should not market itself as a vague universal AI platform.

It should position itself as:

`an industrial AgentOS runtime for persistent, auditable, evaluable agents`

## Near-Term Priorities

The current implementation already has a foundation in:

- runtime state
- approvals
- execution
- memory
- recall
- harness

The next steps should stay focused on runtime quality.

### Priority 1: CI-Grade Harness Discipline

Needed work:

- scenario lanes such as `smoke`, `regression`, `soak`
- CI baseline gates
- stable benchmark reporting
- failure triage outputs

### Priority 2: Execution Hardening

Needed work:

- clearer execution contracts
- retry and recovery semantics
- idempotency where appropriate
- stronger root and sandbox boundaries
- better failure observability

### Priority 3: Runtime/Product Separation

Needed work:

- keep chat responsive through fast paths
- keep task execution asynchronous and explicit
- improve session working set behavior
- keep conversation output separate from runtime internals

### Priority 4: Operational Surface

Needed work:

- run-level observability
- metrics and traces
- better execution dashboards
- structured failure inspection

## Decision Rule For New Features

Before adding a major feature, ask:

Does this improve at least one of:

- reliability
- auditability
- replayability
- recoverability
- evaluability
- controllability

If not, it is probably not a near-term priority.

## Final Direction

MnemosyneOS should not try to become “OpenClaw, but with different UI”.

It should become:

- a stronger runtime
- a stronger execution substrate
- a stronger evaluation platform

If that is done correctly, product layers can be added later without weakening the system core.
