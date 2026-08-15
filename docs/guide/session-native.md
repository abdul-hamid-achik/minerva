# Session-native Minerva

Minerva is the library operator for `~/.agents` and a stack readiness
orchestrator. It is not a second agent runtime.

A harness that already speaks MCPHub (sonar, local-agent, Grok) should be able
to ask two session questions without loading 36 look-alike tools:

1. Which skill should I load *for this task*?
2. What is the honest next library or stack action?

Mutations stay on the CLI so the harness can approval-gate them.

## Authority

| Domain | Owner |
|--------|--------|
| Prompt assembly, `/skill`, `load_skill`, `/agent` | sonar / local-agent |
| MCP trust, AUTO, ledger | the harness |
| Lazy catalog, `resolve_tool`, call stats | MCPHub |
| Task evidence | Cortex |
| Files under `~/.agents` | disk SSOT — Minerva writes, harnesses read |
| `.minerva-skills.json` | Minerva-local only — harnesses do not read it |
| `learn`, `resolve_skill`, presence vs readiness, suggest | Minerva |

`minerva skill activate` does not inject into a live session. Put skills on a
profile (`minerva profile add-skills`) for durable loading. Use the harness
`load_skill` for a one-shot body.

## Nine read-only MCP tools

| Tool | Use when |
|------|----------|
| `minerva_learn` | First time / how Minerva works |
| `minerva_status` | Unified doctor — is the stack actually ready? |
| `minerva_suggest` | Ranked next library or stack actions |
| `minerva_resolve_skill` | Which skill should I load for *this* task? |
| `minerva_skill` | `action=list\|show\|compare` |
| `minerva_profile` | `action=list\|show\|compare` |
| `minerva_library` | `action=lint` |
| `minerva_stack_check` | PATH presence only |
| `minerva_evidence` | `action=docs\|search` |

There are no MCP aliases for the old CRUD names. Create, activate, delete,
add-skills, export/import, and evidence save/close are CLI-only.

## MCPHub config (safe fragment)

Do not commit a live `mcphub.yaml` — it often holds vault refs. Copy this
fragment into `~/.config/mcphub/mcphub.yaml` and point `command` at the
Minerva you actually built.

```yaml
expose: lazy
pin:
  - minerva__learn
  - minerva__resolve_skill
  - minerva__status
  - minerva__suggest
servers:
  minerva:
    command: minerva   # or an absolute path to bin/minerva
    args: [mcp, serve]
    enabled: true
    description: Agent library operator and stack readiness
    tags: [agent, skills, profiles, readiness]
    use_when:
      - Resolve which skill to load for the current task
      - Check whether the shared agent library or companion stack is ready
      - Get a one-page Minerva onboarding brief
      - Rank next library or stack actions without mutating disk
```

Restart the MCPHub gateway after changing pins or the Minerva binary.

## Sonar

Sonar already loads `~/.agents` (`/agent`, `load_skill`). It should call
Minerva through MCPHub, not `./bin/minerva`, except when dogfooding the
Minerva repo itself.

Default trust lists the nine `minerva__*` routes as read-only. AUTO may run
query-only `./bin/minerva learn|status|skill show|skill resolve|library lint`
from a workspace-built `bin/minerva`. Activate, apply, delete, `mcp serve`,
and `stack deep` stay approval-gated.

```bash
minerva learn --json
minerva skill resolve "review this pull request" --json
minerva bridge show <profile> --harness sonar
```

## Profiles

Optional `kind: workspace|role` on `agent.yaml`. Names `default`, `dev`, and
`workspace` infer workspace. Suggest only auto-adds skills to workspace
profiles — never to a specialist like `code-reviewer`.

## Out of scope here

Cross-harness skill sync (`~/.grok/skills`, `~/.claude/skills`, `.skill-lock.json`),
a Studio TUI, and mid-session injection that bypasses `load_skill`.
