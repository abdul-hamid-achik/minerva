# CLI reference

```bash
minerva [command]
```

## Skills

```bash
minerva skill list
minerva skill show <name>
minerva skill compare <a> <b>          # unified diff; --side-by-side for full bodies
minerva skill create <name> [content] [-d description] [--from-file path]
minerva skill update <name> [-d description] [--content body|--from-file path]
minerva skill activate <name>      # Minerva-local catalog pin only
minerva skill deactivate <name>
minerva skill delete <name>
```

Prefer `profile add-skills` for durable local-agent loading. `activate` only updates `~/.agents/.minerva-skills.json`.

## Profiles

```bash
minerva profile list
minerva profile show <name>
minerva profile compare <a> <b>        # unified YAML diff; --side-by-side for summary
minerva profile create <name> [prompt] [-d desc] [-m model] [-s skill]...
minerva profile update-prompt <name> <prompt>
minerva profile update-skills <name> skill1,skill2   # replaces list
minerva profile add-skills <name> skill1,skill2      # merge
minerva profile remove-skills <name> skill1,skill2
minerva profile update-model <name> <model>
minerva profile update-mcp <name> server1,server2
minerva profile update-desc <name> <description>
minerva profile delete <name>
```

Broken `agent.yaml` files surface as warnings on `profile list` / `suggest` instead of disappearing silently.

## Status (doctor)

```bash
minerva status                  # alias: minerva doctor
minerva status --json
minerva status --deep=false     # presence + library only
minerva status --require-retrieval
minerva status --watch --interval 30s
```

Unified view: library inventory, stack presence, deep readiness, open evidence fails, top next actions.

| Code | When (`status`) |
|------|------|
| 0 | healthy |
| 1 | unhealthy (core incomplete) |
| 2 | degraded |
| 3 | retrieval not ready (`--require-retrieval`) |

## Stack

```bash
minerva stack check
minerva stack check --json
minerva stack check --strict          # exit 2 if optional tools missing
minerva stack deep [workspace]
minerva stack deep --json
minerva stack deep --stash
minerva stack deep --require-retrieval   # exit 3 if retrieval not ready
minerva stack deep --require-core        # exit 1 if core presence incomplete
```

### Exit codes (gates)

| Code | When |
|------|------|
| 0 | OK |
| 1 | Core presence incomplete (`stack check`, or `deep --require-core`) |
| 2 | Degraded optional stack (`stack check --strict`) |
| 3 | Retrieval not ready (`stack deep --require-retrieval`) |

Binary map (product → PATH):

| Product | Binary | Tier |
|---------|--------|------|
| bob | `bob` | core |
| cortex | `cortex` | core |
| mcphub | `mcphub` | core |
| codemap | `codemap` | core |
| vecgrep | `vecgrep` | core |
| fcheap | `fcheap` | core |
| monitor | `monitor` | optional |
| hitspec | `hitspec` | optional |
| glyphrun | **`glyph`** | optional |
| cairntrace | **`cairn`** | optional |
| vidtrace | `vidtrace` | optional |
| tinyvault | **`tvault`** | optional |
| veclite | `veclite` | infra |

## Suggest & analytics

```bash
minerva suggest
minerva suggest --json
minerva suggest --apply              # allowlisted profile add-skills only
minerva suggest --apply-local       # also Minerva-local skill activate
minerva analytics
minerva analytics --json
```

`--apply` mutates the shared profile SSOT. It does not auto-activate Minerva-local pins unless you pass `--apply-local`.

## Evidence

```bash
minerva evidence docs
minerva evidence save <path> --kind eval --outcome pass \
  --tag skill:qa-tester --tag profile:code-reviewer
minerva evidence search minerva-eval
minerva evidence close <stash-id> [--note "fixed"]   # close receipt loop
```

## Templates

Builtin seeds plus disk overrides under `~/.agents/templates/<name>/template.yaml`.

```bash
minerva template list
minerva template show <name>
minerva template apply <name> [-p profile]
minerva template install <name>     # copy builtin → disk for editing
minerva template save <name> --prompt "..." [-s skill]...
```

## Library

```bash
minerva library export ./bundle.tgz
minerva library export ./bundle-dir
minerva library import ./bundle.tgz [--force]
minerva library lint               # exit 1 on errors
minerva library lint --json
```

## Bridge (local-agent)

```bash
minerva bridge show <profile>                 # markdown docs
minerva bridge show <profile> -f shell -o run.sh
minerva bridge show <profile> -f yaml
```

## MCP

```bash
minerva mcp serve
```
