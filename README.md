# MnemosyneOS

> A Cognitive-Inspired Long-Term Memory Operating System for AI Agents
>
> Episodic–Semantic Dual-Graph Memory with Versioned Facts, Evidence Chains, and Temporal Reasoning.

---

## Overview

MnemosyneOS is a cognitive-inspired memory architecture designed for long-running AI assistants.

Unlike traditional vector-based memory systems, MnemosyneOS models memory as:

* **Memory Cards** (structured knowledge units)
* **Dual Graphs** (Episodic + Semantic)
* **Temporal validity and version chains**
* **Evidence-backed facts**
* **Consolidation and reconsolidation mechanisms**
* **Activation-based memory decay**
* **Self/Goal kernel for persistent identity**

It is built to support:

* 24/7 persistent AI agents
* Multi-agent shared memory
* Time-aware fact evolution
* Conflict detection and correction
* Narrative continuity and personalization
* Reliable, evidence-backed reasoning

---

## Architecture

MnemosyneOS consists of four layers:

1. **Journal Layer**

   * Append-only event stream
   * Crash-safe and replayable

2. **Card Layer**

   * Event cards (episodic memory)
   * Fact cards (semantic memory)
   * Preference, plan, and self-goal cards
   * Versioning and temporal validity

3. **Dual Graph Layer**

   * Episodic Graph (narrative structure)
   * Semantic Graph (fact structure)
   * Evidence bridges between them

4. **Governance Layer**

   * Consolidation daemon (replay & abstraction)
   * Reconsolidation (conflict handling)
   * Activation-based decay
   * Log rotation & compaction

---

## Key Features

* Evidence-backed knowledge
* Versioned and time-scoped facts
* Conflict-aware updates
* Minimal explanation subgraph extraction
* Activation-driven memory retention
* Crash-recoverable event sourcing
* Rebuildable projections

---

## Research Goals

MnemosyneOS aims to explore:

* Cognitive-inspired long-term memory modeling
* Temporal fact evolution
* Memory contamination resistance
* Reconsolidation dynamics
* Reliable AI memory benchmarking

---

## Evaluation (Planned)

MnemosyneOS includes a benchmark suite to test:

* Temporal correctness
* Evidence integrity
* Narrative coherence
* Memory pollution resistance
* Long-running stability


