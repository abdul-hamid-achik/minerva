# Changelog

All notable changes to Minerva are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.2.1] - 2026-10-08

### Added

- Sessions report `skipped_lines`: transcript lines that could not be read
  (longer than 8 MB, or not JSON), so a partly parsed session is visible in
  `--json` output

### Fixed

- OpenCode session and message ids read from storage files can no longer
  point outside the storage directory (an id such as `../..` made Minerva read
  JSON files elsewhere on disk); such ids fall back to the file name
- Sessions with the same modification time are ordered by harness, id and
  path, so `--limit` returns the same sessions on every run

## [1.2.0] - 2026-10-08

Correctness pass from a full code review: tool results matched to their call
by id (retry signals now fire on real Claude and Codex transcripts), Hermes
sessions read from `state.db`, safe skill names, and a sync that protects the
user's own links and folders.

### Added

- Hermes Agent harness (`hermes`): skills synced by symlink into
  `~/.hermes/skills`, sessions read from `~/.hermes/state.db` (SQLite, read-only)
  and older `~/.hermes/sessions/*.jsonl` logs
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
- `proposals.json` keeps the latest 100 proposals across runs (newest first)
  instead of only the last run, so an id printed earlier still applies after
  a narrower or empty `propose`; it is replaced atomically
- MCP: `minerva_propose` is no longer annotated read-only (it saves drafts);
  only `minerva_apply` is marked destructive

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
- One duplicate or unparsable `SKILL.md` no longer breaks the whole library:
  it is skipped with a warning (and reported by `skill lint`). `mcp serve`
  starts anyway, and `analyze`/`propose` no longer run with a silently empty
  catalog
- `skill update` and `propose apply` keep every frontmatter key
  (`allowed-tools`, `license`, `metadata`, comments), not only `name` and
  `description`; SKILL.md files are written atomically
- A transcript line over 8 MB (an inlined image, a huge tool result) is
  skipped instead of silently dropping the whole session
- `--workspace` is applied before `--limit`, so `analyze --workspace X
  --limit 5` returns X's five newest sessions instead of filtering the five
  newest overall; `sessions --workspace` (and MCP `minerva_sessions`) now
  filter. Sessions whose location already shows another workspace are not
  parsed, so a workspace scan stays fast
- Cursor sessions match `--workspace /path` (their project slug is compared
  to the path); the slug is the directory above `agent-transcripts`
- `--session` matches a prefix of the id or of an id in the file name (a
  Codex rollout uuid), no longer any directory name containing it
- `--since` rejects negative durations and malformed day counts (`1.5d`,
  `7xd`) instead of silently widening the window
- Codex sessions report the model from `turn_context`, not the provider
- Secret redaction covers JSON-quoted and compound keys (`"password": "…"`,
  `"aws_secret_access_key"`, `"Authorization": "Bearer …"`), private keys cut
  off before their END line, tokens glued to `_` (`mcp__srv_ghp_…`), Hugging
  Face and DigitalOcean tokens, and tool names. MCP `minerva_skill`
  show/compare and `resolve` no longer return secrets verbatim
- `skill resolve` keeps the words of a skill name the query spells out, so
  `docker-workflow` is found in a large catalog
- `update_skill` proposals target a skill only when its whole name appears
  in the signal key; skills named `tool`, `go` or `retry` no longer collect
  unrelated patterns
- `skill sync` without `--to` touches only installed harnesses; `--to`
  rejects unknown and read-only harnesses (`cursor`, `omp`, `sonar`). A
  symlink pointing outside `~/.agents/skills` is kept unless `--force`; a
  harness folder without `SKILL.md` is never replaced
- `skill install` swaps a fully copied staging dir in (a failed copy keeps
  the installed skill) and does not copy `.git`; `.skill-lock.json` is
  written atomically
- `skill delete` removes the harness symlinks to the skill; `harness doctor`
  reports dangling links as `broken-link` and unreadable copies as `error`
- Corrections are searched in what the user typed (injected reminders and
  attachments stripped, first 400 bytes); a bare "no" answering the
  assistant's question is not a correction. Signal order is deterministic
- `sessions`/`propose` print `[]`, not `null`, when empty; `started_at` is
  omitted when unknown; Copilot dirs without `events.jsonl` and sonar logs
  (no parser yet) are no longer listed as empty sessions
- A UTF-8 BOM no longer hides a SKILL.md's frontmatter; `skill create`
  enforces the same size limits as `update`; `skill compare` on huge files
  falls back to a whole-file diff instead of allocating gigabytes

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

[1.2.1]: https://github.com/abdul-hamid-achik/minerva/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/abdul-hamid-achik/minerva/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/abdul-hamid-achik/minerva/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/abdul-hamid-achik/minerva/releases/tag/v0.1.0
