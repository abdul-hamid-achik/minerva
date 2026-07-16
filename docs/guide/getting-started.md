# Getting started

Minerva is a **Go CLI + MCP server** that manages the shared agent library under `~/.agents` and orchestrates stack readiness for your intelligence tools.

## Install

```bash
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
