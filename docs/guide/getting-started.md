# Getting started

Minerva is a **Go CLI + MCP server** that reads agent-harness conversations,
proposes skills, and syncs `SKILL.md` libraries across harnesses.

::: info What you will have in five minutes
A canonical skill directory, a list of recent sessions, and ranked proposals
backed by tool-call evidence.
:::

## Choose your path

| I want to… | Start with |
|---|---|
| See which harnesses Minerva can read | `minerva harness list` |
| List recent conversations | `minerva sessions --since 7d` |
| Extract patterns from the last session | `minerva analyze --last` |
| Draft skills from the last month | `minerva propose --since 30d` |
| Manage the skill library | `minerva skill list` |
| Connect an agent harness | [MCP integration](/guide/mcp) |

## Install

**Prerequisite:** Go 1.25 or newer.

```bash
# Homebrew (recommended)
brew install --cask abdul-hamid-achik/tap/minerva

# from source
git clone https://github.com/abdul-hamid-achik/minerva.git
cd minerva
task build
./bin/minerva --version

# or
go install github.com/abdul-hamid-achik/minerva/cmd/minerva@latest
```

## Initialize the agents root

```bash
minerva init
```

Creates:

```text
~/.agents/
  skills/     # SKILL.md definitions
  .minerva/   # proposal store
```

Override for tests or sandboxes:

```bash
export MINERVA_AGENTS_DIR=/tmp/minerva-agents
export MINERVA_HOME=/tmp/minerva-home
minerva init
```

`MINERVA_HOME` is the tree Minerva uses to find `~/.claude`, `~/.codex`, and
other harness session files.

## First useful commands

```bash
minerva learn
minerva harness list
minerva sessions --since 7d
minerva analyze --last --json
minerva propose --since 30d
minerva skill resolve "review this PR"
```

## Wire MCP (via MCPHub)

```yaml
# ~/.config/mcphub/mcphub.yaml
servers:
  minerva:
    command: minerva
    args: [mcp, serve]
    enabled: true
```

## Next

- [Concepts](/guide/concepts) — traces, signals, canonical skills
- [Harnesses](/guide/harnesses) — paths and formats
- [CLI](/guide/cli) — full command surface
