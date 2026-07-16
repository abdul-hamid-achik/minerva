# Minerva

Agent **library operator** and **stack readiness** CLI/MCP for the shared `~/.agents` tree.

Minerva manages skills and profiles on disk (the same layout [local-agent](https://github.com/abdul-hamid-achik/local-agent) loads), and probes companion tools (bob, cortex, mcphub, codemap, vecgrep, …) using **real binary names** and tiered health.

It is **not** a second agent runtime, not Cortex, and not MCPHub. Skill “activation” updates Minerva-local state only; local-agent injects skills via profiles, `/skill`, and `load_skill`.

Built with Go + Cobra + MCP stdio (same family as Bob / Cortex).

## What Minerva does

| Surface | Role |
|---|---|
| **Skills** | Create/list/show/compare/delete under `~/.agents/skills/` |
| **Profiles** | Create/list/show/update/delete `~/.agents/agents/*/agent.yaml` |
| **Templates** | Seed profiles with role prompts + skill bundles |
| **Stack check** | Presence probe (correct binaries, core vs optional tiers) |
| **Stack deep** | Bob/cortex/mcphub + readiness doctors/status |
| **Suggest** | Ranked library/stack suggestions (one engine for CLI + MCP) |
| **Analytics** | Append-safe Minerva-local event log |
| **MCP** | Expose the above over stdio for harnesses / MCPHub |

## Quick start

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)

### Install / build

```bash
task build              # bin/minerva
# or
go install github.com/abdul-hamid-achik/minerva/cmd/minerva@latest
```

### Initialize

```bash
minerva init
```

Creates:

```text
~/.agents/
  agents/       # agent profiles (agent.yaml)
  skills/       # skill definitions (SKILL.md)
  tasks/        # reserved
  memories/     # reserved
```

Override root with `MINERVA_AGENTS_DIR` (tests should always use this).

## CLI

### Skills

```bash
minerva skill list
minerva skill show <name>
minerva skill compare <a> <b>
minerva skill create <name> <content> [-d description]
minerva skill activate <name>     # Minerva-local only
minerva skill deactivate <name>
minerva skill delete <name>
```

### Profiles

```bash
minerva profile list
minerva profile show <name>
minerva profile compare <a> <b>
minerva profile create <name> [prompt] [-d desc] [-m model] [-s skill]...
minerva profile update-prompt <name> <prompt>
minerva profile update-skills <name> skill1,skill2
minerva profile delete <name>
```

### Stack

```bash
minerva stack check           # presence, tiered
minerva stack check --json
minerva stack deep [workspace]
minerva stack deep --json
minerva stack deep --stash    # save report to fcheap (minerva-stack, TTL 30d)
```

`stack deep` sets **`retrieval_ready`** only when both **codemap** and **vecgrep** are domain-ready (indexed, not stale). Binary-on-PATH is not enough.

MCPHub slice includes **unused_enabled** servers and harness **agents_drift** from `mcphub status --json`. Suggest will propose `mcphub disable <server>` for zero-call enabled servers and flag profile `mcp_servers` that name unknown hub servers.

Cortex slice uses **`cortex overview --json`** (sessions/active/stale/verified rates) plus sample stale sessions and active count for the current workspace. Suggest surfaces stale backlog and low verified rates — it never mutates cortex tasks.

**Binary map (product → PATH command):**

| Product | Binary | Tier |
|---|---|---|
| bob | `bob` | core |
| cortex | `cortex` | core |
| mcphub | `mcphub` | core |
| codemap | `codemap` | core |
| vecgrep | `vecgrep` | core |
| fcheap | `fcheap` | core |
| monitor | `monitor` | optional |
| hitspec | `hitspec` | optional |
| glyphrun | **`glyph`** | optional |
| cairntrace | **`cairn`** | optional |
| vidtrace | `vidtrace` | optional |
| tinyvault | **`tvault`** | optional |
| veclite | `veclite` | infra |

Core missing → unhealthy. Optional missing → degraded only.

### Suggest / analytics / templates

```bash
minerva suggest
minerva suggest --json
minerva suggest --apply          # only allowlisted skill activate actions
minerva analytics
minerva template list|show|apply
```

### Evidence (fcheap tags)

Durable outcomes go through [fcheap](https://github.com/abdul-hamid-achik/file.cheap) with standard tags — Minerva does not reimplement the vault.

```bash
minerva evidence docs
minerva evidence save ./run-artifacts --kind eval --outcome pass \
  --tag skill:qa-tester --tag profile:code-reviewer --index
minerva evidence search minerva-eval
fcheap list --tag minerva --tag outcome:fail --json
```

Tags always include `minerva`; kind adds `minerva-eval` / `minerva-suggest` / `minerva-stack` / `minerva-incident`; optional `outcome:pass|fail|skip`.  
Attribution: `skill:<name>`, `profile:<name>` — `minerva suggest` reads failed stashes for evidence-backed recommendations.

## MCP

```bash
minerva mcp serve
```

Tools include skill/profile CRUD, `minerva_stack_check`, `minerva_stack_deep`, `minerva_analytics`, `minerva_suggest`.

Wire via MCPHub:

```yaml
servers:
  minerva:
    command: minerva   # or absolute path to bin/minerva
    args: [mcp, serve]
    enabled: true
```

For local-agent, list exact trust routes (no wildcards). Prefer read-only tools in AUTO; gate mutations.

## Architecture

```text
cmd/minerva/
internal/
  cli/           Cobra commands
  mcp/           MCP stdio server
  skill/         Skill discovery + Minerva activation state
  profile/       Profile YAML management
  templates/     Role prompt templates
  monitor/       Presence probes (bins + tiers)
  integration/   Deep readiness (sibling CLIs)
  suggest/       Shared suggestion engine
  analytics/     Append-safe event store
  version/
```

### Contracts with local-agent

**Shared (disk SSOT):**

- `~/.agents/skills/*/SKILL.md`
- `~/.agents/agents/*/agent.yaml`
- optional `agents.md` / `instructions.md`

**Not shared:**

- `~/.agents/.minerva-skills.json` — Minerva activation only
- `~/.agents/.minerva-analytics.json` — Minerva analytics only

local-agent projects profile skills into session Active Skills at startup; it never reads Minerva activation state.

### Do not reimplement

MCPHub gateway/sync/telemetry · Cortex task lifecycle · Bob plan/apply · Codemap graph · Vecgrep index · fcheap vault · glyph/cairn runners · tvault secrets · monitor process ops

Minerva **orchestrates** their public `--json` surfaces.

## Development

```bash
task build
task test
task lint
task fmt
```

## License

MIT
