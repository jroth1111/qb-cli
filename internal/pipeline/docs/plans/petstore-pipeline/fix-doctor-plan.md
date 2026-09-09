# Fix Plan: doctor

**API:** petstore-cli
**Current Score:** 2/10
**Target Score:** 8/10

## Templates to Modify

- `templates/doctor.go.tmpl`

## What to Change

Add health check HTTP calls for each API dependency:
- Base API URL reachability
- Auth endpoint validation
- Rate limit status check
- Each check should use http.Get and report pass/fail

Template to modify: doctor.go.tmpl (add http.Get health check functions).

## Verification

After applying changes, re-run the scorecard:

```bash
printing-press scorecard --api petstore-cli
```

The doctor dimension should score at least 8/10.
