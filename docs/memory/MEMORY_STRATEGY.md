# Memory Strategy

MnemosyneOS should not treat memory as a larger storage bucket or a stronger retrieval index.

The goal is not to remember more.
The goal is to remember more selectively, activate the right context, preserve durable knowledge, and accumulate reliable operator experience.

The design target is:

- cue-driven recall
- working-memory continuity
- deliberate promotion into durable memory
- offline consolidation
- procedural memory growth
- fact lifecycle management
- harnessable memory correctness

This document defines the strategic memory model for an industrial AgentOS runtime.

## Core Thesis

Human memory is not used as a raw archive lookup.

A partial cue activates relevant prior patterns, those patterns are reconstructed into usable context, and that reconstructed context is used to interpret the present and choose the next action.

MnemosyneOS should follow the same principle:

- do not retrieve everything
- do not write everything durably
- do not collapse all memory into one store
- do not treat retrieval as the final goal

Instead:

- use cues to activate the most relevant memory objects
- reconstruct action-oriented context
- decide what deserves durable promotion
- reorganize memory during consolidation

## Why The Current “Store And Search” Approach Is Not Enough

A naive memory system usually behaves like this:

1. capture a lot of content
2. write it immediately
3. search it later
4. summarize again in the prompt path

This works as a bootstrap, but it creates the wrong incentives:

- online paths become heavy
- long-term memory becomes noisy
- recall becomes increasingly expensive
- incorrect or transient conclusions are promoted too early
- multi-turn continuity depends too much on durable recall
- the agent remembers facts, but not stable procedures

This is effectively a brute-force memory strategy.

MnemosyneOS should replace it with a staged memory system.

## Strategic Memory Pipeline

The recommended pipeline is:

`sensory buffer -> working memory -> candidate memory -> consolidation -> durable memory -> archive`

Procedural memory and fact lifecycle sit across this pipeline rather than at only one stage.

## Memory Layers

### 1. Sensory Buffer

This layer holds the current raw material entering the system.

Examples:

- the latest user turn
- connector responses
- shell output
- email payloads
- current artifact drafts
- streaming generation fragments

Properties:

- short-lived
- not durable
- not directly recallable
- exists to support current processing only

This layer should not be confused with memory proper.

It is staging input.

### 2. Working Memory

Working memory is the operational memory of the current session or active task.

It should remain small, explicit, and fast.

Recommended working-memory fields:

- `topic`
- `focus_task_id`
- `pending_question`
- `pending_action`
- `active_artifact_ids`
- `recent_recall_ids`
- `last_user_act`
- `last_assistant_act`
- `open_approval_ids`
- `current_skill_hint`

Properties:

- session-scoped
- task-scoped where necessary
- mutable
- low-latency
- restartable via snapshots

Working memory should be the first source consulted for:

- short follow-ups
- clarification turns
- approval continuation
- task continuation after a recent exchange

Most conversation failures are working-memory failures, not long-term-memory failures.

### 3. Candidate Memory

Candidate memory is the buffer between raw experience and durable knowledge.

This is where new information waits before being promoted.

Examples:

- a web result that may matter later
- a summary that still depends on weak evidence
- an email conclusion not yet stabilized
- a pattern inferred from one successful run

Properties:

- durable enough to survive short horizons
- not yet treated as trusted long-term knowledge
- explicitly promotable or discardable

Candidate memory exists to prevent premature durable writes.

### 4. Episodic Memory

Episodic memory records what happened.

Examples:

- task transitions
- approval requests and decisions
- execution actions
- connector fetches
- error traces
- recovery attempts

Properties:

- strongly time-ordered
- evidence-rich
- replayable
- audit-oriented

Episodic memory is the operational history of the system.

### 5. Semantic Memory

Semantic memory stores stable knowledge abstractions.

Examples:

- durable summaries
- normalized facts
- stable environment descriptions
- durable user/operator preferences
- connector-derived knowledge that should survive sessions

Properties:

- cross-session reusable
- evidence-backed
- confidence-bearing
- freshness-aware
- queryable through recall

Semantic memory answers:

- what the system currently believes
- what it knows about the world
- what stable abstractions should guide future work

### 6. Procedural Memory

Procedural memory is the memory of successful method, not just successful information.

Examples:

- task decomposition patterns that lead to completion
- recovery workflows for shell failures
- safe root-approval playbooks
- effective search-and-summarize chains
- skill compositions that work under specific conditions

Properties:

- action-oriented
- reusable
- benchmarkable
- gradually strengthened by repeated success

This is one of the highest-value memory types for an industrial AgentOS.

Without procedural memory, the system remembers facts but does not truly improve at operating.

### 7. Archive

Archive stores cold, old, demoted, or superseded memory objects.

Archive is not deletion.

It is a lower-activation tier that preserves:

- auditability
- replayability
- historical context
- superseded knowledge

Archived memory should rarely appear in default recall, but should remain reachable when explicitly needed.

## Why This Layering Is Better

This design is better than a single memory bucket because each layer has a different job:

- `sensory buffer` handles raw incoming state
- `working memory` maintains continuity
- `candidate memory` prevents premature promotion
- `episodic memory` preserves what happened
- `semantic memory` preserves what is believed
- `procedural memory` preserves how to succeed
- `archive` preserves history without polluting active recall

The system becomes:

- lighter online
- cleaner durably
- easier to correct
- easier to evaluate
- more useful over long-running operation

## Promotion Strategy

Not everything should enter durable memory.

Promotion should be explicit and selective.

### Likely Working-Memory-Only

- current routing hints
- pending question markers
- current focus task markers
- unresolved approval pointers
- transient speculation

### Likely Candidate Memory

- one-off search findings
- tentative summaries
- early pattern observations
- partially trusted connector information

