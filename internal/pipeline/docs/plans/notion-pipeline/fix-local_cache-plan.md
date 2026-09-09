# Fix Plan: local_cache

**API:** notion-cli
**Current Score:** 0/10
**Target Score:** 8/10

## Templates to Create

- `templates/cache.go.tmpl` (new)

## What to Change

This requires a new template. Create cache.go.tmpl that:
- Uses a local SQLite or bolt database in ~/.cache/<cli-name>/
- Caches GET responses with configurable TTL
- Provides --no-cache flag to bypass
- Provides --clear-cache flag to purge

Note: No existing template covers this - a new cache.go.tmpl is needed.

## Verification

After applying changes, re-run the scorecard:

```bash
printing-press scorecard --api notion-cli
```

The local_cache dimension should score at least 8/10.
