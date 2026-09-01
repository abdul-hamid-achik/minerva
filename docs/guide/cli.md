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
minerva skill install owner/repo[/path] [--force]
minerva skill sync [--to claude,codex] [--dry-run] [--force] [--json]
```

`skill lint` exits `1` when errors (secrets, invalid bodies) are present.

### Sync safety

`skill sync` replaces symlinks and byte-identical copies freely. A directory
in a harness tree that Minerva did not write and whose contents differ is the
harness's own work: it is reported as `skipped` with a reason and the command
exits `1`. Pass `--force` to replace it. `--dry-run` writes nothing, not even
the harness skills directory.

`skill install` fails when the repository holds several `SKILL.md` files and
no path narrows the choice; the error lists them. It also refuses to replace a
local skill that `.skill-lock.json` does not track unless `--force`.

`harness doctor` distinguishes `missing`, `extra`, `drift` (different
contents, or a symlink pointing outside `~/.agents/skills`), and
`broken-link`.

## MCP

```bash
minerva mcp serve
```
