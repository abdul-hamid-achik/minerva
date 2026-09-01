# Architecture

```text
cmd/minerva
internal/
  cli/            Cobra commands (one file per surface)
  mcp/            MCP stdio server
  skill/          SKILL.md discovery, CRUD, resolve
  harness/        Runtime catalog and path map
  session/        Transcript adapters → Trace
  signal/         Deterministic extractors
  propose/        Signals → drafts + apply
  sync/           Link/copy + .skill-lock.json
  library/        Skill lint
  secret/         Secret detection / redact
  surface/        Compact MCP contract
  learn/          One-page brief
  textdiff/       Unified diffs
  version/
docs/             VitePress site
specs/            Glyphrun CLI contracts
```

## Design rules

1. **Disk is the SSOT** for skill bodies (`~/.agents/skills`).
2. **Adapters are tolerant** — unknown JSONL line types are skipped.
3. **Signals are deterministic** — no API key required for analyze/propose.
4. **Propose then apply** — drafts persist in `.minerva/proposals.json`.
5. **Do not overwrite harness-owned trees** (Cursor `skills-cursor`).
6. **Redact before MCP** — secret-like strings never leave the host raw.
7. **Dogfood** with glyph (CLI) and cairn (site).

## Related tools

Minerva no longer shells sibling CLIs for readiness. Cortex, MCPHub,
codemap, and vecgrep remain useful *around* Minerva; they are not part of
this binary.
