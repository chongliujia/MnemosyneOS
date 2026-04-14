# Skill Registry

The runtime should not require editing the core `skills.Runner` switch every time a new skill is added.

## Current Direction

`internal/skills` now has a registry layer:

- `Definition`
- `MaintenancePolicy`
- `Registry`
- `ManifestStatus`

This lets the runtime separate:

1. skill dispatch
2. skill description
3. maintenance policy
4. manifest load health

## Registration Model

Skills are registered programmatically through `skills.Runner.RegisterSkill(...)`.

Each skill definition provides:

- `name`
- `description`
- `handler`
- optional `maintenance_policy`

## Management Model

The current management flow is:

1. Built-in skills register during runner construction.
2. Manifest skills load from `<runtime-root>/skills/*.json`.
3. Persisted enable/disable overrides load from `<runtime-root>/state/skills.json`.
4. Skill inventory is exposed through `GET /skills`.
5. Skills can be reloaded through `POST /skills/reload`.
6. Skills can be enabled or disabled through `PATCH /skills/{name}`.
7. The web console exposes the same surface at `/ui/skills`.
8. Manifest files can be created or updated through:
   - `PUT /skills/manifests/{name}`
   - `POST /ui/skills/manifests`
8. External code-backed skills can still register through a stable API instead of editing core dispatch code.

## Manifest Shape

The manifest format is JSON:

```json
{
  "version": 1,
  "name": "web-research",
  "description": "Alias for web search with its own policy.",
  "uses": "web-search",
  "enabled": true,
  "default_metadata": {
    "query_style": "research"
  },
  "execution_profile": "user",
  "maintenance_policy": {
    "enabled": true,
    "scope": "project",
    "allowed_card_types": ["web_result"],
    "min_candidates": 1
  }
}
```

`version` is currently required to stay at `1` when present. Any future manifest migration must use an explicit version bump; unsupported versions are rejected at load time.

`uses` must point to an already-registered handler. This keeps manifest loading safe and deterministic.

## External Skill Manifests

The second-stage external extension model supports command-backed skills:

```json
{
  "name": "external-research",
  "description": "Run an external command adapter.",
  "external": {
    "kind": "command",
    "command": "./external-research.sh",
    "args": ["--mode", "fast"],
    "timeout_ms": 2000
  }
}
```

The external command receives JSON on stdin:

```json
{
  "task": { "...": "full runtime task payload" },
  "runtime_root": "/path/to/runtime"
}
```

By default the external adapter does **not** expose the runtime root. The command only receives:

- the current task payload
- `MNEMOSYNE_SKILL_NAME`
- `MNEMOSYNE_TASK_ID`

`runtime_root` and `MNEMOSYNE_RUNTIME_ROOT` are only exposed when `external.allow_write_root=true`.

Every external run also emits telemetry into task metadata and an observation record, including:

- manifest path
- command path
- duration
- approval requirement
- runtime root exposure flag

It must return JSON on stdout shaped like:

```json
{
  "state": "done",
  "next_action": "external skill completed",
  "metadata": {"source": "external"},
  "artifacts": [
    {"kind": "reports", "name": "report.md", "content": "# Report"}
  ],
  "observations": [
    {"kind": "os", "name": "report.json", "payload": {"summary": "ok"}}
  ]
}
```

## API Surface

- `GET /skills`
  - returns the current registry, including builtin, manifest, and external skills
  - also returns `manifest_statuses`
- `POST /skills/reload`
  - rescans `<runtime-root>/skills/*.json` and reapplies persisted enabled state
  - reports manifest load errors without discarding successfully loaded skills
- `PUT /skills/manifests/{name}`
  - validates and writes a manifest to `<runtime-root>/skills/{name}.json`
  - reloads the registry after save
- `PATCH /skills/{name}`
  - request body: `{"enabled": true|false}`
  - persists the override to `<runtime-root>/state/skills.json`
- `GET /ui/skills`
  - operator view for registry state and manifest health
- `GET /ui/skills?manifest={name}`
  - loads the current manifest file into the editor
- `POST /ui/skills/reload`
  - reloads manifests from the operator surface
- `POST /ui/skills/manifests`
  - saves a raw JSON manifest from the operator surface
- `POST /ui/skills/{name}/toggle`
  - toggles enable/disable state from the operator surface

## Extension Model

The first external extension model is intentionally conservative:

1. Code-backed skills can register directly through `Runner.RegisterSkill(...)`.
2. Manifest-backed skills can alias an existing handler with:
   - a new public name
   - a new description
   - default metadata
   - an execution profile
   - a maintenance policy
3. Command-backed external skills can run outside the core process through the `external.kind=command` adapter.
4. The runtime can manage builtin, manifest, and external skills through the same list/reload/enable API surface.

This gives users a stable way to register and operate new skills without editing the core dispatch path.

## Validation

Manifest loading now rejects and reports:

- invalid skill names
- unsupported manifest versions
- missing `uses` / `external`
- manifests that define both `uses` and `external`
- invalid `execution_profile`
- invalid maintenance policy scopes
- unsupported external kinds
- missing external commands

Load failures are surfaced through `manifest_statuses` and the `/ui/skills` page.

## External Guardrails

The command-backed adapter now enforces:

- `external.command` must be relative to the manifest directory
- `external.workdir` must be relative to the manifest directory
- root-profile external execution requires `external.require_approval=true`
- runtime root exposure is disabled by default unless `external.allow_write_root=true`
