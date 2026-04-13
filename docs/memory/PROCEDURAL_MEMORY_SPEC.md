# Procedural Memory Spec

Procedural memory is the durable layer that captures how the runtime should carry out a recurring class of work.

It is distinct from:

- working memory: current session/task state
- episodic memory: what happened in a specific run
- semantic memory: stable facts and summaries

Procedural memory should answer:

- what sequence works for this task class
- what guardrails apply
- what steps tend to succeed
- what recovery path should be tried first

## Scope

Phase 1 keeps procedural memory intentionally small:

- first-class durable card type: `procedure`
- stored in the same memory store as other durable cards
- promoted through explicit consolidation
- validated through harness assertions and scenarios

Phase 1 does not yet include automatic extraction from repeated real runs.
It does include a first extraction path from repeated successful task metadata.

## Card Shape

Recommended content fields for `procedure` cards:

- `name`
- `task_class`
- `summary`
- `steps`
- `guardrails`
- `success_signal`

Example:

```json
{
  "name": "expense_audit_v1",
  "task_class": "expense_audit",
  "summary": "Audit reimbursements by extracting fields, validating policy, and flagging missing evidence.",
  "steps": "extract_fields\nvalidate_policy\nflag_missing_evidence",
  "guardrails": "never invent invoice ids\nrequest clarification when tax data is missing",
  "success_signal": "policy matched and exceptions enumerated"
}
```

Phase 1 stores `steps` as a newline-delimited string to keep harness seeding simple.

## Lifecycle

Procedural memory uses the same fact lifecycle as other durable cards:

- `candidate`
- `active`
- `stale`
- `superseded`
- `archived`

Phase 1 behavior:

- new procedures may be seeded or extracted as `candidate`
- consolidation promotes them to `active`
- new procedures can supersede older procedures
- cold leftover candidates may be archived

## Promotion Rules

Phase 1 promotion is explicit and deterministic:

- `consolidate_memory` can target `card_type=procedure`
- the promoted procedure becomes `active`
- any `supersedes` target is marked `superseded`
- optional `archive_remaining=true` archives leftover candidate procedures

Future promotion should require repeated supporting episodes and success evidence.

## Phase 1 Extraction Path

The first real extraction path is deterministic and rule-based.

Inputs come from repeated successful tasks that expose:

- `task_class`
- `procedure_steps`
- optional `procedure_guardrails`
- optional `procedure_summary`
- optional `procedure_success_signal`

Extraction rules:

- only `done` tasks are eligible
- tasks are grouped by:
  - `task_class`
  - `selected_skill`
  - `procedure_steps`
  - `procedure_guardrails`
- only groups with at least `min_runs` become procedure candidates

This keeps Phase 1 harnessable and avoids pretending that procedural learning is already model-derived.

## Recall and Orchestration

Procedural memory is not always injected into the model.

It should only be activated when the current task class benefits from execution guidance:

- planning
- approval-heavy execution
- repeated operator workflows
- repair/recovery flows

Phase 1 only requires procedural cards to be recall-visible once active.

## Harness Validation

Procedural memory must be regression-tested as a first-class subsystem.

Minimum assertions:

- `procedure_count`
- `procedure_contains`
- `procedure_step_contains`
- `durable_card_status`
- `durable_card_supersedes`

Minimum scenario:

- `procedural_promotion`
  - seed a candidate procedure
  - consolidate it
  - assert it becomes active
  - assert steps can be recalled from durable procedure memory
