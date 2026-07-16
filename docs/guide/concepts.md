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
| `minerva skill activate` | Flips Minerva state file |
| local-agent profile apply | Activates profile skills **in session** |
| local-agent `load_skill` | One-shot body, does not flip Active |

For durable behavior: **put skills on a profile**, then start local-agent with that profile.

## Suggest philosophy

Suggestions are **proposals**:

- ranked by priority  
- include exact CLI next actions when possible  
- `--apply` only runs allowlisted `minerva skill activate <name>`  
- never mutates Cortex tasks or MCPHub configs automatically  

## Evidence philosophy

Outcomes live in **fcheap**, not a second vault:

```text
tags: minerva, minerva-eval|stack|…, outcome:pass|fail, skill:name, profile:name
```

Suggest can then say “skill X appears in N failed stashes” instead of guessing.
