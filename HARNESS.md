# Harness

MnemosyneOS now includes a local-first harness layer for deterministic scenario execution.

The goal is not UI demo coverage. The goal is engineering coverage:

- replayable runs
- durable run records
- structured assertions
- regression comparison
- execution trace capture

## Phase 1 Scope

Phase 1 focuses on a compact industrial baseline:

1. `web_search_summary`
2. `root_approval_flow`
3. `chat_followup_continuity`
4. `email_inbox_summary`
5. `file_read_roundtrip`

Each scenario runs inside an isolated runtime root and produces a run report.

## Scenario Layout

Each scenario lives under `scenarios/<name>/`.

Required files:

- `scenario.json`

Optional fixtures:

- `fixtures/search_response.json`
- `fixtures/email_response.json`

## Scenario Format

Scenarios are JSON for now to avoid adding a new parser dependency.

Top-level fields:

- `name`
- `description`
- `fixtures`
- `steps`
- `assertions`

## Supported Step Types

### `submit_task`

Create a runtime task through the orchestrator.

Supported fields:

- `title`
- `goal`
- `requested_by`
- `source`
- `execution_profile`
- `requires_approval`
- `selected_skill`
- `metadata`

### `run_task`

Run a previously created task through `skills.Runner`.

Supported fields:

- `task_ref`

`task_ref` may be:

- a previous step id
- `last`
- a literal task id

### `approve_pending`

Approve a pending root approval request and move the related task back to `planned`.

Supported fields:

- `approval_ref`
- `approved_by`

### `send_chat`

Send a message through the chat runtime.

Supported fields:

- `session_id`
- `message`
- `requested_by`
- `source`
- `execution_profile`

## Supported Assertions

### `task_state`

Assert the referenced task reached a target state.

Fields:

- `step`
- `equals`

### `selected_skill`

Assert the referenced task used the expected skill.

Fields:

- `step`
- `equals`

### `approval_count`

Assert approvals in a given status.

Fields:

- `status`
- `expected` or `min`

### `artifact_count`

Assert a step produced at least N artifacts.

Fields:

- `step`
- `expected` or `min`

### `observation_count`

Assert a step produced at least N observations.

Fields:

- `step`
- `expected` or `min`

### `artifact_contains`

Assert any artifact from a step contains a substring.

Fields:

- `step`
- `contains`

### `assistant_contains`

Assert the assistant reply for a `send_chat` step contains a substring.

Fields:

- `step`
- `contains`

### `file_contains`

Assert a runtime file contains a substring.

Fields:

- `path`
- `contains`

Relative paths are resolved from the run's `runtime/` directory.

### `session_state_contains`

Assert a session state field contains a substring.

Fields:

- `session_id`
- `field`
- `contains`

Supported fields:

- `topic`
- `focus_task_id`
- `pending_action`
- `pending_question`
- `last_user_act`
- `last_assistant_act`

## Run Output

Each harness execution writes into `runs/<timestamp>-<scenario>/`.

Current output includes:

- `scenario.json`
- `report.json`
- `runtime/`

The embedded `runtime/` directory contains the same durable artifacts used by the main system:

- tasks
- approvals
- actions
- observations
- artifacts
- sessions

## CLI

Run all scenarios:

```bash
go run ./cmd/mnemosyne-harness
```

Run a single scenario:

```bash
go run ./cmd/mnemosyne-harness -scenario ./scenarios/web_search_summary
```

Run a tagged subset:

```bash
go run ./cmd/mnemosyne-harness -tags chat,memory
go run ./cmd/mnemosyne-harness -tags execution
```

Choose a custom output root:

```bash
go run ./cmd/mnemosyne-harness -out ./runs
```

Diff two runs:

```bash
go run ./cmd/mnemosyne-harness -report-a ./runs/<run-a> -report-b ./runs/<run-b>
```

The diff now compares:

- scenario/report metadata
- step-level task state and selected skill
- assertion outcomes
- artifact counts
- artifact content for matching step artifact slots
- semantic JSON content, ignoring formatting-only changes

Build a benchmark rollup:

```bash
go run ./cmd/mnemosyne-harness -rollup ./runs
go run ./cmd/mnemosyne-harness -rollup ./runs -tags chat,memory
go run ./cmd/mnemosyne-harness -rollup ./runs -rollup-json ./runs/rollup.json
```

Save a golden baseline:

```bash
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -tags execution
```

Check current runs against the baseline:

```bash
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -tags email
```

## Design Notes

- Harness runs use fixture-backed connectors instead of live network calls.
- Harness runs use a deterministic stub model for routing, chat continuation, and summaries.
- The objective is stable regression coverage, not model realism.

## Next Steps

Phase 2 should add:

- richer evaluator rules
- benchmark rollups
- approval and recovery coverage for more execution types
