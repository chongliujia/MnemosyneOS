# MnemosyneOS Execution Plane

## Goal

Separate decision-making from action execution so the AI user runtime can safely operate a computer while preserving auditability, replaceability, and privilege control.

## Core split

The platform should be divided into:

- `control plane`
- `execution plane`

## Control plane

The control plane decides what should happen.

Responsibilities:

- planning
- skill selection
- task state management
- memory retrieval and updates
- policy checks
- approval flows
- connector coordination
- browser orchestration

Recommended implementation:

- `Go`

## Execution plane

The execution plane performs real actions on the operating system.

Responsibilities:

- command execution
- file mutations
- process management
- privilege escalation path
- browser-side actions
- artifact collection
- action-level audit records

Initial implementation:

- Go is acceptable for MVP

Future hardened implementation:

- Rust is a strong candidate

## Why this split matters

This prevents the entire platform from being tightly coupled to the most security-sensitive code paths.

Benefits:

- easier iteration on the runtime
- safer privilege boundary design
- simpler future migration to Rust executor components
- clearer audit trails
- better testing in VM environments

## Required interface

The control plane should talk to the execution plane through stable actions.

Suggested action lifecycle:

1. `prepare_action`
2. `authorize_action`
3. `execute_action`
4. `stream_result`
5. `record_audit`

## Action model

Each execution request should include:

- action id
- task id
- skill id
- execution profile
- requested capability
- command or structured action
- target paths or resources
- timeout
- expected outputs

Each result should include:

- status
- stdout/stderr or structured result
- changed files
- exit code
- execution duration
- privilege level used
- audit metadata

## Privilege handling

The execution plane must support at least:

- `user mode`
- `root mode`

Rules:

- user mode is default
- root mode requires explicit authorization
- privileged actions must be audited
- destructive actions should support snapshot-aware execution in testbeds

## Browser actions inside the execution plane

Browser operations should also be treated as execution actions, for example:

- open page
- click element
- fill field
- submit form
- download file
- capture screenshot

This keeps browser automation inside the same audit and approval model as shell actions.

## Replaceability rule

The control plane must not depend on executor implementation details.

That means:

- no direct shell calls from planning logic
- no direct root operations from memory or planner components
- all risky actions go through execution-plane contracts

## MVP recommendation

Phase 1:

- implement the execution plane in Go
- keep the interface strict
- record all actions to files

Phase 2:

- evaluate Rust for the privileged executor path
- harden isolation and sandbox behavior
- split local execution workers from the main runtime

## Platform rule

The execution plane is where the AI user touches the real machine.

It should be designed as a replaceable, auditable subsystem from day one.
