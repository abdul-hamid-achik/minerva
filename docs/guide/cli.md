# CLI reference

```bash
minerva [command]
```

## Learn / init

```bash
minerva init
minerva learn
minerva learn --json
```

## Harnesses

```bash
minerva harness list
minerva harness list --json
minerva harness doctor
minerva harness doctor --json
```

## Sessions

```bash
minerva sessions
minerva sessions --harness claude --since 7d --limit 20
minerva sessions --json
```

`--since` accepts Go durations (`24h`) or day counts (`7d`).

## Analyze / propose

```bash
minerva analyze --last --json
minerva analyze --harness codex --since 7d
minerva propose --since 30d
minerva propose --json
minerva propose apply <proposal-id>
```

## Skills

```bash
minerva skill list
minerva skill show <name>
minerva skill compare <a> <b>
minerva skill create <name> [content] [-d description] [--from-file path]
minerva skill update <name> [-d description] [--content body|--from-file path]
minerva skill delete <name>
minerva skill resolve "<intent>"
minerva skill lint
minerva skill install owner/repo[/path]
minerva skill sync [--to claude,codex] [--dry-run] [--json]
```

`skill lint` exits `1` when errors (secrets, invalid bodies) are present.

## MCP

```bash
minerva mcp serve
```
