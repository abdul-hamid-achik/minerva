# AGENTS.md

Instructions for agents working on Minerva. `CLAUDE.md` points here.

Read `README.md` for what Minerva is (skill intelligence: session traces →
proposed SKILL.md + cross-harness sync).

## Docs site (Vercel)

Repo-root `vercel.json` auto-builds **`main` only**. Feature branches do not
create Preview deployments. `ignoreCommand` skips the build unless `docs/`,
lockfiles, or `vercel.json` changed. Do not `vercel promote` this site; `main`
is the docs release. The CLI/binary release is a separate tag pipeline. Do not
add GitHub Pages.
