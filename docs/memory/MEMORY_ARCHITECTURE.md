# Memory Architecture

MnemosyneOS memory should not be treated as one generic storage bucket.

It should be understood as two cooperating systems:

1. `Working Memory`
2. `Long-Term Memory`

This split is essential for an industrial AgentOS runtime.

## 1. Working Memory

Working memory is session-scoped and interaction-critical.

It exists to preserve continuity while the agent is actively operating.

Examples:

- current topic
- focus task
- pending question
- pending action
- recent artifact context
- recent recall hits

In the current implementation, the closest durable form of working memory is the session state.

Relevant code:

- [internal/chat/types.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/chat/types.go)
- [internal/chat/store.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/chat/store.go)
- [internal/chat/service.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/chat/service.go)

### Why It Matters

Most multi-turn failures are working-memory failures, not long-term-memory failures.

Typical examples:

- the agent forgets the current topic
- the agent loses the focus task
- a short follow-up gets routed as a new task
- a pending question is not recognized on the next turn

If working memory is unstable, long-term memory quality does not save the interaction.

## 2. Long-Term Memory

Long-term memory is durable, queryable, and reusable across tasks and sessions.

It exists to preserve knowledge that should outlive the current interaction.

Examples:

- search result summaries
- email thread summaries
- canonical result cards
- canonical message cards
- connector-derived facts
- evidence-bearing recall content

Relevant code:

- [internal/memory/store.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/memory/store.go)
- [internal/recall/service.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/recall/service.go)
- [internal/skills/runner.go](/Users/jiachongliu/My-Github-Project/MnemosyneOS/internal/skills/runner.go)

## Working Memory vs Long-Term Memory

These systems should cooperate, but they should not be collapsed into one thing.

### Working Memory Should Optimize For

- continuity
- low-latency follow-up handling
- short-horizon context
- session recovery

### Long-Term Memory Should Optimize For

- durable recall
- evidence-backed reuse
- cross-session retrieval
- stable card/edge structures

## Promotion Rules

Not everything that appears in working memory should become long-term memory.

Promotion into long-term memory should be intentional.

Examples that should usually stay in working memory:

- transient pending questions
- temporary focus markers
- intermediate routing hints
- unresolved approval state

Examples that can be promoted:

- durable summaries
- evidence-backed search results
- stable email thread summaries
- task outputs with future reuse value

## Memory Correctness Requirements

Industrial memory needs more than retrieval.

It needs correctness guarantees around:

- what gets written
- when it gets written
- whether it should have remained transient
- whether later recall uses the right durable source

That implies three evaluation domains:

1. `working memory continuity`
2. `long-term memory durability`
3. `recall correctness`

## Memory And Harness

Memory must be harnessed directly.

The harness should verify:

- memory card count
- memory card content
- edge count
- recall hit content
- session state continuity

If memory is not asserted in scenarios, it is not an engineering guarantee.

## Recommended Scenario Classes

### Working Memory Follow-Up

Goal:

- prove that a second-turn follow-up uses the current session state instead of drifting

Checks:

- topic preserved
- focus task preserved
- pending question/follow-up interpreted correctly
- reply references prior artifact context

### Memory Write / Recall Roundtrip

Goal:

- prove that a workflow writes durable memory and that recall can retrieve it correctly

Checks:

- summary card written
- canonical cards written
- edges written
- recall hits contain expected content

### Memory Contamination Resistance

Goal:

- prove that bad intermediate output does not become final durable fact

### Approval Boundary Memory

Goal:

- prove that unapproved actions are not persisted as completed outcomes

### Session Recovery Continuity

Goal:

- prove that after restart, the session still has enough working state to continue correctly

## Current Direction

Near-term memory work should focus on:

1. stronger working-memory continuity
2. better long-term-memory assertions in harness
3. promotion boundary discipline
4. recall correctness under multi-turn usage

The project should treat memory as an operational subsystem, not just a recall feature.
