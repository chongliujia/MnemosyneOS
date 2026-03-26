# MnemosyneOS Architecture v1

## 1. Design Principles

- Event sourced first: every memory mutation is an immutable journal event.
- Temporal correctness over convenience: queries must support "as-of T".
- Evidence before assertion: semantic facts are traceable to episodic evidence.
- Rebuildability: projections and indexes are disposable and replayable.
- Governance as core path: consolidation/decay/reconsolidation are first-class.

## 2. System Layers

### 2.1 Journal Layer (source of truth)

Responsibilities:
- Append-only event persistence.
- Ordered sequence IDs and timestamps.
- Crash-safe fsync policy.
- Replay cursor and idempotent re-processing support.

Core events:
- `CardCreated`
- `CardUpdated`
- `CardLinked`
- `CardSuperseded`
- `ActivationTouched`
- `ConsolidationApplied`
- `DecayApplied`
- `CompactionCheckpointed`

### 2.2 Card Layer (structured memory units)

Card types:
- `event` (episodic)
- `fact` (semantic)
- `preference`
- `plan`
- `goal_kernel`

Unified envelope:
- `card_id` (ULID/UUIDv7)
- `card_type`
- `created_at`
- `valid_time` (`valid_from`, `valid_to`)
- `version` (`v`, `prev_version_id`, `status`)
- `content` (typed payload)
- `evidence_refs` (list of event card ids + snippets/hash)
- `provenance` (`agent_id`, `source`, `confidence`)
- `activation_state` (`score`, `last_access_at`, `decay_policy`)

### 2.3 Dual Graph Layer

Two logical graphs over shared card IDs:
- Episodic Graph: temporal/narrative edges.
- Semantic Graph: ontology/claim edges.

Edge model:
- `edge_id`
- `from_card_id`
- `to_card_id`
- `edge_type` (e.g. `causes`, `supports`, `contradicts`, `same_as`, `about`)
- `weight` and `confidence`
- `valid_time`
- `evidence_refs`

Evidence bridge rule:
- Any semantic claim with confidence above threshold must point to at least one episodic evidence path.

### 2.4 Governance Layer

Subsystems:
- Consolidation daemon: promotes stable event patterns to fact cards.
- Reconsolidation engine: resolves conflicts and version supersession.
- Activation/decay engine: updates activation scores and archives cold memory.
- Compaction/rebuild manager: snapshots and replay checkpoints.

Policies:
- Conflict policy: keep both versions + mark one `active`, one `disputed/superseded`.
- Freshness policy: decays confidence/activation by time and access patterns.
- Safety policy: never hard-delete source evidence in hot window.

### 2.5 Query & Retrieval Layer

Query modes:
- Exact lookup (`card_id`, `entity`, `edge relation`).
- Temporal query (`as_of`, `between`).
- Evidence-backed answer (`fact + minimal evidence subgraph`).
- Narrative reconstruction (episodic chain walk).

Retrieval contract:
- Return payload plus:
  - `version_context`
  - `temporal_context`
  - `evidence_subgraph`
  - `confidence_explanation`

### 2.6 API Layer (Memory VFS)

Minimal API surface:
- `PUT /cards` create card
- `PATCH /cards/{id}` new version update
- `POST /edges` link cards
- `GET /query` filter/temporal/evidence query
- `POST /consolidate` trigger consolidation job
- `POST /rebuild` replay and projection rebuild
- `GET /health` liveness/readiness + lag metrics

### 2.7 Architecture Diagram (high-level)
Figure 1. High-Level Layered Architecture

```mermaid
flowchart TB
    Client["AI Agent / App"]
    API["Memory VFS API"]
    Client --> API

    subgraph Runtime["Core Runtime"]
        CMD["Write Path"]
        QRY["Query Layer"]
        JR["Journal (source of truth)"]
        PRJ["Projector / Replay Worker"]
        GOV["Governance Layer"]
    end

    subgraph Stores["Projections & Indexes"]
        CARD["Card Projection Store"]
        GRAPH["Dual Graph Projection Store"]
        IDX["Temporal / Entity / Activation Indexes"]
    end

    API --> CMD
    API --> QRY

    CMD --> JR
    JR --> PRJ
    PRJ --> CARD
    PRJ --> GRAPH
    PRJ --> IDX

    QRY --> CARD
    QRY --> GRAPH
    QRY --> IDX
    QRY --> RESP["Answer + Evidence + Confidence"]
    RESP --> Client

    GOV -. policies .-> JR
    GOV -. maintenance .-> PRJ

    classDef entry fill:#E8F0FE,stroke:#4A78C2,stroke-width:1px,color:#1E2F4F;
    classDef core fill:#EAF7EF,stroke:#3B8C5A,stroke-width:1px,color:#1F3B2A;
    classDef store fill:#FFF6E8,stroke:#B7791F,stroke-width:1px,color:#4A3312;
    classDef governance fill:#FDECEC,stroke:#C75151,stroke-width:1px,color:#4A1F1F;
    classDef output fill:#F3F4F6,stroke:#6B7280,stroke-width:1px,color:#1F2937;

    class Client,API entry;
    class CMD,QRY,JR,PRJ core;
    class CARD,GRAPH,IDX store;
    class GOV governance;
    class RESP output;
```
Read guide: Top-down view of request entry, core runtime, projection/index storage, and response output.

## 3. Storage Topology

Physical stores (can start single-node):
- Journal store: append-only log segments.
- Card projection store: latest and historical versions.
- Graph projection store: adjacency index for episodic/semantic edges.
- Auxiliary indexes:
  - temporal index (`valid_from`, `valid_to`)
  - entity index
  - activation index
  - evidence reverse index

