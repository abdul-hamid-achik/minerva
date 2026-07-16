# MCP integration

```bash
minerva mcp serve
```

Exposes library, stack, templates, evidence, and suggest tools over **stdio** for MCPHub / local-agent.

## Representative tools

| Tool | Class |
|------|--------|
| `minerva_skill_list` / `show` / `compare` | read-only |
| `minerva_profile_list` / `show` | read-only |
| `minerva_stack_check` | read-only |
| `minerva_stack_deep` | read-only (`stash` optional) |
| `minerva_suggest` | read-only |
| `minerva_analytics` | read-only |
| `minerva_template_list` / `show` | read-only |
| `minerva_evidence_docs` | read-only |
| `minerva_skill_create` / `activate` / … | effectful |
| `minerva_evidence_save` | effectful |
| `minerva_profile_*` mutations | effectful |

## MCPHub

```yaml
servers:
  minerva:
    command: /path/to/minerva
    args: [mcp, serve]
    enabled: true
    tags: [agent, skills, profiles]
```

## Trust rules (local-agent)

- **Exact routes only** — no `minerva_*` wildcards  
- Enumerate live tools via introspection  
- Read-only in AUTO when possible  
- Mutations approval-gated  
- `minerva_skill_activate` does **not** inject into a live session  

## Lazy mode

Under MCPHub `expose: lazy`, discover via:

```text
mcphub_resolve_tool → minerva__…
mcphub_call_tool
```
