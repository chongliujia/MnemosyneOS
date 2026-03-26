# Shell Command

## Purpose
Execute a controlled shell command through the AgentOS execution plane.

## When To Use
Use this skill when a task needs a bounded shell command rather than a file mutation or web retrieval.

## Required Metadata
- `command`
- `args` (optional, whitespace-separated string)
- `workdir` (optional)

## Approval
- `user` profile runs immediately inside the execution allowlist.
- `root` profile enters the formal approval flow before execution.
