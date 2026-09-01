# Proposals

`minerva propose` (and `minerva_propose`) turns [signals](/guide/concepts)
into a ranked list. The last run is stored at
`~/.agents/.minerva/proposals.json`.

| Kind | Meaning | Apply? |
|------|---------|--------|
| `new_skill` | Draft a SKILL.md from a repeated pattern | yes |
| `update_skill` | Append an observed pattern to an existing skill | yes |
| `load_gap` | Catalog skill matched the prompt but was not loaded | no — load it in the harness |

Each proposal has a stable `id` (`kind` + hash of the skill name).

```bash
minerva propose --since 30d --json
minerva propose apply new_skill-ab12cd34
```

Drafts include frontmatter (`name`, `description` starting with “Use when”),
the observed pattern, and up to five session evidence rows.

Minerva does not call an LLM to write the body. The host agent can rewrite
the draft after apply if you want richer prose.
