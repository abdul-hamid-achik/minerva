# Changelog

All notable changes to Minerva are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-07-19

Operator control plane release: durable library management, honest readiness gates,
evidence close loop, portable bundles, and local-agent bridge snippets.

### Added

- **Profiles:** `add-skills`, `remove-skills`, `update-model`, `update-mcp`, `update-desc`
- **Skills:** `update` with `--content` / `--from-file`; `create --from-file`
- **Status / doctor:** unified library + presence + deep + evidence + next actions
  - Exit codes: healthy / unhealthy / degraded / retrieval gate
  - `--watch --interval` continuous probe
- **Stack gates:** `stack check --strict`; `stack deep --require-retrieval` / `--require-core`
- **Evidence close loop:** `evidence close <id>` receipts (`closes:<id>`, `outcome:closed`)
- **Templates on disk:** `~/.agents/templates/` overrides builtins; `install` / `save`
- **Library:** `export` / `import` (dir or `.tar.gz`) and `lint` (refs, secrets, orphans)
- **Bridge:** `bridge show <profile>` in `md` | `shell` | `yaml` for harness wiring
- **Compare:** unified diffs for skills and profiles (`--side-by-side` for legacy dump)
- **MCP:** status, library lint/export/import, bridge, template apply, evidence search/close,
  profile add/remove skills, skill update, and related mutations
- **Glyphrun specs:** library/profile, status/bridge, skill update/compare

### Changed

- **Suggest** prefers durable `profile add-skills` over Minerva-local `skill activate`
- `--apply` only runs profile membership; `--apply-local` opts into activate
- Profile load warnings surface broken YAML instead of silent skip
- Analytics clearly labeled Minerva-local only (not harness telemetry)
- Docs: CLI, concepts, MCP, architecture, getting started, dogfood

### Fixed

- Activation honesty messaging on `skill activate` and suggest apply paths

## [0.1.0] - 2026-07-16

Initial public release.

- Skills and profiles under `~/.agents` (shared with local-agent)
- Stack presence (`stack check`) and deep readiness (`stack deep`)
- Suggest engine, analytics, templates (builtin), evidence via fcheap
- MCP stdio server; docs site; Glyphrun and Cairn dogfood
- Homebrew cask via GoReleaser

[0.2.0]: https://github.com/abdul-hamid-achik/minerva/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/abdul-hamid-achik/minerva/releases/tag/v0.1.0
