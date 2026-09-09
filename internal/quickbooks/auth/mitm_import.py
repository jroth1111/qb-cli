"""mitmdump -nr DUMP -s mitm_import.py

Persists credentials.json (QB_HOME) and a redacted auth bible.
Never prints secret values.
"""

from __future__ import annotations

import json
import os
from datetime import datetime, timezone
from pathlib import Path

from mitmproxy import http

home = Path(os.environ["QB_HOME"]) if os.environ.get("QB_HOME") else Path.home() / ".config" / "qb"
CRED_PATH = home / "credentials.json"
BIBLE_PATH = home / "mitm-bible.json"


def _parse_set_cookie(sc: str) -> dict | None:
    first, *rest = sc.split(";")
    if "=" not in first:
        return None
    name, value = first.split("=", 1)
    rec = {
        "name": name.strip(),
        "value": value,
        "domain": "",
        "path": "/",
        "expires": None,
        "secure": False,
        "http_only": False,
    }
    for attr in rest:
        attr = attr.strip()
        if not attr:
            continue
        low = attr.lower()
        if low == "secure":
            rec["secure"] = True
        elif low.replace("-", "") == "httponly":
            rec["http_only"] = True
        elif "=" in attr:
            k, v = attr.split("=", 1)
            kl = k.strip().lower()
            if kl == "domain":
                rec["domain"] = v.strip()
            elif kl == "path":
                rec["path"] = v.strip()
            elif kl == "expires":
                rec["expires"] = _http_date(v.strip())
    return rec


def _http_date(raw: str) -> str | None:
    from email.utils import parsedate_to_datetime

    try:
        dt = parsedate_to_datetime(raw)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    except Exception:
        return None


def _header_map_no_cookie(flow: http.HTTPFlow) -> dict[str, str]:
    """All request headers except Cookie. Last value wins. Values are secret."""
    out: dict[str, str] = {}
    for name in flow.request.headers.keys():
        if name.lower() == "cookie":
            continue
        vals = flow.request.headers.get_all(name)
        if vals:
            out[name] = vals[-1]
    return out


