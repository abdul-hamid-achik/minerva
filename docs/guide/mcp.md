# MCP integration

```bash
minerva mcp serve
```

Exposes a **compact, read-only** session surface over stdio for MCPHub / sonar / local-agent. Mutations stay on the CLI so harnesses can approval-gate them.

## Product tools (9)

| Tool | Use when |
|------|----------|
| `minerva_learn` | First time / how Minerva works |
| `minerva_status` | Unified doctor / is the stack ready? |
| `minerva_suggest` | Ranked next library/stack actions |
| `minerva_resolve_skill` | Which skill should I load for *this* task? |
| `minerva_skill` | `action=list\|show\|compare` |
| `minerva_profile` | `action=list\|show\|compare` |
| `minerva_library` | `action=lint` |
| `minerva_stack_check` | PATH presence only |
| `minerva_evidence` | `action=docs\|search` |

There are no MCP aliases for the old 36 CRUD names. Create, activate, delete, add-skills, export/import, and evidence save/close are CLI-only.

## MCPHub

Do not commit a live `~/.config/mcphub/mcphub.yaml` (vault refs). Merge this
fragment, then restart the gateway. Full loop: [Session-native](/guide/session-native).

```yaml
expose: lazy
pin:
  - minerva__learn
  - minerva__resolve_skill
  - minerva__status
  - minerva__suggest
servers:
  minerva:
    command: minerva   # or an absolute path to the binary you built
    args: [mcp, serve]
    enabled: true
    tags: [agent, skills, profiles, readiness]
    use_when:
      - Resolve which skill to load for the current task
      - Check whether the shared agent library or companion stack is ready
      - Get a one-page Minerva onboarding brief
      - Rank next library or stack actions without mutating disk
```

## Trust (sonar / local-agent)

Exact routes only. All nine product tools are read-only:

`minerva__learn`, `minerva__status`, `minerva__suggest`, `minerva__resolve_skill`, `minerva__skill`, `minerva__profile`, `minerva__library`, `minerva__stack_check`, `minerva__evidence`

`minerva skill activate` does **not** inject into a live sonar or local-agent session.

## Lazy mode

```text
mcphub_resolve_tool → minerva__resolve_skill | minerva__learn | minerva__status
mcphub_call_tool
```