Initial implementation recommendation:
- Journal: local segment files (or embedded KV).
- Projections/indexes: embedded DB for fast local iteration.
- Keep all projection schemas replay-derivable from journal.

## 4. Core Flows

### 4.1 Write flow (ingest/update/link)
1. Validate command against card schema.
2. Append immutable event to journal.
3. Async projector updates card and graph projections.
4. Emit metrics/traces.

Write sequence:
Figure 2. Write Path Sequence

```mermaid
sequenceDiagram
    autonumber
    actor Client
    box rgb(232,240,254) Entry Layer
        participant API
    end
    box rgb(234,247,239) Core Layer
        participant Journal
        participant Projector
    end
    box rgb(255,246,232) Storage Layer
        participant CardStore as Card Projection
        participant GraphStore as Graph Projection
        participant Indexes
    end

    Client->>API: Write command
    API->>API: Validate + authorize
    API->>Journal: Append event
    Journal-->>API: ACK(seq_id, timestamp)
    API-->>Client: Accepted

    Journal-->>Projector: Stream event
    Projector->>CardStore: Update card view
    Projector->>GraphStore: Update graph view
    Projector->>Indexes: Refresh indexes
```
Read guide: Writes are acknowledged after journal append, then asynchronously projected into card/graph/index views.

### 4.2 Read flow (as-of/evidence)
1. Parse query + temporal constraints.
2. Read from projections.
3. Resolve version chain at `as_of`.
4. Build minimal evidence subgraph.
5. Return answer + confidence metadata.

Read sequence:
Figure 3. Read Path Sequence

```mermaid
sequenceDiagram
    autonumber
    actor Client
    box rgb(232,240,254) Entry Layer
        participant Query as Query Layer
    end
    box rgb(255,246,232) Storage Layer
        participant Indexes as Temporal/Entity Indexes
        participant CardStore as Card Projection
        participant GraphStore as Graph Projection
    end

    Client->>Query: Query(entity, as_of, mode)
    Query->>Indexes: Resolve candidate ids
    Query->>CardStore: Load version chain
    Query->>GraphStore: Load evidence paths
    Query->>Query: Resolve active version at as_of
    Query->>Query: Build minimal evidence subgraph
    Query-->>Client: Answer + context + confidence
```
Read guide: Reads resolve candidates from indexes, then assemble version-correct answers with evidence context.

### 4.3 Consolidation flow
1. Scan recent episodic patterns.
2. Propose candidate fact cards.
3. Attach evidence chain.
4. Write consolidation events.
5. Re-score related nodes/edges.

### 4.4 Reconsolidation flow
1. Detect contradictions by entity/time overlap.
2. Create new superseding fact version.
3. Mark prior version status and keep lineage.
4. Preserve both for audit and temporal querying.

Fact lifecycle:
Figure 4. Fact Version Lifecycle

```mermaid
flowchart TB
    C["Candidate"] -->|evidence >= threshold| A["Active"]
    A -->|contradiction detected| D["Disputed"]
    D -->|new evidence reconciles| A
    A -->|newer version created| S["Superseded"]
    D -->|resolved by newer version| S
    S -->|decay/compaction window reached| R["Archived"]

    classDef entry fill:#E8F0FE,stroke:#4A78C2,stroke-width:1px,color:#1E2F4F;
    classDef core fill:#EAF7EF,stroke:#3B8C5A,stroke-width:1px,color:#1F3B2A;
    classDef governance fill:#FDECEC,stroke:#C75151,stroke-width:1px,color:#4A1F1F;
    classDef output fill:#F3F4F6,stroke:#6B7280,stroke-width:1px,color:#1F2937;

    class C entry;
    class A core;
    class D,S governance;
    class R output;
```
Read guide: Facts move from candidate to active, then to disputed/superseded, and finally archived by governance policy.

## 5. Reliability & Observability

Reliability:
- At-least-once projection with idempotent handlers.
- Replay from checkpoint after crash.
- Deterministic projector outputs for same event stream.

Observability:
- Metrics: append latency, projector lag, query p95, conflict rate, decay rate.
- Tracing: command -> journal append -> projection update -> query path.
- Auditing: every returned fact has resolvable event lineage.

## 6. Security & Multi-Agent Isolation

- Namespace key: `tenant_id + agent_id`.
- Access control at query and mutation boundaries.
- Encryption at rest for journal segments (phase 2+).
- Redaction support via tombstone overlay (without breaking audit chain).

## 7. Suggested Codebase Layout

```text
mnemosyneos/
├── core/
│   ├── journal/
│   ├── cards/
│   ├── graph/
│   ├── activation/
│   ├── consolidation/
│   └── governance/
├── api/
│   └── memory_vfs/
├── benchmarks/
└── docs/
    ├── design/
    ├── research/
    └── benchmarks/
```

## 8. MVP Boundary (Phase 1 target)

Must-have:
- Append-only journal + replay.
- Card create/update with version chain.
- Dual graph link creation.
- Basic temporal query (`as_of`).
- Evidence pointer support.

Can defer:
- Advanced decay policies.
- Full compaction.
- Horizontal sharding.
- Complex ontology management.

## 9. Open Decisions

- Storage engine choice for local-first MVP.
- Event schema serialization format.
- Consistency model between write ACK and projection visibility.
- Confidence scoring formula and decay parameters.
- Evidence snippet hashing and immutable anchoring strategy.
