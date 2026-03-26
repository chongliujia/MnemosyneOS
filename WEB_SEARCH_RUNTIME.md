# MnemosyneOS Web Search Runtime

## Goal

Provide a stable API-first search connector for AgentOS tasks that need external information retrieval.

## Core rule

Use structured search APIs first.

This runtime exists to:

- issue search queries through a configured provider
- return normalized results
- keep token usage low
- avoid browser/session complexity in the main path

## Current providers

- `serpapi`
- `tavily`

Configuration is environment-driven:

- `MNEMOSYNE_WEB_SEARCH_PROVIDER`
- `MNEMOSYNE_WEB_SEARCH_API_KEY`
- `MNEMOSYNE_WEB_SEARCH_ENDPOINT` (optional override)

## Runtime behavior

- tasks with search/web goals map to the `web-search` skill
- the skill calls the configured search connector
- results are stored as artifacts and observations
- missing provider configuration blocks the task cleanly

## Output paths

- `runtime/artifacts/reports/*-web-search.md`
- `runtime/observations/os/*-web-search.json`

## Why API-first

- more stable than browser automation
- cheaper than page-level extraction
- easier to test
- easier to govern

## Future scope

- provider ranking and fallback
- domain filtering
- source deduplication
- evidence-backed memory writes
