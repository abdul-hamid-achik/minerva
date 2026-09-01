# Changelog

All notable changes to Minerva are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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

[1.0.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/abdul-hamid-achik/minerva/releases/tag/v0.1.0
