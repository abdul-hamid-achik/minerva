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
minerva sessions --workspace ~/projects/app
minerva sessions --json
```

Shared filters for `sessions`, `analyze` and `propose`:

| Flag | Meaning |
|------|---------|
| `--harness id` | One harness (`claude`, `codex`, `cursor`, …); an unknown id is an error |
| `--since` | Go duration (`24h`) or day count (`7d`); negative or malformed values are errors |
| `--workspace path` | Sessions run in that directory (a path, or its last segment). Applied before `--limit` |
| `--limit n` | Newest `n` sessions after the other filters |
| `--session id` | `analyze` only: id prefix, or a Codex rollout uuid prefix |

## Analyze / propose

```bash
minerva analyze --last --json
minerva analyze --harness codex --since 7d
minerva analyze --workspace ~/projects/app --limit 10
minerva propose --since 30d
minerva propose --json
minerva propose apply <proposal-id>
```

## Skills

```bash
minerva skill list
minerva skill show <name>
minerva skill compare <a> <b> [--side-by-side]
minerva skill create <name> [content] [-d description] [--from-file path]
minerva skill update <name> [-d description] [--content body|--from-file path]
minerva skill delete <name>
minerva skill resolve "<intent>" [--json]
minerva skill lint [--json]
minerva skill install owner/repo[/path] [--force]
minerva skill sync [--to claude,codex] [--dry-run] [--force] [--json]
```

`skill lint` exits `1` when errors (secrets, invalid bodies) are present.

### Sync safety

`skill sync` without `--to` syncs every installed harness (its root directory,
such as `~/.claude`, exists) and creates nothing for the others. `--to` with an
unknown harness, or one Minerva never writes (`cursor`, `omp`, `sonar`), is an
error.

Symlinks into `~/.agents/skills` and byte-identical copies are replaced
freely. A directory that Minerva did not write and whose contents differ, or a
symlink pointing somewhere else, is your or the harness's own work: it is
reported as `skipped` with a reason and the command exits `1`. Pass `--force`
to replace it. A folder without `SKILL.md` (a harness category) is never
replaced. Flat `<name>.md` skills are skipped; move them to `<name>/SKILL.md`.
`--dry-run` writes nothing, not even the harness skills directory.

`skill delete` also removes the harness symlinks that pointed at the skill.

`skill install` fails when the repository holds several `SKILL.md` files and
no path narrows the choice; the error lists them. It also refuses to replace a
local skill that `.skill-lock.json` does not track unless `--force`.

`harness doctor` distinguishes `missing`, `extra`, `drift` (different
contents, or a symlink pointing outside `~/.agents/skills`), `broken-link`
(including links to skills deleted outside Minerva) and `error` (a harness
copy it cannot read).

## MCP

```bash
minerva mcp serve
```
