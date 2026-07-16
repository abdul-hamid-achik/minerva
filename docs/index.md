---
layout: home
title: Agent library operator for the intelligence stack
description: Manage skills and profiles under ~/.agents, probe stack readiness with fail-closed retrieval signals, and close the loop with fcheap evidence — without becoming a second agent runtime.
pageClass: minerva-home
hero:
  name: Minerva
  text: Library control for agent harnesses
  tagline: Skills, profiles, and honest stack readiness — for local-agent, MCPHub, Cortex, and the rest of your intelligence fleet. Not a second runtime. Not a cosplay self-improvement loop.
  image:
    src: /favicon.svg
    alt: Minerva mark
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Architecture
      link: /guide/architecture
    - theme: alt
      text: GitHub
      link: https://github.com/abdul-hamid-achik/minerva
features:
  - title: Shared ~/.agents SSOT
    details: Create and update skills and profiles on the same disk layout local-agent already loads. No shadow activation fantasy.
  - title: Fail-closed retrieval
    details: retrieval_ready is green only when codemap and vecgrep are domain-ready — not merely present on PATH.
  - title: Operator intelligence
    details: Compose Bob, Cortex, MCPHub, monitor, and fcheap signals into ranked suggestions with real next actions.
  - title: Evidence loop
    details: Stash stack reports and eval outcomes with standard tags. Suggest from outcome:fail and skill/profile attribution.
  - title: MCP for harnesses
    details: Expose library and readiness tools over stdio for MCPHub lazy routing and local-agent trust catalogs.
  - title: Dogfooded contracts
    details: Glyphrun PTY specs for the CLI and Cairn browser specs for this site keep the surface honest.
---

## Authority map

Minerva **orchestrates**. Sibling tools **own** their domains.

| Lane | Owner |
|------|--------|
| Prompt assembly, session skills, MCP trust | **local-agent** |
| Task evidence lifecycle | **Cortex** |
| MCP gateway, sync, tool_calls | **MCPHub** |
| Repo contract / drift | **Bob** |
| Graph / semantic readiness | **Codemap / Vecgrep** |
| Durable artifacts | **fcheap** |
| Secrets (metadata only) | **tvault** |
| Skills & profiles on disk + readiness dashboard | **Minerva** |

## Quick install

```bash
go install github.com/abdul-hamid-achik/minerva/cmd/minerva@latest
minerva init
minerva stack check
minerva stack deep
minerva suggest
```

## What Minerva is not

- Not a second agent runtime
- Not Cortex (no task verify/remember ownership)
- Not MCPHub (no harness sync or gateway)
- Not silent auto-mutation of production prompts under AUTO

::: tip Honest activation
`minerva skill activate` updates Minerva-local state only. For durable harness behavior, put skills on a **profile** that local-agent loads.
:::
