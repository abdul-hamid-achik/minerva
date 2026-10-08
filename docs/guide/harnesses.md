# Harnesses

Minerva discovers sessions by globbing well-known paths under `$MINERVA_HOME`
(default `$HOME`).

| Id | Skills dir | Sessions | Sync |
|----|------------|----------|------|
| `claude` | `~/.claude/skills` | `~/.claude/projects/*/*.jsonl` | symlink |
| `codex` | `~/.codex/skills` | `~/.codex/sessions/**/*.jsonl` | symlink |
| `cursor` | `~/.cursor/skills-cursor` | `…/agent-transcripts/*/*.jsonl` | **read-only** |
| `opencode` | `~/.config/opencode/skills` | `~/.local/share/opencode/storage/session/*/*.json` | copy/link |
| `copilot` | `~/.copilot/skills` | `~/.copilot/session-state/*` | copy/link |
| `gemini` | `~/.gemini/skills` | `~/.gemini/tmp/*/chats/*.jsonl`, `~/.gemini/antigravity/conversations/*` | copy/link |
| `hermes` | `~/.hermes/skills` | `~/.hermes/sessions/*.jsonl` | symlink |
| `omp` | `~/.agents/skills` (native) | `~/.omp/agent/sessions/*/*.jsonl` | skip |
| `sonar` | `~/.agents/skills` (native) | `~/.sonar/logs`, `sonar.db` | skip |

**Native** harnesses (`omp`, `sonar`) load `~/.agents/skills` themselves, so
`skill sync` never writes to them and `harness doctor` has nothing to compare.
They count as present only when their own data exists (`~/.omp/agent`,
`~/.sonar`, or matching sessions), not merely because the shared skills tree
exists.

**Hermes Agent** follows symlinks in `~/.hermes/skills`, so synced skills are
links to the canonical tree. Its own entries in that directory (dot-entries
such as `.archive` and `.curator_backups`, `_shared`, and category folders
without a `SKILL.md`) are never treated as skills and never modified. A
Hermes-authored skill with the same name as a canonical one is kept unless you
pass `--force`.

## Formats

- **Claude Code** — JSONL with `type` + `message.content[]` (`tool_use`, `Skill`).
- **Cursor** — JSONL `{role, message.content[]}` with `tool_use`.
- **Codex** — JSONL `session_meta` / `response_item` / `event_msg` (`function_call`, `mcp_tool_call_end`).
- **OpenCode** — session JSON plus `storage/message/<id>` and `storage/part/<msg>` tool/text parts.
- **GitHub Copilot** — `session-state/<id>/events.jsonl` (`user.message`, `tool.execution_*`) and `workspace.yaml`.
- **Gemini CLI** — append-only JSONL: line 1 is `{sessionId, startTime}`, then
  `{id, type: user|gemini, content, toolCalls[]}` records, revisions of the same
  `id`, and `$set`/`$push` snapshots of `messages`. Latest revision wins.
  Legacy `.json` exports are read too. Workspace is reverse-mapped through
  `~/.gemini/projects.json`.
- **Hermes Agent** — `~/.hermes/sessions/<id>.jsonl` gateway transcripts:
  OpenAI-style `user` / `assistant` (`tool_calls[].function`) / `tool`
  messages, with an optional leading `session_meta`. Results are matched to
  calls by `tool_call_id`; a non-zero `exit_code` or an `error` marks a failed
  call. Transcripts carry no working directory. Legacy `session_*.json`
  snapshots are not read.
- **oh-my-pi (omp)** — `~/.omp/agent/sessions/<workspace>/*.jsonl`: a
  `session` entry (`id`, `cwd`) followed by `message` entries whose assistant
  content holds `toolCall` parts and whose `toolResult` messages carry
  `toolCallId` and `isError`. Thinking blocks are dropped.
- **Antigravity / sonar** — listed when files exist; Antigravity conversations
  are protobuf and are not decoded.

Unknown line types are ignored. A single corrupt line never fails the file.

## Tool categories

Names are normalised (case, `-`, `_`) before matching. Gateway-namespaced tools
(`server__tool`, `mcp__…`) are always `mcp`. Shell tools also get a `command`:
the leading executable after `cd … &&`, env prefixes, and wrappers.
