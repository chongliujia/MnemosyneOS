# Harness

MnemosyneOS now includes a local-first harness layer for deterministic scenario execution.

The goal is not UI demo coverage. The goal is engineering coverage:

- replayable runs
- durable run records
- structured assertions
- regression comparison
- execution trace capture
- procedure promotion/supersession rollup

## Phase 1 Scope

Phase 1 focuses on a compact industrial baseline:

1. `web_search_summary`
2. `root_approval_flow`
3. `chat_followup_continuity`
4. `email_inbox_summary`
5. `file_read_roundtrip`
6. `working_memory_followup`
7. `memory_write_recall_roundtrip`
8. `approval_memory_boundary`
9. `session_recovery_continuity`
10. `memory_contamination_resistance`
11. `candidate_memory_promotion`
12. `memory_contamination_recovery`
13. `procedural_promotion`
14. `procedural_extraction_repeated_runs`
15. `procedural_extraction_memory_consolidate`
16. `procedural_supersession_lifecycle`
17. `memory_usefulness_feedback`
18. `memory_feedback_noop_direct_reply`
19. `retryable_timeout_shell`
20. `process_exit_not_retried`
21. `idempotent_shell_replay`
22. `failed_shell_manual_rerun`
23. `scheduled_memory_consolidation_after_web_search`

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
- `lane`
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

### `restart_runtime`

Rebuild the harness runtime services from the existing runtime root.

Use this to validate:

- session continuity after restart
- working-memory recovery
- replayable runtime reconstruction

### `schedule_memory_consolidation`

Probe the memory scheduler and optionally enqueue a `memory-consolidate` task.

Supported fields:

- `metadata.reason` (optional)
- `metadata.scope` (optional)
- `metadata.cooldown_ms` (optional)
- `metadata.min_candidates` (optional)
- `metadata.allowed_card_types` (optional, comma-separated)
- `metadata.extract_procedures` (optional)
- `metadata.min_runs` (optional)

### `consolidate_memory`

Promote candidate memory into active durable memory without routing through chat.

Supported fields:

- `metadata.card_type` (optional)
- `metadata.scope` (optional)
- `metadata.limit` (optional)
- `metadata.archive_remaining` (optional)
- `metadata.extract_procedures` (optional)
- `metadata.task_class` (optional)
- `metadata.selected_skill` (optional)
- `metadata.min_runs` (optional)

Use this to validate:

- candidate-to-active promotion
- recall visibility after consolidation
- bounded memory promotion by type or scope
- procedure extraction from repeated successful task evidence discovered through `*_observation` / `*_artifact` metadata paths, with task metadata only as fallback

### `seed_memory_card`

Insert a deterministic durable-memory card directly into the harness runtime.

Supported fields:

- `metadata.card_id`
- `metadata.card_type`
- `metadata.scope` (optional)
- `metadata.status` (optional)
- `metadata.supersedes` (optional)
- `metadata.source` (optional)
- `metadata.confidence` (optional)
- `metadata.activation_score` (optional)
- `metadata.activation_decay_policy` (optional)
- `metadata.content.<field>` for content payload fields

Use this to validate:

- fact lifecycle transitions
- supersession logic
- archive policies
- recall boundaries without routing through a live task

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

### `file_absent`

Assert a runtime file does not exist.

Fields:

- `path`

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

### `working_topic_contains`

Assert the session working-memory topic contains a substring.

Fields:

- `session_id`
- `contains`

### `working_focus_task_equals`

Assert the session working-memory focus task matches an exact task id.

Fields:

- `session_id`
- `equals`

### `working_pending_question_contains`

Assert the session working-memory pending question contains a substring.

Fields:

- `session_id`
- `contains`

### `working_pending_action_contains`

Assert the session working-memory pending action contains a substring.

Fields:

- `session_id`
- `contains`

### `durable_card_count`

Assert the durable memory store contains cards of a given type.

Fields:

- `field` (`card_type`)
- `expected` or `min`

### `durable_card_contains`

Assert a durable card of a given type contains a substring in its content payload.

Fields:

- `field` (`card_type`)
- `contains`

### `durable_card_status`

Assert a matching durable card has the expected lifecycle status.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `equals` (expected status)

Expected lifecycle values currently include:

- `candidate`
- `active`
- `stale`
- `superseded`
- `archived`

