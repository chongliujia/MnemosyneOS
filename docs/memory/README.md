# Memory Design Docs

This directory is the canonical home for MnemosyneOS memory design documents.

The memory system is treated as a first-class runtime subsystem rather than a retrieval add-on.

## Core Documents

- [`MEMORY_STRATEGY.md`](./MEMORY_STRATEGY.md)
  Strategic direction for building a cue-driven, layered memory system.
- [`MEMORY_ARCHITECTURE.md`](./MEMORY_ARCHITECTURE.md)
  High-level split between working memory, durable memory, recall, and harness validation.
- [`WORKING_MEMORY_SPEC.md`](./WORKING_MEMORY_SPEC.md)
  Operational spec for session/task working memory.
- [`MEMORY_ORCHESTRATION_SPEC.md`](./MEMORY_ORCHESTRATION_SPEC.md)
  Retrieval, activation, and context-packet construction spec.
- [`MEMORY_CONSOLIDATION_SPEC.md`](./MEMORY_CONSOLIDATION_SPEC.md)
  Candidate promotion, consolidation, fact lifecycle, and archive spec.
- [`PROCEDURAL_MEMORY_SPEC.md`](./PROCEDURAL_MEMORY_SPEC.md)
  Procedural memory schema, promotion rules, and harness validation targets.

## Design Principles

- Working memory first, durable memory second.
- Retrieval should be cue-driven and bounded.
- Durable writes should be promoted deliberately, not emitted by default.
- Memory should accumulate method as well as facts.
- Memory behavior must remain harnessable.
