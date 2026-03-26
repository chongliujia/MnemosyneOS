# MnemosyneOS Testbed Architecture

## Goal

Provide a safe, repeatable environment for validating an AgentOS runtime that can operate a full Linux/macOS system with either normal-user privileges or authorized root privileges.

## Current usage

This document describes the later-stage VM and hardware test environments.

Early development should not be VM-first.

For the active development loop, see [`TEST_STRATEGY.md`](./TEST_STRATEGY.md), which defines:

- local-machine-first development
- Docker as a constrained integration layer
- VMs and Raspberry Pi as later validation stages

## Accepted Test Stack

- Standard virtualization core: `QEMU`
- Mac local debugging: `UTM`
- Linux automated testing: `libvirt + virt-install + virt-manager`
- Real deployment validation: `Raspberry Pi 5 / ARM64 Linux`

## Why this stack

- `QEMU` is the common virtualization substrate across local debugging and automated test flows.
- `UTM` provides a practical macOS-native frontend for QEMU during iterative local development.
- `libvirt` gives a stable automation layer for Linux CI-like orchestration, cloning, and lifecycle control.
- `Raspberry Pi 5` gives a realistic low-power ARM64 target for long-running deployment validation.

## Environment Roles

### 1. Local Mac development

Use `UTM` on macOS for:

- fast guest creation
- local debugging
- manual GUI testing
- terminal and browser interaction tests
- snapshot-based recovery testing

Target guest:

- `Ubuntu` or `Debian` ARM64 preferred
- desktop image for GUI automation tests
- headless image for runtime-only tests

### 2. Linux automated test lane

Use `libvirt + virt-install + virt-manager` for:

- repeatable VM provisioning
- snapshot management
- headless automated test runs
- regression testing of AI-user workflows
- privilege boundary validation

Recommended host pattern:

- `QEMU/KVM` on Linux
- `virt-install` for provisioning
- `virsh` for lifecycle automation
- `virt-manager` only for interactive debugging when needed

### 3. Real deployment lane

Use `Raspberry Pi 5 / ARM64 Linux` for:

- long-run soak testing
- reboot and crash recovery validation
- resource-constrained runtime behavior
- real filesystem and device interaction
- realistic network and peripheral behavior

## Guest OS baseline

Default baseline for MVP:

- `Ubuntu 24.04 LTS ARM64` or `Debian 12 ARM64`

Reasoning:

- strong package ecosystem
- stable ARM64 support
- consistent behavior across QEMU and Raspberry Pi
- straightforward SSH and service management

## Image strategy

Keep the image lifecycle simple and reproducible.

Base layers:

- `base`: clean OS install
- `bootstrap`: AI runtime dependencies and common tools installed
- `profile-user`: user-mode runtime configured
- `profile-root`: root-capable runtime configured

Each layer should be reproducible from scripts or declarative config.

## Snapshot strategy

Use snapshots as explicit test checkpoints:

- `pre-bootstrap`
- `post-bootstrap`
- `pre-skill-run`
- `post-task-run`
- `failure-capture`

This makes it possible to:

- replay failures
- compare filesystem diffs
- reproduce destructive operations safely
- validate recovery logic

## Privilege testing

The runtime should support two execution profiles:

- `user mode`
- `root mode`

Test coverage must include:

- normal operation in user mode
- explicit elevation request path
- audit logging of root operations
- failure handling during privileged commands
- downgrade back to user mode after sensitive actions

## What to validate

### Functional validation

- file reads/writes
- shell command execution
- browser-assisted workflows
- package installation
- task continuation across restarts
- memory recall across sessions

### Safety validation

- root-mode gating
- command audit trails
- recovery after command failure
- rollback via snapshot
- containment of destructive actions inside VM

### Performance validation

- token usage per task
- task completion latency
- memory retrieval latency
- long-running process stability
- storage growth over time

## Artifact collection

Each test run should preserve:

- runtime logs
- task files
- observation files
- action results
- memory journal
- snapshots or snapshot metadata
- system metrics

These artifacts should be enough to replay or diagnose any failed run.

## Recommendation for later-stage validation

Use this order:

1. `QEMU/UTM/libvirt` once browser and full-system validation is needed
2. `Raspberry Pi 5` for long-running deployment validation

Do not optimize for VM workflows before the local and Docker-based development loops are stable.