### `durable_card_confidence_range`

Assert a matching durable card has provenance confidence inside a bounded range.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `min_confidence`
- `max_confidence`

### `durable_card_scope`

Assert a matching durable card belongs to the expected memory scope.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `equals` (expected scope, for example `user` or `project`)

### `durable_card_supersedes`

Assert a matching durable card explicitly supersedes an older fact.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `equals` (expected superseded card id)

### `durable_card_version_equals`

Assert a matching durable card remains at an exact version.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `expected`

### `durable_card_version_at_least`

Assert a matching durable card has been revised at least N times.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `min`

### `durable_card_activation_score_range`

Assert a matching durable card has activation score inside a bounded range.

Fields:

- `field` (`card_type`)
- `contains` (optional content filter)
- `min_confidence`
- `max_confidence`

### `procedure_count`

Assert the active durable procedure store contains at least N procedures.

Fields:

- `expected` or `min`

### `procedure_contains`

Assert an active procedure contains a substring in its content payload.

Fields:

- `contains`

### `procedure_step_contains`

Assert an active procedure includes a matching step, guardrail, or summary fragment.

Fields:

- `contains`

### `action_attempt_count`

Assert a step action executed with the expected number of attempts.

Fields:

- `step`
- `expected` or `min`

### `action_failure_category`

Assert a step action ended with the expected executor failure category.

Fields:

- `step`
- `equals`

### `action_replayed`

Assert whether a step action reused a previously completed action via idempotent replay.

Fields:

- `step`
- `equals` (`true` / `false`)

### `retry_succeeded`

Assert whether a step only succeeded after one or more retries.

Fields:

- `step`
- `equals` (`true` / `false`)

### `scheduler_triggered`

Assert whether a scheduler step actually enqueued a consolidation task.

Fields:

- `step`
- `equals` (`true` / `false`)

### `scheduler_skip_reason`

Assert why a scheduler step skipped instead of enqueueing maintenance.

Fields:

- `step`
- `equals`

### `recall_contains`

Assert recall returns content containing a substring.

Fields:

- `query`
- `source` (optional)
- `contains`

### `recall_not_contains`

Assert recall does not return content containing a substring.

Fields:

- `query`
- `source` (optional)
- `contains`

### `edge_exists`

Assert at least one memory edge exists for the given type.

Fields:

- `field` (`edge_type`)
- `contains` (optional substring filter matched against `from_card_id` or `to_card_id`)

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

## Tags

Scenarios can include tags to support selective execution and reporting.

Current tags are used for domains such as:

- `chat`
- `memory`
- `procedural`
- `working-memory`
- `connector`
- `execution`
- `email`
- `web`
- `recall`
- `approval`
- `shell`
- `observability`

## Lanes

Scenarios can also include a `lane` for layered execution:

- `smoke`
- `regression`
- `soak`

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

Run a lane:

```bash
go run ./cmd/mnemosyne-harness -lane smoke
go run ./cmd/mnemosyne-harness -lane regression -tags memory
```

Choose a custom output root:

```bash
go run ./cmd/mnemosyne-harness -out ./runs
```

See also:

- [HARNESS_ARCHITECTURE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/HARNESS_ARCHITECTURE.md)
- [docs/memory/MEMORY_ARCHITECTURE.md](/Users/jiachongliu/My-Github-Project/MnemosyneOS/docs/memory/MEMORY_ARCHITECTURE.md)

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
go run ./cmd/mnemosyne-harness -rollup ./runs -lane regression
go run ./cmd/mnemosyne-harness -rollup ./runs -rollup-json ./runs/rollup.json
```

Rollups now surface scheduler policy behavior as first-class counters:

- `scheduler_triggers`
- `scheduler_cooldown_skips`
- `scheduler_threshold_skips`
- `scheduler_type_skips`
- `scheduler_existing_task_skips`
- `scheduler_runtime_busy_skips`

Save a golden baseline:

```bash
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -tags execution
go run ./cmd/mnemosyne-harness -save-baseline ./runs -baseline-dir ./baselines/harness -lane smoke
```

Check current runs against the baseline:

```bash
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -tags email
go run ./cmd/mnemosyne-harness -check-baseline ./runs -baseline-dir ./baselines/harness -lane regression
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
