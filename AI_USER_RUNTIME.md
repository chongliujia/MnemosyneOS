# MnemosyneOS AI User Runtime

## Goal

Define the persistent runtime that makes the platform behave like one continuous AI user operating a Linux/macOS machine across tasks, sessions, and reboots.

## Runtime identity

The platform presents one outward-facing AI user identity.

That identity has:

- `user_id`
- `display_name`
- `execution_profile`
- `workspace_roots`
- `active_goals`
- `preferences`
- `memory_mounts`

Internally, the runtime is coordinated by multiple agents, but externally it should behave like one user.

## Core runtime roles

### Orchestrator

The top-level coordinator.

Responsibilities:

- accept tasks and events
- choose the next role to activate
- manage task state transitions
- keep the runtime loop moving

### Planner

Responsible for deciding what should happen next.

Responsibilities:

- break goals into tasks
- choose skills
- produce next actions
- request browser, shell, or connector work

### Observer

Responsible for turning machine state into structured observations.

Responsibilities:

- read filesystem state
- read command output
- inspect browser or desktop state
- summarize the environment for the planner

### Executor

Responsible for executing concrete actions through the execution plane.

Responsibilities:

- run shell actions
- perform file actions
- invoke browser actions
- collect artifacts and results

### Memory steward

Responsible for memory formation and recall.

Responsibilities:

- retrieve relevant memory views
- write memory cards and summaries
- compress session history
- maintain evidence references

### Reviewer

Responsible for verification and risk control.

Responsibilities:

- review sensitive actions
- validate task outcomes
- enforce approval gates
- check for regressions or unsafe behavior

## Runtime loop

The default operating loop is:

1. ingest task or external event
2. load current user state and relevant memory views
3. planner produces or updates the active plan
4. executor performs the next approved action
5. observer records what changed
6. reviewer validates outcomes if needed
7. memory steward writes summaries and memory updates
8. orchestrator decides continue, wait, block, escalate, or finish

## Task states

Tasks should move through explicit states:

- `inbox`
- `planned`
- `active`
- `blocked`
- `awaiting_approval`
- `done`
- `failed`
- `archived`

## Session model

The runtime should maintain:

- one active session per AI user
- resumable session state on disk
- periodic session summaries
- action and observation trails

Session restarts must not require replaying full prompt history.

## Filesystem layout

The runtime should use a stable filesystem-first layout:

```text
runtime/
├── identities/
├── profiles/
├── state/
├── tasks/
│   ├── inbox/
│   ├── active/
│   ├── blocked/
│   ├── done/
│   └── plans/
├── observations/
│   ├── filesystem/
│   ├── terminal/
│   ├── browser/
│   └── os/
├── actions/
│   ├── pending/
│   ├── running/
│   ├── completed/
│   └── failed/
├── artifacts/
│   ├── reports/
│   ├── downloads/
│   ├── screenshots/
│   └── patches/
├── memory/
│   ├── journal/
│   ├── cards/
│   ├── edges/
│   ├── views/
│   └── checkpoints/
└── sessions/
    ├── current/
    ├── history/
    └── summaries/
```

## Runtime files

Recommended first-class files:

- `runtime/identities/default-user.toml`
- `runtime/profiles/user.toml`
- `runtime/profiles/root.toml`
- `runtime/state/runtime.json`
- `runtime/tasks/active/<task-id>.json`
- `runtime/actions/completed/<action-id>.json`
- `runtime/observations/terminal/<obs-id>.json`
- `runtime/sessions/current/session.json`

## Execution profiles

The runtime should support at least:

- `user`
- `root`

Rules:

- start in `user`
- elevate to `root` only through explicit authorization
- log every privileged action
- allow return to `user` after privileged steps

## Interaction sources

The orchestrator should accept work from:

- local console commands
- runtime API calls
- connector events
- scheduled tasks
- recovery on restart

All of these should become task files, not ad hoc control paths.

## Memory integration

The runtime should not pass full history to models.

Instead it should load:

- active task file
- recent session summary
- relevant memory view
- latest observation
- latest action result

## MVP boundary

Phase 1 runtime should support:

- one default AI user
- local console task intake
- user-mode execution
- shell and file actions
- action and observation recording
- session summaries
- memory journal writes

Defer for later:

- multiple persistent AI users
- root-capable execution hardening
- GUI-heavy runtime behavior
- distributed runtime workers

## Platform rule

The AI user runtime is the product core.

It must be resumable, auditable, and filesystem-native before it is optimized for sophistication.
