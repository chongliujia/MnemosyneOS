# Memory Orchestration Spec

Memory orchestration decides what memory enters the model context for the current step.

The goal is not to maximize retrieval volume.
The goal is to construct the smallest context packet that still supports correct action.

## Core Rule

Retrieval should follow:

`cue -> activation -> reconstruction -> action`

This means:

- use the current task and state as cues
- activate only the most relevant memory objects
- reconstruct a compact context packet
- drive routing, planning, or response generation

## Inputs To The Orchestrator

The orchestrator should consider:

- current user turn
- session working memory
- focus task state
- recent artifacts
- recent approvals
- current skill or task type
- token budget
- privacy / scope constraints

## Retrieval Order

Recommended activation order:

1. `working memory`
2. `same-thread episodic memory`
3. `same-scope semantic memory`
4. `relevant procedural memory`
5. `archive fallback`

Archive should only be consulted when the first four layers do not provide enough context.

## Scope Boundaries

Memory should be queried by explicit scope:

- `session`
- `user`
- `project / environment`
- `global runtime`

Do not mix these scopes implicitly.

## Task Classes

### Light tasks

Examples:

- direct reply
- clarification
- short rewrite

Use:

- working memory only
- optional small semantic slice

### Medium tasks

Examples:

- personalized recommendation
- structured summary
- continuation with constraints

Use:

- working memory
- semantic memory
- small episodic slice when necessary

### Heavy tasks

Examples:

- planning
- search and synthesize
- recovery after failure
- approval-sensitive execution

Use:

- working memory
- semantic memory
- episodic memory
- procedural memory

## Context Packet Shape

A context packet should be assembled as small labeled sections.

Recommended layout:

- `Current task`
- `Active constraints`
- `Relevant stable facts`
- `Relevant prior episodes`
- `Execution guidance`

The packet should favor structured short items over large raw text blocks.

## Token Budgeting

The orchestrator should be budget-aware.

Suggested behavior:

- reserve budget for the actual model task first
- bound each memory layer separately
- cap item counts per layer
- discard low-utility recalls aggressively

Example per-call caps:

- working summary: 250-350 tokens
- semantic facts: 200-300 tokens
- episodic recalls: 200-350 tokens
- procedural guidance: 100-200 tokens

These are orchestration ceilings, not storage limits.

## Retrieval Strategy

Use hybrid retrieval, not embedding-only retrieval.

Signals should include:

- semantic similarity
- lexical match
- metadata filter
- recency
- entity overlap
- outcome relevance
- scope match

The final packet should be selected by utility-to-task, not by similarity score alone.

## Output Requirements

The orchestrator should return memory objects, not just text snippets.

Useful object fields include:

- id
- type
- scope
- summary
- evidence refs
- confidence
- freshness
- lifecycle status

## Post-Use Feedback

After a memory object is used, the runtime should record whether it:

- helped the task
- was ignored
- introduced conflict
- should be promoted, downgraded, or archived

This feedback becomes input to consolidation.

## Failure Modes To Prevent

- top-k over-recall
- unrelated user-profile pollution
- wrong-scope memory injection
- stale facts crowding current constraints
- procedural guidance appearing in simple chat turns
- archive dominating active memory layers