class Extract:
    def __init__(self) -> None:
        self.cookies: dict[str, dict] = {}
        self.auths: list[dict] = []
        self.ats_ok = 0
        self.ats_headers: list[dict[str, str]] = []
        self.token_eps: list[dict] = []
        self.hosts: dict[str, int] = {}

    def _ingest_cookie_header(self, host: str, header: str) -> None:
        for part in header.split(";"):
            part = part.strip()
            if "=" not in part:
                continue
            k, v = part.split("=", 1)
            k = k.strip()
            if not k:
                continue
            self.cookies.setdefault(
                k,
                {
                    "name": k,
                    "value": v,
                    "domain": host,
                    "path": "/",
                    "expires": None,
                    "secure": True,
                    "http_only": False,
                },
            )

    def response(self, flow: http.HTTPFlow) -> None:
        if not flow.request or not flow.response:
            return
        host = flow.request.host
        self.hosts[host] = self.hosts.get(host, 0) + 1
        path = flow.request.path.split("?", 1)[0]

        for sc in flow.response.headers.get_all("set-cookie"):
            rec = _parse_set_cookie(sc)
            if rec:
                self.cookies[rec["name"]] = rec

        for ck in flow.request.headers.get_all("cookie"):
            self._ingest_cookie_header(host, ck)

        auth = flow.request.headers.get("authorization")
        if auth:
            scheme = auth.split(" ", 1)[0]
            self.auths.append(
                {
                    "host": host,
                    "path": path,
                    "status": flow.response.status_code,
                    "scheme": scheme,
                    "authorization": auth,
                    "apikey": flow.request.headers.get("apikey")
                    or flow.request.headers.get("apiKey"),
                    "authtype": flow.request.headers.get("authtype")
                    or flow.request.headers.get("authType"),
                    "csrf": flow.request.headers.get("csrftoken"),
                    "xcsrf": flow.request.headers.get("x-csrf-token"),
                    "company": flow.request.headers.get("intuit-company-id"),
                    "appid": flow.request.headers.get("intuit_appid"),
                    "tid": flow.request.headers.get("intuit_tid"),
                    "plugin": flow.request.headers.get("intuit-plugin-id"),
                    "header_names": sorted({k.lower() for k in flow.request.headers.keys()}),
                    "request_headers": _header_map_no_cookie(flow),
                }
            )

        low = (host + path).lower()
        if any(x in low for x in ("oauth", "/token", "refresh", "session")):
            self.token_eps.append(
                {
                    "method": flow.request.method,
                    "host": host,
                    "path": path,
                    "status": flow.response.status_code,
                    "req_ct": flow.request.headers.get("content-type"),
                    "has_refresh_in_body": b"refresh" in (flow.response.content or b"")[:8000].lower(),
                }
            )

        if "/ats/v1/" in path and flow.response.status_code == 200:
            self.ats_ok += 1
            self.ats_headers.append(_header_map_no_cookie(flow))

    def done(self) -> None:
        chosen = None
        for a in reversed(self.auths):
            if a["status"] == 200 and "/ats/v1/" in a["path"] and a["scheme"] == "Intuit_APIKey":
                chosen = a
                break
        if chosen is None:
            for a in reversed(self.auths):
                if a["scheme"] == "Intuit_APIKey" and "qbo.intuit.com" in a["host"]:
                    chosen = a
                    break

        realm = None
        if chosen and chosen.get("company"):
            realm = chosen["company"]
        if not realm and "qbo.currentcompanyid" in self.cookies:
            realm = self.cookies["qbo.currentcompanyid"]["value"]

        request_headers: dict[str, str] = {}
        if self.ats_headers:
            request_headers = self.ats_headers[-1]
        elif chosen and chosen.get("request_headers"):
            request_headers = chosen["request_headers"]

        cred = {
            "version": 1,
            "captured_at": datetime.now(timezone.utc).isoformat(),
            "source": "mitm-login",
            "login_url": "https://accounts.intuit.com/app/sign-in?app_group=QBO&asset_alias=Intuit.accounting.core.qbowebapp&app_environment=prod",
            "final_url": "https://qbo.intuit.com/app/banking",
            "realm_id": realm or "",
            "company_name": "",
            "email": "",
            "token_type": "Intuit_APIKey",
            "access_token": "",
            "api_key": "",
            "intuit_appid": "",
            "authorization": "",
            "request_headers": request_headers,
            "cookies": list(self.cookies.values()),
            "relay_url": "http://127.0.0.1:9224",
        }
        if chosen:
            cred["authorization"] = chosen["authorization"]
            cred["access_token"] = chosen["authorization"]
            if chosen.get("apikey"):
                cred["api_key"] = chosen["apikey"]
            if chosen.get("appid"):
                cred["intuit_appid"] = chosen["appid"]
            if chosen.get("authtype"):
                cred["token_type"] = chosen["authtype"]
            cred["access_expiry"] = datetime.now(timezone.utc).isoformat()

        CRED_PATH.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        tmp = CRED_PATH.with_suffix(".json.tmp")
        tmp.write_text(json.dumps(cred, indent=2))
        tmp.chmod(0o600)
        tmp.replace(CRED_PATH)
        CRED_PATH.chmod(0o600)

        hdr_names = sorted({k.lower() for k in request_headers})
        bible = {
            "source": "mitm /tmp/qb-login.mitm",
            "captured_at": cred["captured_at"],
            "auth_scheme": "Intuit_APIKey",
            "authorization_shape": "Intuit_APIKey intuit_apikey=<key>,intuit_apikey_version=1.0",
            "store_fields": [
                "authorization (full header value — send as Authorization verbatim)",
                "request_headers (full last-200 ATS header map except Cookie)",
                "cookies[] (Set-Cookie union all Cookie headers, including ATS)",
                "api_key, intuit_appid, realm_id",
            ],
            "required_request_headers": hdr_names
            or [
                "authorization",
                "cookie",
                "x-csrf-token",
                "intuit-company-id",
            ],
            "csrf_distinct_from_xcsrf": bool(
                request_headers.get("csrftoken")
                and request_headers.get("x-csrf-token")
                and request_headers.get("csrftoken") != request_headers.get("x-csrf-token")
            ),
            "ats_200_count": self.ats_ok,
            "auth_samples": len(self.auths),
            "cookie_count": len(self.cookies),
            "cookie_names": sorted(self.cookies),
            "token_endpoints": self.token_eps[:40],
            "top_hosts": sorted(self.hosts.items(), key=lambda kv: -kv[1])[:20],
            "has_ticket": any("ticket" in n.lower() for n in self.cookies),
            "has_csrf": "qbo.csrftoken" in self.cookies,
            "has_authorization": bool(cred["authorization"]),
            "request_header_count": len(request_headers),
        }
        BIBLE_PATH.parent.mkdir(parents=True, exist_ok=True)
        BIBLE_PATH.write_text(json.dumps(bible, indent=2))
        print(
            json.dumps(
                {
                    "ok": True,
                    "cred": str(CRED_PATH),
                    "bible": str(BIBLE_PATH),
                    "cookie_count": len(self.cookies),
                    "has_authorization": bool(cred["authorization"]),
                    "has_ticket": bible["has_ticket"],
                    "has_csrf": bible["has_csrf"],
                    "ats_200": self.ats_ok,
                    "token_endpoints": len(self.token_eps),
                    "realm_set": bool(cred["realm_id"]),
                    "request_header_count": len(request_headers),
                    "csrf_distinct_from_xcsrf": bible["csrf_distinct_from_xcsrf"],
                }
            )
        )


addons = [Extract()]
