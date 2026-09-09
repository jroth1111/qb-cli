# QBO auth bible (from a mitmproxy capture of the QBO login)

Source of truth for `qb login` and credential storage. Captured 2026-08-17 via mitmdump.

## What to store

| Field | Why |
|---|---|
| `authorization` | Full `Authorization` header from a 200 ATS request. Send **verbatim**. Shape: `Intuit_APIKey intuit_apikey=…,intuit_apikey_version=1.0` |
| `request_headers` | Full last-200 ATS request header map except `Cookie`. Includes distinct `csrftoken` vs `x-csrf-token`, `apikey`, `intuit_appid`, `intuit_tid`, Chrome `User-Agent` / `sec-ch-ua*`, `Accept: */*`, dump `Referer`. |
| `cookies[]` | Union of Set-Cookie and every request `Cookie` header (including ATS). Required names: `qbn.ticket`, `qbo.ticket`, `qbn.authid`, `qbo.authid`, `qbo.csrftoken`, `qbo.currentcompanyid` |
| `realm_id` | `intuit-company-id` / `qbo.currentcompanyid` |
| `api_key` / `intuit_appid` | Typed copies of ATS headers |

There is **no** OAuth `refresh_token` in the dump. Cookie `Expires` can be 2036; that is not a remint API.

## How tickets are minted (observed)

1. `POST https://accounts.intuit.com/identity-api/signin/graphql` → Set-Cookie `qbn.ticket`, `qbn.authid`, `qbn.gauthid` (HttpOnly, Secure)
2. `POST https://accounts.intuit.com/identity-api/v2/graphql` → same cookies
3. `GET https://qbo.intuit.com/app/postlogin` → Set-Cookie `qbn.ticket`, `qbo.ticket`, `qbn.authid`, `qbo.authid`

## How Intuit_APIKey appears (observed)

Not a Set-Cookie. Not a JSON token endpoint. The SPA attaches `Authorization: Intuit_APIKey …` to ATS/neo after postlogin. First ATS 200 in the dump: `GET /ats/v1/company/{realm}/banking/getInitialData`.

## Remint

No OAuth refresh URL.

## Interactive `qb login` / `qb auth remint`

Same ladder:
1. Authenticated `qbo.intuit.com/app/*` tab on the OMP relay → intercept ATS.
2. Else isolated ego Space (`qb-login`). Sign-in stays in that Space.
3. `--no-open`: relay wait only. `--from-mitm PATH`: dump import.

`qb feed` 401 tries step 1 only (~12s). Failed ego capture leaves the Space open.



## Import

```
qb login --from-mitm ~/capture.mitm
```

Runs `mitmdump -nr <dump> -s internal/quickbooks/auth/mitm_import.py`.
