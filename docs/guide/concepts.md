# Concepts

## Product thesis

> Minerva is the **agent library operator** and **stack readiness orchestrator** for the shared `~/.agents` tree. It is not a second agent runtime.

## Shared vs private state

### Shared (local-agent consumes)

| Path | Role |
|------|------|
| `~/.agents/skills/*/SKILL.md` | Skill bodies + frontmatter |
| `~/.agents/agents/*/agent.yaml` | Profiles: model, skills, mcp_servers, system_prompt |
| `agents.md` / `instructions.md` | Global instructions (if present) |

### Minerva-local (not read by local-agent)

| Path | Role |
|------|------|
| `~/.agents/.minerva-skills.json` | Activation flags for Minerva catalog only |
| `~/.agents/.minerva-analytics.json` | Append-safe usage events |

## Activation honesty

| Action | Effect |
|--------|--------|
| `minerva skill activate` | Flips Minerva state file only |
| `minerva profile add-skills` | Updates shared `agent.yaml` (local-agent SSOT) |
| local-agent profile apply | Activates profile skills **in session** |
| local-agent `load_skill` | One-shot body, does not flip Active |

For durable behavior: **put skills on a profile**, then start local-agent with that profile.

## Suggest philosophy

Suggestions are **proposals**:

- ranked by priority  
- include exact CLI next actions when possible  
- prefer **profile membership** over Minerva-local activate  
- `--apply` runs allowlisted `minerva profile add-skills …`  
- `--apply-local` also allows `minerva skill activate <name>`  
- never mutates Cortex tasks or MCPHub configs automatically  

## Evidence philosophy

Outcomes live in **fcheap**, not a second vault:

```text
tags: minerva, minerva-eval|stack|…, outcome:pass|fail, skill:name, profile:name
```

Suggest can then say “skill X appears in N failed stashes” instead of guessing.

## Templates (builtin + disk)

| Layer | Location |
|-------|----------|
| Builtin | Embedded in the Minerva binary |
| Disk | `~/.agents/templates/<name>/template.yaml` |

Disk templates **override** builtins with the same name. Use `minerva template install <name>` to copy a builtin for editing, or `template save` for new roles.

## Library portability

```bash
minerva library export ./team-lib.tgz
minerva library import ./team-lib.tgz --force
minerva library lint
```

Bundles include skills, profiles, and templates — not Minerva-local analytics/activation state.

## Bridge to local-agent

`minerva bridge show <profile>` prints launch examples and **exact** MCP trust routes. Minerva never starts the harness; it documents how the shared disk SSOT is consumed.
