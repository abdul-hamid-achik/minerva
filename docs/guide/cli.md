# CLI reference

```bash
minerva [command]
```

## Skills

```bash
minerva skill list
minerva skill show <name>
minerva skill compare <a> <b>
minerva skill create <name> <content> [-d description]
minerva skill activate <name>      # Minerva-local only
minerva skill deactivate <name>
minerva skill delete <name>
```

## Profiles

```bash
minerva profile list
minerva profile show <name>
minerva profile compare <a> <b>
minerva profile create <name> [prompt] [-d desc] [-m model] [-s skill]...
minerva profile update-prompt <name> <prompt>
minerva profile update-skills <name> skill1,skill2   # replaces list
minerva profile delete <name>
```

## Stack

```bash
minerva stack check
minerva stack check --json
minerva stack deep [workspace]
minerva stack deep --json
minerva stack deep --stash
```

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
minerva suggest --apply
minerva analytics
minerva analytics --json
```

## Evidence

```bash
minerva evidence docs
minerva evidence save <path> --kind eval --outcome pass \
  --tag skill:qa-tester --tag profile:code-reviewer
minerva evidence search minerva-eval
```

## Templates

```bash
minerva template list
minerva template show <name>
minerva template apply <name> [-p profile]
```

## MCP

```bash
minerva mcp serve
```
