# GitHub Issue Search

## Purpose
Search issues in a configured GitHub repository through the connector runtime.

## When To Use
Use this skill when a task needs repository issue context and the GitHub connector is configured.

## Required Metadata
- `query` (optional; defaults to task goal)

## Connector
- Requires `MNEMOSYNE_GITHUB_OWNER` and `MNEMOSYNE_GITHUB_REPO`
- Uses `MNEMOSYNE_GITHUB_TOKEN` when available
