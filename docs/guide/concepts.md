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
| `~/.gemini/tmp/*/chats/*.jsonl` | Gemini CLI chats |
| `~/.hermes/skills` | Hermes Agent skills (symlinked; its dot-entries and `_shared` are left alone) |
| `~/.hermes/state.db`, `~/.hermes/sessions/*.jsonl` | Hermes Agent sessions (SQLite store, older JSONL logs) |
| `~/.omp/agent/sessions/*/*.jsonl` | oh-my-pi (omp) transcripts; omp reads `~/.agents/skills` natively |

## Trace model

Every adapter maps a harness file onto:

```text
Session { harness, id, workspace, turns[] }
Turn    { role, text, tool_calls[], skills_invoked[] }
ToolCall { name, args, result, is_error, category, command }
```

Categories: `shell`, `read`, `edit`, `search`, `mcp`, `subagent`, `skill`, `other`.

`command` is the leading executable of a shell call (`git`, `go`, `npm`), extracted
from the args after skipping `cd … &&`, `FOO=bar`, `sudo`/`env`/`time`, loop
keywords, and comments. Pure `ls`/`cat`/`echo` pipelines yield no command.

### The first prompt

Harnesses prepend injected context as user turns (Cursor `<user_info>`,
`<git_status>`, Codex `# AGENTS.md instructions`, `<environment_context>`).
Minerva strips those and, when a `<user_query>` wrapper exists, uses only its
body. The result is capped at 1500 bytes before it reaches the resolver.

## Signals

Deterministic — no LLM required:

| Signal | Meaning | Threshold |
|--------|---------|-----------|
| `repeat_sequence` | A 3-tool n-gram with an anchor (a named shell command, MCP tool, subagent, or skill). Pure read/search or read/edit sequences are how coding works, not a skill | 2+ sessions, or 3+ times in one |
| `retry_loop` | Same tool immediately after it errored | 1 |
| `user_correction` | A user turn, after the assistant acted, that opens with `no`/`wrong` or contains a redo phrase (`do it again`, `otra vez`, `the wrong file`, …) on word boundaries | 1 |
| `load_gap` | A catalog skill matched the first prompt on 2+ distinct terms (or its exact name) but was never invoked. At most 2 per session | resolve score ≥ 6 |
| `long_manual` | 8+ tool calls, zero skills loaded | 1 |
| `shell_family` | The same executable 3+ times in a session (`git`, `go`, `bun`, …) | 3 |

Shell calls in n-grams carry their command: `Shell(go) → Read → StrReplace`
(test-fix loop) is a different pattern from `Shell(git) → Read → StrReplace`.

### Ranking

Each session contributes at most 5 to a repeat or shell-family signal, so one
marathon session cannot outrank a pattern seen across many. Every extra
session adds +2. Kind bonuses: load gap +8, correction +6, retry +4, shell
family +3, long manual +2.

## Propose, then apply

`minerva propose` ranks drafts and saves them. `minerva propose apply <id>`
(or MCP `minerva_apply`) writes a `SKILL.md`. Load-gap proposals are not
applyable — they tell the harness to load an existing skill.

`user_correction` and `long_manual` are diagnostic: they say a skill was
missing, not which one. `analyze` reports them; `propose` does not draft from
them.

Applying an `update_skill` proposal merges one bullet into a Minerva-owned
`## Observed patterns` section. Re-applying refreshes that bullet's session
count instead of appending. Everything else in the body is left byte-for-byte.

## Privacy

Transcripts stay on disk. MCP handlers redact secret-like strings before
returning text. Minerva does not upload sessions.

Detection and redaction share one pattern table, so anything `skill lint`
can flag is also stripped on the wire: key/token/password assignments,
`Authorization: Bearer …`, private key blocks, OpenAI/Anthropic `sk-`,
Stripe `sk_live_`/`whsec_`, GitHub `ghp_`/`github_pat_`, Slack `xox…`, AWS
`AKIA…`, Google `AIza…`, Vercel/npm tokens, JWTs, `user:pass@host` URLs, and
`export SOME_KEY=…` lines. Prefixes stay; the secret material becomes
`[redacted]`.
