# MnemosyneOS Interfaces

## Goal

Define the three external interface layers of the platform:

- local console
- runtime API
- connector interface

These interfaces should all feed the same runtime and task model.

## 1. Local console

The local console is the primary human control surface.

Responsibilities:

- submit tasks
- inspect runtime state
- approve or deny actions
- inspect memory and recent observations
- switch execution profile when permitted

Recommended MVP commands:

- `status`
- `ask`
- `tasks`
- `memory`
- `logs`
- `approve`
- `deny`
- `mode`
- `connectors`

The console should be CLI-first in Phase 1.

## 2. Runtime API

The runtime API is the programmatic interface to the AI user runtime.

Responsibilities:

- submit tasks
- fetch task state
- fetch runtime state
- fetch action logs
- inspect memory views
- control connector registrations

Suggested initial endpoints:

- `GET /health`
- `GET /runtime/state`
- `POST /tasks`
- `GET /tasks/{id}`
- `POST /tasks/{id}/approve`
- `POST /tasks/{id}/deny`
- `POST /tasks/{id}/run`
- `POST /actions/shell`
- `POST /actions/file-read`
- `POST /actions/file-write`
- `GET /actions/{id}`
- `GET /memory/views/{name}`

The runtime API should be distinct from the memory API over time, even if the MVP keeps them together.

## 3. Connector interface

Connectors are how the runtime interacts with remote systems.

Examples:

- mail
- calendar
- GitHub
- Slack
- webhook events

Connector responsibilities:

- ingest external events
- normalize them into task or observation files
- execute remote actions through runtime policy
- produce artifacts and audit records

## Unified task intake rule

Whether work comes from:

- a human in the console
- an HTTP client
- an email event
- a scheduled timer

it should enter the runtime as a task file.

No separate hidden execution path should exist for connectors.

## Interface contracts

All interfaces should converge on these internal objects:

- `task`
- `observation`
- `action`
- `artifact`
- `memory candidate`

## Approval model

Approvals should be possible from both:

- local console
- runtime API

Connectors should not bypass approval requirements.

## Identity model

All interfaces should act on behalf of the same AI user identity unless explicitly configured otherwise.

## MVP boundary

Phase 1 should include:

- local CLI console
- basic runtime HTTP API
- one connector pattern for future extension

The first remote connector can be deferred until the runtime loop is stable.

## Platform rule

Interfaces are entry points, not separate products.

They should remain thin layers over the same runtime, policy, memory, and execution model.
