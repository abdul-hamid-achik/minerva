# Proposals

`minerva propose` (and `minerva_propose`) turns [signals](/guide/concepts)
into a ranked list. The last run is stored at
`~/.agents/.minerva/proposals.json`.

| Kind | Meaning | Apply? |
|------|---------|--------|
| `new_skill` | Draft a SKILL.md from a repeated pattern | yes |
| `update_skill` | Merge an observed pattern into an existing skill's `## Observed patterns` section | yes, idempotent |
| `load_gap` | Catalog skill matched the prompt but was not loaded | no — load it in the harness |

Each proposal has a stable `id` (`kind` + hash of the skill name).

Proposals come from `shell_family`, `repeat_sequence`, `retry_loop`, and
`load_gap` signals. Names are derived from the anchor: `git-workflow`,
`go-strreplace-loop`, `bash-retry`. `user_correction` and `long_manual` show
up in `analyze` only.

## Applying an update

```markdown
## Observed patterns

- repeated shell family git (11 sessions; last cursor 6d2fd597)
- retry after error: Bash (2 sessions; last codex 01a0466f)
```

Minerva owns that section and nothing else. A legacy `## Observed later`
paragraph from earlier releases is folded into it on the next apply.

```bash
minerva propose --since 30d --json
minerva propose apply new_skill-ab12cd34
```

Drafts include frontmatter (`name`, `description` starting with “Use when”),
the observed pattern, and up to five session evidence rows.

Minerva does not call an LLM to write the body. The host agent can rewrite
the draft after apply if you want richer prose.
