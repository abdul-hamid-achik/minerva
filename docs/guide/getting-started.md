# Getting started

Minerva is a **Go CLI + MCP server** that manages the shared agent library under `~/.agents` and orchestrates stack readiness for your intelligence tools.

::: info What you will have in five minutes
A shared agent library, a tiered inventory of your local stack, and an honest deep-readiness report with concrete next actions.
:::

## Choose your path

| I want to… | Start with |
|---|---|
| Organize skills and agent profiles | `minerva init`, then `minerva skill list` |
| One honest operator dashboard | `minerva status` (alias: `doctor`) |
| Audit installed intelligence tools | `minerva stack check` |
| Verify retrieval and operator readiness | `minerva stack deep` / `status --require-retrieval` |
| Portable library for a team machine | `minerva library export` / `import` / `lint` |
| Wire a profile into local-agent | `minerva bridge show <profile>` |
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
  agents/     # profiles (agent.yaml)
  skills/     # SKILL.md definitions
  templates/  # disk role templates
  tasks/
  memories/
```

Override for tests or sandboxes:

```bash
export MINERVA_AGENTS_DIR=/tmp/minerva-agents
minerva init
```

## First useful commands

```bash
minerva skill list
minerva profile list
minerva status                # library + presence + deep + next actions
minerva stack check          # presence only, tiered
minerva stack deep           # readiness + cortex + mcphub
minerva stack deep --stash   # save report to fcheap
minerva suggest              # ranked next actions (prefer profile membership)
minerva library lint
minerva bridge show <profile>
```

`status` is the default operator loop. The fast stack check answers “what is present?” The deep check asks each owning tool whether its domain is actually ready. Read [Stack readiness](/guide/stack) and [CLI](/guide/cli) for exit-code gates (`--require-retrieval`, `--strict`).

## Wire MCP (via MCPHub)

```yaml
# ~/.config/mcphub/mcphub.yaml
servers:
  minerva:
    command: minerva   # or absolute path to bin/minerva
    args: [mcp, serve]
    enabled: true
```

In local-agent, trust **exact** tool names (no wildcards). Prefer read-only tools in AUTO; gate create/delete/update.

## Next

- [Concepts](/guide/concepts) — what is shared vs Minerva-local  
- [CLI](/guide/cli) — full command surface  
- [Stack readiness](/guide/stack) — retrieval green light  
