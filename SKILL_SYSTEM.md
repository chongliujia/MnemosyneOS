# MnemosyneOS Skill System

## Goal

Define skills as executable behavior modules that the AI user runtime can select, run, review, and learn from.

## Skill definition

A skill is not just prompt text.

A skill is a runtime behavior package that can include:

- instructions
- input/output schema
- policy requirements
- scripts
- references
- memory hooks

## Skill categories

The platform should support these core categories first:

- `observe`
- `edit`
- `execute`
- `browse`
- `memory`
- `task`

## Skill lifecycle

The normal lifecycle is:

1. planner selects a skill
2. runtime validates policy and required profile
3. executor runs the skill
4. reviewer checks result if needed
5. memory steward stores useful outputs

## Skill directory layout

```text
skills/
├── <skill-name>/
│   ├── SKILL.md
│   ├── policy.toml
│   ├── io.schema.json
│   ├── scripts/
│   ├── references/
│   └── assets/
```

## Required files

### `SKILL.md`

Contains:

- skill name
- purpose
- trigger guidance
- step-level instructions
- failure and recovery notes

### `policy.toml`

Defines:

- required execution profile
- allowed commands
- allowed path scopes
- network requirements
- risk level
- approval requirements

### `io.schema.json`

Defines:

- input fields
- output fields
- artifact paths
- expected observation shape
- memory write candidates

## Policy model

Each skill should declare:

- `profile = user|root`
- `approval = none|review|required`
- `risk = low|medium|high`
- `filesystem_scope`
- `network_scope`
- `destructive = true|false`

This keeps skill execution aligned with runtime policy.

## Skill inputs

Common inputs should include:

- task id
- goal summary
- relevant paths
- relevant memory handles
- execution profile
- prior observations

## Skill outputs

Common outputs should include:

- action result
- changed files
- artifacts produced
- follow-up observations
- memory candidates
- status and failure reason

## Skill and memory integration

Each skill should declare:

- what memory views it may read
- what action results should become memory candidates
- what evidence should be attached to resulting memory cards

Skills should not write long-term memory directly without going through the memory steward.

## Skill and Search Integration

Browser-oriented skills should be normal skills, not special cases.

Examples:

- `web-search`
- `api-read`
- `api-compare-sources`

They still follow the same policy, I/O, and audit model.

## Skill and model integration

Skills may require:

- text reasoning
- vision perception
- no model at all if the action is deterministic

The skill definition should not hard-code one vendor API.

It should request capabilities from the model gateway instead.

## MVP recommendation

Start with a small skill set:

- `shell-run`
- `file-read`
- `file-edit`
- `repo-summarize`
- `web-search`
- `memory-consolidate`
- `task-plan`

## Validation rule

Every skill should be testable in isolation with:

- sample input
- expected output shape
- known policy profile
- audit trail

## Platform rule

Skills are the reusable action vocabulary of the AI user.

They must be executable, policy-aware, and memory-aware from day one.
