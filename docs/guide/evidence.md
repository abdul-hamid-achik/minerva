# Evidence & fcheap

Minerva does **not** reimplement an artifact vault. It shells **fcheap** with standard tags so outcomes become searchable and suggestable.

## Tag scheme

| Tag | Meaning |
|-----|---------|
| `minerva` | Always present |
| `minerva-eval` | Behavioral / eval artifact |
| `minerva-stack` | Stack deep / readiness snapshot |
| `minerva-suggest` | Suggest-related capture |
| `minerva-incident` | Ops incident |
| `outcome:pass` / `fail` / `skip` | Result |
| `skill:<name>` | Attribution |
| `profile:<name>` | Attribution |

## Commands

```bash
minerva evidence docs
minerva evidence save ./run-dir --kind eval --outcome fail \
  --tag skill:qa-tester --tag profile:code-reviewer --index
minerva stack deep --stash
fcheap list --tag minerva --tag outcome:fail --json
```

## Closed loop

```text
eval or stack deep
    → fcheap (tags)
    → minerva suggest
    → human / host applies library changes
```

::: warning Never stash secrets
Do not save `.env`, vault dumps, or tokens. Use **tvault** for secrets; reference key **names** only in skills.
:::
