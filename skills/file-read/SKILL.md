# File Read

## Purpose
Read a bounded file through the execution plane and persist the result as an artifact.

## When To Use
Use this skill when a task needs controlled file inspection inside the allowed workspace roots.

## Required Metadata
- `path`

## Approval
- `user` profile runs immediately.
- `root` profile uses the formal approval flow before execution.
