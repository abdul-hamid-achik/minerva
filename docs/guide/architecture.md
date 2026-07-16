# Architecture

```text
cmd/minerva
internal/
  cli/            Cobra commands
  mcp/            MCP stdio server
  skill/          SKILL.md discovery + Minerva activation
  profile/        agent.yaml management
  templates/      role prompt seeds
  monitor/        presence (bins + tiers)
  integration/    deep readiness (sibling CLIs)
  suggest/        shared suggestion engine
  analytics/      append-safe events
  evidence/       fcheap tag conventions
  version/
docs/             VitePress site (this site)
specs/            Glyphrun CLI contracts
browser-specs/    Cairn docs-site contracts
```

## Design rules

1. **Disk is the SSOT** for library content shared with harnesses.  
2. **Sibling tools own domain truth** — Minerva shells their `--json` APIs.  
3. **Presence ≠ readiness** — especially codemap/vecgrep.  
4. **Suggest proposes; host applies** — except allowlisted activate.  
5. **No secret values** in analytics, skills, or stashes.  
6. **Dogfood** with glyph (CLI) and cairn (site).

## Related stack

| Tool | Relationship |
|------|----------------|
| local-agent | Primary runtime consumer of `~/.agents` |
| MCPHub | Gateway + call intelligence |
| Cortex | Task kernel (overview/stale signals only) |
| Bob | Repo contract |
| Codemap / Vecgrep | Retrieval readiness |
| fcheap | Evidence vault |
| glyph / cairn | Contract testing |
