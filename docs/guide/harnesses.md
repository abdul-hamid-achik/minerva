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
| `sonar` | `~/.agents/skills` | `~/.sonar/logs`, `sonar.db` | skip |

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
- **Antigravity / sonar** — listed when files exist; Antigravity conversations
  are protobuf and are not decoded.

Unknown line types are ignored. A single corrupt line never fails the file.

## Tool categories

Names are normalised (case, `-`, `_`) before matching. Gateway-namespaced tools
(`server__tool`, `mcp__…`) are always `mcp`. Shell tools also get a `command`:
the leading executable after `cd … &&`, env prefixes, and wrappers.
