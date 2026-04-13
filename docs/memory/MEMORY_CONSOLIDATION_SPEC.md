# Memory Consolidation Spec

Consolidation is the offline or low-priority process that transforms raw experience into reliable durable memory.

It plays the same role that sleep-like replay and restructuring plays in biological memory systems:

- replay
- selection
- strengthening
- abstraction
- correction
- demotion

## Why Consolidation Exists

Without consolidation, the runtime falls into a brute-force pattern:

- write everything early
- search everything later
- summarize repeatedly on the hot path

That causes:

- noisy durable memory
- expensive recall
- premature fact promotion
- low-quality long-term abstractions

Consolidation exists to keep online paths light and long-term memory clean.

## Inputs

Consolidation should process:

- recent episodic events
- candidate memories
- post-use feedback from orchestration
- task completion outcomes
- approval decisions
- failures and recoveries
- repeated successful procedures

## Triggers

Recommended triggers:

- task completion
- session end
- idle runtime window
- periodic maintenance cycle
- explicit maintenance job

Consolidation should not block normal conversational turns.

## Pipeline

### 1. Replay

Group related recent events:

- by session
- by task
- by entity
- by repeated workflow

### 2. Dedupe And Merge

Collapse duplicate candidates or near-identical event summaries.

### 3. Promotion Decision

Decide whether a candidate should become:

- retained episodic memory
- semantic memory
- procedural memory
- archived-only material
- discarded material

### 4. Abstraction

Generate durable summaries or procedural templates only when there is enough support.

### 5. Lifecycle Update

Adjust existing durable facts:

- strengthen
- downgrade confidence
- mark stale
- mark superseded
- archive

### 6. Archive And Cleanup

Move cold or low-value items out of the high-activation set.

## Promotion Rules

### Promote To Semantic Memory When

- the information is reusable
- evidence is sufficient
- the conclusion is stable enough to guide future tasks
- repeated episodes support the same abstraction

### Promote To Procedural Memory When

- a repeatable task pattern succeeded
- the steps are reusable
- guardrails can be expressed
- future tasks are likely to benefit from the pattern

### Keep As Episodic Memory When

- the value is historical rather than abstract
- the event may be useful for replay or audit
- the lesson is not yet stable enough to generalize

### Do Not Promote When

- the content is speculative
- the action was unapproved
- the execution failed without useful durable lesson
- the information is too transient or privacy-sensitive

## Fact Lifecycle

Durable memory objects should support lifecycle states:

- `candidate`
- `active`
- `stale`
- `superseded`
- `archived`

Recommended metadata:

- `confidence`
- `freshness`
- `evidence_refs`
- `first_seen`
- `last_confirmed`
- `supersedes`
- `contradicted_by`

Consolidation is the primary place where lifecycle transitions are applied.

## Procedural Memory Extraction

Procedural memory should capture:

- task class
- successful sequence
- decision points
- failure risks
- recovery steps
- usage constraints
- success evidence

Procedural memory should be versioned and benchmarkable.

## Archive Policy

Archive is for:

- cold episodic material
- superseded facts retained for audit
- low-utility historical traces
- old candidate material that should not clutter active recall

Archive is not deletion.

Deletion remains a separate privacy/compliance mechanism.

## Harness Expectations

Consolidation must be harnessable.

Useful scenario classes:

- memory write / recall roundtrip
- contamination resistance
- approval memory boundary
- recovery continuity
- procedural promotion after repeated success
- stale fact supersession

## Near-Term Implementation Guidance

First implementation should prefer discipline over sophistication:

1. introduce candidate memory explicitly
2. separate promotion from hot-path execution
3. record lifecycle metadata on durable writes
4. keep consolidation asynchronous
5. start with deterministic promotion rules before adding model-based refinements
