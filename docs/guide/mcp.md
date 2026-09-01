# MCP integration

```bash
minerva mcp serve
```

Compact stdio surface for MCPHub / Claude Code / Cursor / Codex.

## Product tools (8)

| Tool | Use when |
|------|----------|
| `minerva_learn` | First time / how Minerva works |
| `minerva_sessions` | List recent harness conversations |
| `minerva_analyze` | Extract signals from sessions |
| `minerva_propose` | Rank skill drafts with evidence |
| `minerva_resolve_skill` | Which catalog skill matches this intent? |
| `minerva_skill` | `action=list\|show\|compare` |
| `minerva_harness` | `action=list\|doctor` |
| `minerva_apply` | Write a proposal id to `~/.agents/skills` |

Only `minerva_apply` mutates disk. Gate it.

## MCPHub

```yaml
expose: lazy
pin:
  - minerva__learn
  - minerva__resolve_skill
  - minerva__sessions
  - minerva__analyze
  - minerva__propose
servers:
  minerva:
    command: minerva
    args: [mcp, serve]
    enabled: true
    tags: [agent, skills, sessions]
    use_when:
      - Analyze a harness conversation and propose skills
      - Resolve which skill to load for the current task
      - Check skill-dir drift across harnesses
```

Restart the gateway after changing pins or the Minerva binary.
