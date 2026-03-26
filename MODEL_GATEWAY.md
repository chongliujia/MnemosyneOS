# MnemosyneOS Model Gateway

## Goal

Provide a provider-agnostic model interface for the AI user runtime, with a text-first MVP path and an optional multimodal extension for GUI-driven workflows.

## Core decision

The platform should support model access through APIs and should not be bound to any single model vendor.

The model strategy is:

- text models are sufficient for the MVP
- multimodal models are enabled when the AI user must interpret screenshots, browser pages, or desktop applications

## Why text-first

Text models are enough for early platform milestones:

- terminal interaction
- shell execution
- file editing
- task planning
- memory consolidation
- skill selection
- audit summarization

This keeps:

- complexity lower
- latency lower
- cost lower
- integration easier

## Why multimodal support is still required

Once the AI user needs to operate like a real desktop user, text-only reasoning is insufficient for:

- screenshot inspection
- browser page layout understanding
- desktop UI state interpretation
- button, modal, and dialog recognition
- visual confirmation of side effects

For these workflows, multimodal perception becomes necessary.

## Gateway design

The runtime should talk to a single internal model gateway.

Example internal capabilities:

- `generate_text`
- `analyze_image`
- `call_tools`
- `embed_text`
- `rerank_candidates`
- `transcribe_audio`

The gateway then routes requests to configured providers.

## Provider strategy

Supported provider classes:

- OpenAI-compatible APIs
- Anthropic
- Gemini
- local model backends such as `Ollama` or `vLLM`

The runtime should depend on capability contracts, not provider-specific APIs.

## Routing model

### 1. Text route

Use for:

- planning
- code and shell reasoning
- memory summarization
- skill selection
- safety review
- task decomposition

### 2. Vision route

Use only when required by the task:

- screenshot analysis
- browser page inspection
- desktop application understanding
- OCR-assisted UI reading

### 3. Utility route

Use specialized cheaper models or services for:

- embeddings
- reranking
- OCR post-processing
- classification
- short summaries

## Role mapping

Recommended model assignment by runtime role:

- `planner`: text model
- `executor`: minimal model usage, mostly structured execution
- `observer`: text model for logs/files, vision model for screenshots/UI
- `memory steward`: text model
- `reviewer/safety`: text model, vision only if verifying GUI state

## Cost control principles

- default to text models
- call vision only when visual evidence is necessary
- avoid sending full desktop history to the model
- pass handles, summaries, and structured observations instead of raw transcripts where possible
- separate embedding/rerank workloads from premium reasoning models

## API shape

The external configuration can vary by provider, but the internal interface should remain stable.

Suggested internal request types:

- `TextRequest`
- `VisionRequest`
- `ToolCallRequest`
- `EmbeddingRequest`
- `RerankRequest`

Suggested shared response fields:

- `provider`
- `model`
- `latency_ms`
- `input_tokens`
- `output_tokens`
- `cost_estimate`
- `trace_id`

## Failure strategy

The gateway should support:

- provider fallback
- retry policy by request type
- timeouts per route
- capability-aware degradation

Example:

- if no vision model is configured, GUI tasks should fail fast or fall back to OCR-only mode with lower confidence
- if premium text models are unavailable, low-risk summarization may fall back to a local model

## MVP recommendation

Phase 1:

- one text provider
- no mandatory vision path
- embeddings optional

Phase 2:

- add multimodal provider support
- add screenshot/browser observation pipeline
- add utility model routes for embeddings and reranking

## Platform rule

Multimodality is a platform capability, not a platform dependency.

The system must remain functional in text-only mode for terminal, filesystem, memory, and skill-driven workflows.
