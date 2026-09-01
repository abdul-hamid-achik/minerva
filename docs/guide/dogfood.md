# Dogfooding (glyph & cairn)

## Glyphrun — CLI contracts

PTY specs under `specs/`:

```bash
task glyph-fast
task glyph
```

Covers version, help, isolated skill create, skill update/compare,
learn/resolve, and harness list + propose against a fixture transcript.

## Cairn — docs site contracts

```bash
npm run docs:dev   # already running? skip
task cairn
```

Checks landing hero, Concepts, and Getting started.

After intentional contract changes:

```bash
glyph spec verify ./specs/<name>.yml --stamp
```
