# Harness Architecture

MnemosyneOS should treat the harness as a first-class runtime subsystem, not as a thin testing utility.

The harness exists to answer one question:

Can this AgentOS runtime be trusted to behave the same way tomorrow, after changes, under evaluation?

## Purpose

The harness is responsible for:

- replayable scenario execution
- structured assertions
- durable run records
- diffable outputs
- baseline comparison
- benchmark rollups

This is what turns the project from a demo into an engineering platform.

## Core Model

The harness has five layers:

1. `Scenario Spec`
2. `Runtime Driver`
3. `Inspectors`
4. `Diff + Baseline`
5. `Rollup + Reporting`

### 1. Scenario Spec

Each scenario defines:

- fixtures
- steps
- assertions
- tags

Scenarios are the contract between product behavior and engineering evaluation.

They should describe full workflows, not isolated function calls.

Examples:

- web search then summarize
- root approval then rerun
- chat follow-up continuity
- email triage then recall
- shell failure then observability checks

### 2. Runtime Driver

The runtime driver should execute the same major paths that real users exercise:

- `send_chat`
- `submit_task`
- `run_task`
- `approve_pending`

The driver should not invent a separate fake runtime protocol.

It should drive the same orchestration, execution, memory, and chat layers that the product uses.

### 3. Inspectors

Inspectors are where industrial evaluation becomes meaningful.

The current harness already inspects:

- task state
- selected skill
- approval count
- artifacts
- observations
- session state
- memory cards
- memory edges
- recall hits

This should continue to expand.

The most important inspector split is:

- `execution inspectors`
- `memory inspectors`
- `conversation continuity inspectors`

### 4. Diff + Baseline

Every run should be comparable.

The harness already supports:

- report diff
- artifact diff
- semantic diff for structured artifacts
- saved baselines
- baseline checks

This means a runtime change can be judged by output behavior instead of intuition.

### 5. Rollup + Reporting

A single pass/fail is not enough.

Rollup should answer:

- how many runs passed
- which scenarios regressed
- which assertion types fail most often
- which lanes are unstable

This is the bridge from local engineering to CI.

## Scenario Design Rules

Scenario design should follow these rules:

1. Prefer workflows over unit-like fragments.
2. Assert on durable outputs, not only on chat text.
3. Include memory expectations when memory is part of the workflow.
4. Include session continuity expectations for multi-turn chat.
5. Use fixtures for deterministic connector responses.

## Lanes

The harness should evolve toward three lanes:

- `smoke`
- `regression`
- `soak`

### Smoke

Fast, high-signal checks for every core path.

Examples:

- chat continuity
- web search summary
- root approval flow

### Regression

Broader scenario coverage for normal development gates.

Examples:

- email follow-up continuity
- memory roundtrip
- shell failure observability

### Soak

Long-running or repeated scenarios that validate stability over time.

Examples:

- repeated session continuation
- repeated memory writes and recall
- repeated failure and recovery cycles

## Why Memory Must Be In The Harness

If memory behavior is not part of harness execution, the project cannot claim industrial reliability.

The harness must verify:

- what was written
- what was linked
- what was recalled
- what was kept only in working memory
- what was promoted to long-term memory

This is why memory assertions are part of the runtime contract, not just a product feature.

## Near-Term Harness Priorities

1. Add explicit lane metadata.
2. Expand memory-centered scenarios.
3. Add CI gate usage.
4. Extend failure reporting for baseline regressions.
5. Add soak-style session and recovery scenarios.

## Current Implementation Anchors

Core implementation lives in:

- [internal/harness/types.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness/types.go)
- [internal/harness/loader.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness/loader.go)
- [internal/harness/runner.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness/runner.go)
- [internal/harness/diff.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness/diff.go)
- [internal/harness/rollup.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/harness/rollup.go)
- [cmd/mnemosyne-harness/main.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/cmd/mnemosyne-harness/main.go)
