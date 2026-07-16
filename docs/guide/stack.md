# Stack readiness

Presence ≠ readiness. Minerva separates them on purpose.

## Presence (`stack check`)

- PATH probe with **correct** binary names  
- **Tiers**: core missing → unhealthy; optional missing → degraded  
- Fast, parallel, no deep domain logic  

## Deep (`stack deep`)

Composes sibling public JSON contracts:

| Signal | Source |
|--------|--------|
| Bob contract | `bob check/context --json` + `next_actions` |
| Cortex health | doctor + **overview** + stale session samples |
| MCPHub | stats + **status** (`unused_enabled`, agents drift) |
| Codemap | `status --json` (registered, nodes, stale) |
| Vecgrep | `status --format json` (chunks, freshness, profile) |
| fcheap / tvault / monitor | doctors / status |

## Retrieval green light

```json
{
  "retrieval_ready": false,
  "retrieval_gaps": ["codemap", "vecgrep"],
  "retrieval_detail": "codemap: not indexed; vecgrep: …"
}
```

**Green only if both codemap and vecgrep report domain ready.**  
Until then, agents should not treat semantic/graph answers as trustworthy.

## Stash

```bash
minerva stack deep --stash
```

Writes the JSON report to fcheap with `minerva-stack` tags and `outcome:pass|fail` based on `retrieval_ready`.

## Suggest integration

Deep signals feed `minerva suggest`:

- CRIT when retrieval is red  
- HIGH for Cortex stale backlog / low verified rate  
- HIGH for MCPHub high-error servers  
- LOW/MED for unused enabled servers  

Actions point at the owning tool (`cortex show …`, `mcphub doctor --server …`), not a Minerva reimplementation.
