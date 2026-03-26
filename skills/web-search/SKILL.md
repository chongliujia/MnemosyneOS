---
name: web-search
description: Query a configured web search API, normalize results, and write a search artifact plus observation.
---

# web-search

Use this skill when a task needs external web information and a structured search API is available.

Inputs:

- task goal
- optional `metadata.query`

Outputs:

- `runtime/artifacts/reports/*-web-search.md`
- `runtime/observations/os/*-web-search.json`

This skill is API-first and does not require a browser session.
