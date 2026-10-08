# Changelog

All notable changes to Minerva are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Hermes Agent harness (`hermes`): skills synced by symlink into
  `~/.hermes/skills`, sessions read from `~/.hermes/sessions/*.jsonl`
- oh-my-pi harness (`omp`): modelled as a native reader of
  `~/.agents/skills` (never synced into); sessions read from
  `~/.omp/agent/sessions/*/*.jsonl`
- `harness.Harness.Native` and `PresenceDirs`: native readers are present only
  when their own data exists, not because `~/.agents/skills` does

### Changed

- `sonar` is now present only when `~/.sonar` (or its sessions) exists, no
  longer whenever `~/.agents/skills` exists
- `harness doctor` and `skill sync` ignore harness-owned entries in a skills
  dir: dot-entries, `_shared`, and real directories without a `SKILL.md`
  (category folders). They no longer show up as `extra`
- `retry_loop` needs the same command (for shell tools), not just the same
  tool, and one session can add at most `PerSessionCap` to it

### Fixed

- Skill names must be a single path element (no `.`, `..`, separators or
  leading dot). `skill create`/`delete`, `skill sync` and `skill install` can
  no longer write, link or delete outside the library or harness dir
- `skill create` no longer overwrites a folder whose `SKILL.md` declares a
  different name; `skill delete` removes the folder the skill was loaded from
- `skill sync` skips flat `<name>.md` skills instead of linking the whole
  library; copies refuse symlinks; `skill install` rejects `.`/`..` in the spec
- Tool results are matched to their call by id: Claude/Cursor `tool_use_id`,
  Codex `call_id`, Copilot `toolCallId`. Failed calls are now detected for
  Claude (`is_error`) and Codex (`Script failed`, exit codes), so
  `retry_loop` fires on real transcripts, not only on fixtures
- Copilot and omp no longer count one tool call twice
- Skill loads are detected from reads of `skill://<name>` (omp) and
  `…/skills/<name>/SKILL.md`; plugin skills (`plugin:skill`) record the skill,
  not the plugin; Hermes `skill_manage` no longer counts as a load
- Long tool arguments stay valid JSON, so long shell commands keep their
  command

## [1.1.0] - 2026-09-01

Signal quality pass: fewer, better proposals from real sessions.

### Added

- Gemini CLI session parser (`~/.gemini/tmp/*/chats/*.jsonl`): append-only
  JSONL with revisions and `$set`/`$push` snapshots; legacy `.json` read too
- `ToolCall.command`: leading executable of shell calls (`git`, `go`, `bun`);
  skips `cd … &&`, env prefixes, `sudo`/`env`/`time`, loop keywords, comments
- `session.CleanPrompt`: strips harness-injected context (`<user_info>`,
  `<git_status>`, `# AGENTS.md`, `<environment_context>`) and honours
  `<user_query>` wrappers; prompt capped at 1500 bytes
- `ResolveHit.terms`: distinct query terms that matched
- `propose.MergeObserved`: idempotent `## Observed patterns` section
- `skill sync --force` and `skill install --force`
- `harness doctor` reports `broken-link` and symlinks pointing outside
  `~/.agents/skills`
- MCP server tests over in-memory transports (tool contract, redaction on
  the wire, apply gating); `mcp.Server.Connect` for embedding
- Secret families: Stripe (`sk_live_`, `whsec_`), GitHub `github_pat_`,
  AWS `AKIA`, Google `AIza`, Vercel, npm, JWT, `Authorization: Bearer`,
  `user:pass@host` URLs, private key blocks, `export SOME_KEY=…` lines

### Fixed

- `skill sync --dry-run` created harness skills directories
- `skill sync` deleted any directory at the destination, including a
  harness-local skill the user wrote. Diverged directories are now skipped
  (exit 1) unless `--force`; symlinks and identical copies are still replaced
- `skill install owner/repo` silently picked the first `SKILL.md` in a
  multi-skill repo; it now fails and lists candidates. Re-install keeps
  `installedAt`
- `secret.Redact` covered fewer families than `secret.Hits`; they now share
  one table
- `propose apply` on a missing store leaked the raw file path in the error

### Changed

- `repeat_sequence` needs an anchor (named shell command, MCP tool, subagent,
  or skill) and 2+ sessions or 3+ occurrences; read/edit-only n-grams are out
- Shell calls in n-grams carry their command: `Shell(go) → Read → StrReplace`
- Per-session contribution to repeat/shell-family weight capped at 5;
  cross-session breadth adds +2 per extra session
- `user_correction` matches whole phrases on word boundaries, only after the
  assistant acted; leading `no`/`wrong` checks the next word (`no problem` is
  not a correction). Evidence notes the matched phrase
- `load_gap` requires 2 distinct matched terms (or an exact name hit) and at
  most 2 gaps per session
- `skill resolve`: no score without a matched term (the "Use when" bonus no
  longer creates hits by itself); whole-word matching with light stemming;
  catalog-wide common terms dropped; larger EN/ES stop list; long
  descriptions score 2 per term instead of 3
- `update_skill` apply merges into `## Observed patterns` instead of
  appending `## Observed later`; legacy sections are migrated
- `update_skill` action is `minerva propose apply <id>`
- `user_correction` and `long_manual` are analyze-only; propose skips them
- Tool categorisation normalises `-`/`_`; `server__tool` names are `mcp`
- `--since` parsing lives in one place (`session.ParseSince`)

### Removed

- `propose.KindMerge` (unused), `skill.NewManagerWithState` (compat shim)

## [1.0.0] - 2026-09-01

BREAKING pivot: Minerva is no longer a stack-readiness operator. It reads
harness conversations, proposes skills, and syncs `SKILL.md` libraries.

### Added

- Harness catalog (`claude`, `codex`, `cursor`, `opencode`, `copilot`, `gemini`, `sonar`)
- Session parsers for Claude Code, Codex, Cursor, OpenCode, and Copilot (Gemini protobuf is listed only)
- `minerva sessions`, `analyze`, `propose`, `propose apply`
- `minerva harness list|doctor`
- `minerva skill lint|install|sync`
- MCP tools: `minerva_sessions`, `minerva_analyze`, `minerva_propose`, `minerva_harness`, `minerva_apply`

### Removed

- Stack presence/deep probes (`stack check`, `stack deep`, `status`/`doctor`)
- Profiles, templates, library export/import, bridge snippets
- Evidence/fcheap helpers and Minerva-local analytics
- `skill activate` / `.minerva-skills.json`
- MCP tools `minerva_status`, `minerva_suggest`, `minerva_stack_check`, `minerva_evidence`, `minerva_profile`, `minerva_library`

### Changed

- `minerva learn` thesis is session → skill intelligence
- Compact MCP surface is 8 tools; only `minerva_apply` mutates disk
- Init creates `skills/` and `.minerva/` only

## [0.2.0] - 2026-07-19

Operator control plane release (superseded by the unreleased pivot).

## [0.1.0] - 2026-07-16

Initial public release.

[1.1.0]: https://github.com/abdul-hamid-achik/minerva/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/abdul-hamid-achik/minerva/releases/tag/v0.1.0
