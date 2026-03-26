# MnemosyneOS Test Strategy

## Goal

Define a practical test strategy for an AgentOS that evolves from fast local iteration to controlled isolation and finally to full system validation.

## Core decision

Testing should be staged in this order:

1. local development machine
2. Docker-based constrained integration tests
3. VM-based system tests
4. Raspberry Pi soak and deployment tests

This replaces a VM-first workflow for early development.

## Why local-first

At the current stage, the highest-value feedback loop is:

- fastest iteration
- easiest debugging
- lowest setup overhead
- direct visibility into runtime, tasks, actions, and memory files

Most current platform features can be validated locally:

- runtime state management
- task orchestration
- skill selection
- execution plane MVP
- console/API flows
- browser DOM-first fetching
- memory file writes

## Test layers

### 1. Unit tests

Use for:

- internal packages
- runtime state transitions
- execution plane behavior
- skill runner behavior
- browser parsing logic
- model gateway adapters

Primary tool:

- `go test ./...`

### 2. Local integration tests

Run directly on the development machine.

Use for:

- full API + console flow
- task to skill to action to artifact flow
- browser DOM-first workflows
- filesystem layout validation
- memory persistence checks

This should be the default development path.

### 3. Docker-based constrained tests

Use Docker for repeatable, limited-scope integration where isolation is useful.

Use for:

- execution-plane constraints
- dependency reproducibility
- policy validation in contained environments
- connector service mocks

Do not treat Docker as the final environment for full AgentOS validation.

Docker is useful, but it does not represent:

- desktop state
- full browser user environment
- GUI behavior
- real user-session semantics

### 4. VM-based system tests

Use VM testing when validating:

- full Linux user environments
- browser session realism
- privilege transitions
- snapshot/recovery behavior
- desktop or multi-process interactions

VM testing is important, but it should start after local and Docker-based loops are stable.

### 5. Raspberry Pi deployment tests

Use real hardware for:

- long-running soak tests
- reboot recovery
- constrained-resource behavior
- real device and network behavior
- deployment realism

This is the final validation layer, not the day-to-day development loop.

## Current recommendation

Current default workflow:

1. develop locally
2. run unit and integration tests locally
3. add Docker tests for isolated execution paths
4. defer UTM/QEMU/libvirt until browser and system behaviors require them
5. use Raspberry Pi for later long-run validation

## What Docker should cover

Recommended Docker coverage:

- execution plane smoke tests
- file and command policy checks
- connector mock environments
- reproducible dependency bundles

## What Docker should not be expected to cover

- complete desktop workflows
- realistic GUI automation
- full browser-user interaction fidelity
- long-running user-session realism

## Validation priorities for the current phase

The current phase should focus on:

- runtime correctness
- task lifecycle correctness
- execution-plane auditability
- skill execution correctness
- browser DOM-first correctness
- memory write correctness

## Platform rule

Local-first testing is the default for early AgentOS development.

Isolation and environment realism should be added progressively, not used to slow down the initial build loop.
