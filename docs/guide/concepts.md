# Concepts

## Product thesis

> Minerva reads agent-harness conversations and tool calls, proposes skills
> from those traces, and keeps `SKILL.md` libraries in sync. It is not a
> second agent runtime.

## Canonical vs harness-local

### Canonical (Minerva writes)

| Path | Role |
|------|------|
| `~/.agents/skills/*/SKILL.md` | Skill bodies + Agent Skills frontmatter |
| `~/.agents/.skill-lock.json` | Optional install lock (GitHub source + hash) |
| `~/.agents/.minerva/proposals.json` | Last `propose` run (Minerva-local) |

### Harness-local (Minerva reads, sometimes links)

| Path | Role |
|------|------|
| `~/.claude/skills` | Claude Code skills (symlink-safe) |
| `~/.codex/skills` | Codex skills (symlink-safe) |
| `~/.cursor/skills-cursor` | Cursor-owned — never overwritten |
| `~/.claude/projects/**/*.jsonl` | Claude transcripts |
| `~/.codex/sessions/**/*.jsonl` | Codex rollouts |
| `~/.cursor/projects/**/agent-transcripts/**/*.jsonl` | Cursor transcripts |

## Trace model

Every adapter maps a harness file onto:

```text
Session { harness, id, workspace, turns[] }
Turn    { role, text, tool_calls[], skills_invoked[] }
ToolCall { name, args, result, is_error, category }
```

Categories: `shell`, `read`, `edit`, `search`, `mcp`, `subagent`, `skill`, `other`.

## Signals

Deterministic — no LLM required:

| Signal | Meaning |
|--------|---------|
| `repeat_sequence` | The same 3-tool n-gram appears more than once |
| `retry_loop` | Same tool after an error |
| `user_correction` | User asked to redo / “no, the other way” |
| `load_gap` | Catalog skill matched the prompt but was not invoked |
| `long_manual` | Many tool calls, zero skills loaded |
| `shell_family` | Repeated `git` / `go` / `npm` / … |

## Propose, then apply

`minerva propose` ranks drafts and saves them. `minerva propose apply <id>`
(or MCP `minerva_apply`) writes a `SKILL.md`. Load-gap proposals are not
applyable — they tell the harness to load an existing skill.

## Privacy

Transcripts stay on disk. MCP handlers redact secret-like strings before
returning text. Minerva does not upload sessions.
