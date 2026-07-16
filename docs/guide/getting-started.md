# Getting started

Minerva is a **Go CLI + MCP server** that manages the shared agent library under `~/.agents` and orchestrates stack readiness for your intelligence tools.

::: info What you will have in five minutes
A shared agent library, a tiered inventory of your local stack, and an honest deep-readiness report with concrete next actions.
:::

## Choose your path

| I want to… | Start with |
|---|---|
| Organize skills and agent profiles | `minerva init`, then `minerva skill list` |
| Audit installed intelligence tools | `minerva stack check` |
| Verify retrieval and operator readiness | `minerva stack deep` |
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
minerva stack check          # presence, correct binaries, tiers
minerva stack deep           # readiness + cortex + mcphub
minerva stack deep --stash   # save report to fcheap
minerva suggest              # ranked next actions
```

The fast check answers “what is present?” The deep check asks each owning tool whether its domain is actually ready. Read [Stack readiness](/guide/stack) before using the result as an agent gate.

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
