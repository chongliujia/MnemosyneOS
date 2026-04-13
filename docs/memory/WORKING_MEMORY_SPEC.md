# Working Memory Spec

Working memory is the live operational state that keeps a session or active task coherent.

It should stay small, explicit, fast to update, and cheap to read.

## Purpose

Working memory exists to support:

- follow-up continuity
- task continuation
- approval continuity
- current-topic retention
- low-latency direct reply and routing

It is not a long-term knowledge store.

## Scope

Working memory is maintained at two scopes:

1. `session scope`
   - current conversational topic
   - recent artifacts and recalls
   - pending question / pending action
2. `task scope`
   - focus task id
   - current execution hint
   - unresolved approval ids

## Canonical State Shape

Recommended fields:

- `topic`
- `focus_task_id`
- `pending_question`
- `pending_action`
- `active_artifact_ids`
- `recent_recall_ids`
- `open_approval_ids`
- `current_skill_hint`
- `last_user_act`
- `last_assistant_act`
- `updated_at`

These fields should stay bounded.

## Update Rules

### Always update from runtime facts

The following should be updated deterministically from runtime state rather than model opinion:

- `focus_task_id`
- `open_approval_ids`
- `active_artifact_ids`
- `recent_recall_ids`
- `updated_at`

### Usually update from dialogue interpretation

The following may be updated from routing/dialogue results:

- `topic`
- `pending_question`
- `pending_action`
- `current_skill_hint`
- `last_user_act`
- `last_assistant_act`

### Never treat working-memory state as durable fact

Working memory values are operational context, not long-term truth.

They should not be promoted automatically into durable semantic memory.

## Read Priority

When handling a new turn, read in this order:

1. current session working memory
2. current task working memory facts
3. recent artifacts and approvals
4. only then consult durable memory

Short follow-ups such as `continue`, `expand`, or `yes` should normally be satisfiable from working memory plus recent artifacts.

## Persistence

Working memory should be snapshotted durably so the system can recover after restart.

The snapshot should remain:

- compact
- session-scoped
- overwrite-friendly
- auditable

The snapshot is not the event log. It is the current state projection.

## Expiry And Cleanup

Working memory should decay faster than durable memory.

Suggested rules:

- resolved `pending_question` should be cleared quickly
- completed `pending_action` should be cleared immediately
- old `recent_recall_ids` should roll off
- stale `current_skill_hint` should expire
- session state should compact at session end or consolidation time

## Failure Modes To Prevent

- topic drift across turns
- wrong task continuation
- approval context lost after restart
- follow-up treated as a new unrelated task
- stale temporary constraints persisting too long

## Harness Expectations

Working memory must be directly testable.

Important assertions include:

- `session_state_contains`
- continuity after short follow-up
- continuity after restart
- approval context retained but not over-promoted
- focus task remains stable across relevant turns