### Likely Durable Semantic Memory

- evidence-backed durable summaries
- stable facts confirmed by multiple observations
- reusable environment knowledge
- durable operator preferences

### Likely Durable Procedural Memory

- repeated successful plans
- known-safe recovery sequences
- stable approval-handling patterns
- successful skill chains under specific conditions

Promotion should depend on:

- evidence quality
- reuse likelihood
- confidence
- recency
- repetition
- conflict with existing memory

## Consolidation And The “Sleep Cycle”

Human memory does not stabilize only in the middle of active work.

A large part of useful memory formation happens during offline consolidation.

MnemosyneOS should have an explicit consolidation engine that plays the same role.

### Consolidation Triggers

Recommended triggers:

- session end
- task completion
- idle periods
- scheduled maintenance cycles
- explicit operator-triggered compaction

### Consolidation Responsibilities

1. replay recent events
2. detect duplicate or overlapping candidates
3. generate durable summaries
4. promote candidates into semantic or procedural memory
5. adjust confidence and freshness
6. demote stale memory
7. archive cold memory
8. clean up expired working-memory state

### Why Consolidation Matters

Without consolidation:

- online execution becomes heavy
- durable memory becomes noisy
- memory quality depends on prompt timing
- the system never truly “learns,” it only stores

With consolidation:

- online execution stays lighter
- durable memory becomes more structured
- procedural memory can emerge from repeated runs
- memory behavior becomes easier to evaluate

## Recall Strategy

Recall should be cue-driven, not dump-driven.

The current situation many systems fall into is:

`query -> search -> paste results into model`

That is not memory use.

MnemosyneOS should prefer:

`cue -> activation -> reconstruction -> action`

### Cue Sources

Recall cues should include:

- current topic
- focus task
- current skill
- pending question
- approval state
- active artifacts
- recent failures
- execution profile

### Activation Order

Recommended activation order:

1. working memory
2. directly related episodic memory
3. high-confidence semantic memory
4. matching procedural memory
5. archive only when necessary

### Reconstruction Goal

Recall should not return a dump of stored text.

It should reconstruct:

- current relevant context
- strongest supporting evidence
- useful next-step hints
- contradictions or uncertainty markers

The output of recall should therefore be object-oriented:

- cards
- edges
- evidence references
- snippets
- confidence/freshness markers

## Fact Lifecycle

A durable memory system must support change.

If a fact is written once and never evolves, the system will degrade as the world changes.

Recommended lifecycle states:

- `candidate`
- `active`
- `stale`
- `superseded`
- `archived`

Recommended relations:

- `supports`
- `contradicts`
- `supersedes`
- `derived_from`

### Why Lifecycle Matters

It allows the system to:

- preserve old beliefs without treating them as current truth
- reduce confidence when contradictory evidence appears
- mark newer facts as replacing older ones
- keep recall from surfacing outdated knowledge as authoritative

## Procedural Memory Strategy

Procedural memory should be first-class, not an afterthought.

It is the mechanism by which the system gets better at operation rather than merely accumulating data.

### Procedural Memory Inputs

Candidates for procedural memory can come from:

- repeated successful scenario runs
- successful task completions under stable conditions
- successful recovery flows
- recurring approval/execution patterns
- reliable operator overrides

### Procedural Memory Objects Should Capture

- task class
- triggering context
- successful sequence of steps
- risk points
- recovery conditions
- evidence of success
- confidence
- last validated run ids

### Procedural Memory Should Be Harnessed

A procedural pattern is valuable only if the system can prove:

- it is repeatedly successful
- it is not unsafe
- it remains valid over time

This makes harness and procedural memory tightly linked.

## Memory Correctness Requirements

An industrial memory system must be evaluated beyond “did recall return something”.

It must answer:

- was the right thing written
- was it written at the right time
- should it have remained transient
- was a wrong belief corrected
- did recall activate the correct memory object
- did working memory preserve continuity

## Memory Evaluation Domains

Harness and runtime evaluation should explicitly cover:

### 1. Working Memory Continuity

Examples:

- short follow-up remains on topic
- focus task survives task completion
- session survives restart
- pending question is interpreted correctly

### 2. Durable Memory Correctness

Examples:

- summary cards created when expected
- canonical cards deduplicated correctly
- evidence references attached
- stale knowledge not treated as active

### 3. Recall Correctness

Examples:

- correct cards activated for current cue
- expected snippet returned
- wrong source not preferred
- archived knowledge not surfaced by default

### 4. Procedural Memory Quality

Examples:

- repeated successful workflows get promoted
- failed patterns do not become trusted procedures
- recovery strategies improve over time

### 5. Contamination Resistance

Examples:

- bad intermediate outputs do not become durable facts
- unapproved actions do not become completed facts
- speculative model text is not promoted without evidence

## Recommended Immediate Implementation Priorities

The first implementation wave should prioritize:

1. stronger working-memory contracts
2. candidate-memory staging before durable writes
3. explicit consolidation jobs
4. fact lifecycle metadata
5. harness scenarios for continuity, roundtrip, and contamination

Only after these are stable should the system invest heavily in:

- more complex retrieval ranking
- richer archive policies
- advanced personalization

## Decision Standard

Any memory feature should be evaluated against this question:

Does it improve at least one of:

- continuity
- correctness
- replayability
- evaluability
- recoverability
- contamination resistance

If not, it is probably decoration rather than memory engineering.

## Summary

MnemosyneOS should treat memory as a staged cognitive system, not a stronger storage engine.

The strategic direction is:

- working-memory-first
- candidate-before-durable
- consolidation-before-promotion
- fact-lifecycle-aware
- procedural-memory-first-class
- cue-driven recall
- harness-verified correctness

That is the path from brute-force memory toward an industrial Agent memory system.
