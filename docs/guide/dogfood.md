# Dogfooding (glyph & cairn)

## Glyphrun — CLI contracts

PTY specs under `specs/`:

```bash
task glyph-fast   # quick suite
task glyph        # full suite including stack deep
```

Covers version, help, stack check JSON binary map, isolated skill create, evidence docs, and `retrieval_ready` field presence.

## Cairn — docs site contracts

Browser specs under `browser-specs/` against the VitePress site:

```bash
# terminal 1
npm run docs:dev

# terminal 2
task cairn
# or against production
cairn run ./browser-specs --env production --format md
```

Checks landing hero, navigation to Getting started, and core messaging.

## Why both

| Layer | Tool | Proves |
|-------|------|--------|
| CLI | glyph | Operator commands and JSON contracts |
| Site | cairn | Public docs UX and cold-start load |

After intentional contract changes:

```bash
glyph spec verify ./specs/<name>.yml --stamp
cairn run ./browser-specs --stamp-if-green   # if supported for your cairn version
```
