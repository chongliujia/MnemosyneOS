# MnemosyneOS

> A Cognitive-Inspired Long-Term Memory Operating System for AI Agents
>
> Episodic–Semantic Dual-Graph Memory with Versioned Facts, Evidence Chains, and Temporal Reasoning.


It models memory as:

* Structured **Memory Cards**
* Dual Graph architecture (Episodic + Semantic)
* Temporal validity & versioned facts
* Evidence-backed knowledge
* Consolidation & reconsolidation mechanisms
* Activation-driven decay
* Event-sourced journaling

MnemosyneOS is both:

* A research framework for cognitive-inspired AI memory
* A production-ready long-term memory service for AI systems

---

## Why MnemosyneOS?

Most AI memory systems today:

* Store text chunks in vector databases
* Lack version control
* Cannot handle fact evolution
* Do not model temporal validity
* Struggle with contamination and correction
* Have no long-term governance layer

MnemosyneOS treats memory as a structured, evolving cognitive system.

---

## Core Architecture

### 1. Journal Layer

Append-only event log
Crash-recoverable
Replayable

### 2. Card Layer

Structured memory units:

* Event Cards (episodic memory)
* Fact Cards (semantic memory)
* Preference Cards
* Plan/Commitment Cards
* Self/Goal Kernel Cards

### 3. Dual Graph Layer

* Episodic Graph (narrative continuity)
* Semantic Graph (knowledge structure)
* Evidence Bridges between them

### 4. Governance Layer

* Consolidation daemon
* Reconsolidation (conflict handling)
* Activation-based decay
* Compaction & log rotation
* Projection rebuild & verification

---


```plaintext
mnemosyneos/
│
├── README.md
├── ARCHITECTURE.md
├── ROADMAP.md
├── BENCHMARK.md
│
├── docs/
│   ├── research/
│   ├── design/
│   └── benchmarks/
│
├── core/
│   ├── journal/
│   ├── cards/
│   ├── graph/
│   ├── activation/
│   ├── consolidation/
│   └── governance/
│
├── api/
│   └── memory_vfs/
│
├── benchmarks/
│   ├── temporal_correctness/
│   ├── contamination_tests/
│   ├── narrative_tests/
│   └── stability_tests/
│
└── examples/
    └── agent_integration/
```
--


## Research Goals

MnemosyneOS aims to explore:

* Cognitive memory modeling for AI
* Temporal fact evolution
* Memory contamination resistance
* Dual-channel retrieval (reliable + narrative)
* Long-running AI memory benchmarking

---

## Engineering Goals

* 24/7 long-running memory core
* Multi-agent support
* Versioned knowledge graph
* Evidence-backed retrieval
* Minimal explanation subgraph extraction
* Rebuildable indexing
* Horizontal scalability (future)

---

# 四、ROADMAP

## Phase 0 – Concept Stabilization

* Define Card schema
* Define Edge types
* Define Dual Graph structure
* Define Memory-VFS API

## Phase 1 – Minimal Memory Core

* Event journal
* Card creation
* Graph linking
* Basic querying
* As-of temporal queries

## Phase 2 – Consolidation Engine

* Replay mechanism
* Event → Fact upgrade rules
* Version chain support
* Conflict detection

## Phase 3 – Governance & Stability

* Activation model
* Decay & compaction
* Rebuildable projections
* Soak testing (24h+)

## Phase 4 – Benchmark Suite

* Temporal correctness benchmark
* Evidence integrity benchmark
* Memory contamination benchmark
* Narrative coherence benchmark

---


