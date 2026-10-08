# Minerva

[![Release](https://img.shields.io/github/v/release/abdul-hamid-achik/minerva)](https://github.com/abdul-hamid-achik/minerva/releases)
[![Changelog](https://img.shields.io/badge/changelog-1.2.0-blue)](./CHANGELOG.md)

**Skill intelligence** for agent harnesses. Minerva reads conversations and
tool calls from Claude Code, Codex, Cursor, OpenCode, Copilot, Gemini,
Hermes Agent, oh-my-pi (omp), and sonar — then proposes `SKILL.md` files and keeps the canonical
`~/.agents/skills` library in sync.

It is **not** a second agent runtime, not Cortex, and not a monitor for
companion CLIs.

Built with Go + Cobra + MCP stdio.

## What Minerva does

| Surface | Role |
|---|---|
| **Sessions** | Discover and parse harness transcripts (JSONL / JSON) |
| **Analyze** | Deterministic signals: retries, corrections, load-gaps, tool n-grams |
| **Propose** | Ranked `new_skill` / `update_skill` / `load_gap` drafts with evidence |
| **Skills** | Create/list/show/compare/delete/lint under `~/.agents/skills/` |
| **Sync** | Link or copy skills into writable harness dirs; doctor drift |
| **MCP** | Compact stdio surface for harnesses / MCPHub |

## Quick start

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)

### Install / build

```bash
# Homebrew (recommended for macOS and Linux)
brew install --cask abdul-hamid-achik/tap/minerva

# From source
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
  skills/       # skill definitions (SKILL.md)
  .minerva/     # proposal store and local cache
```

Override the library root with `MINERVA_AGENTS_DIR`. Override the home used
to find harness session files with `MINERVA_HOME` (tests should set both).

## CLI

```bash
minerva learn
minerva harness list
minerva harness doctor
minerva sessions --since 7d --harness claude
minerva analyze --last --json
minerva propose --since 30d
minerva propose apply <proposal-id>

minerva skill list
minerva skill show <name>
minerva skill resolve "review this PR"
minerva skill create <name> [content] [-d description]
minerva skill update <name> [--content body|--from-file path]
minerva skill lint
minerva skill install owner/repo[/path]
minerva skill sync --to claude,codex --dry-run
```

## MCP

```bash
minerva mcp serve
```

Eight tools: `minerva_learn`, `minerva_sessions`, `minerva_analyze`,
`minerva_propose`, `minerva_resolve_skill`, `minerva_skill`,
`minerva_harness`, `minerva_apply`. Only `minerva_apply` writes skills;
`minerva_propose` saves its drafts to `~/.agents/.minerva/proposals.json`.

Wire via MCPHub:

```yaml
servers:
  minerva:
    command: minerva
    args: [mcp, serve]
    enabled: true
```

## Architecture

```text
cmd/minerva/
internal/
  cli/           Cobra commands
  mcp/           MCP stdio server
  skill/         SKILL.md discovery + CRUD + resolve
  harness/       Runtime catalog (paths, capabilities)
  session/       Transcript adapters → Trace
  signal/        Deterministic extractors
  propose/       Signals → SKILL.md drafts
  sync/          Cross-harness link/copy + lockfile
  library/       Skill lint
  surface/       Compact MCP contract
```

Canonical skills live in `~/.agents/skills`. Harness copies/symlinks are
derived. Cursor's `~/.cursor/skills-cursor` is treated as harness-owned and
is never overwritten. Hermes Agent's `~/.hermes/skills` is synced by symlink;
Minerva leaves its dot-entries (`.archive`, `.curator_*`, …) and `_shared`
alone. oh-my-pi (omp) and sonar read `~/.agents/skills` directly, so Minerva
only reads their sessions and never syncs into them.

## Development

```bash
task build
task test
task lint
task glyph-fast
task docs:dev      # VitePress site → http://127.0.0.1:5173
```

## License

MIT
