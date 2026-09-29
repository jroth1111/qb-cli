// ego-browser nodejs < ego_capture.js
// Isolated Space + Network/Fetch intercept of one ATS Intuit_APIKey request.
// Writes capture JSON to QB_CAPTURE_OUT. Never prints secret values.

const h = ego.helpers;
const outPath = process.env.QB_CAPTURE_OUT;
const loginURL = process.env.QB_LOGIN_URL ||
  "https://accounts.intuit.com/app/sign-in?app_group=QBO&asset_alias=Intuit.accounting.core.qbowebapp&app_environment=prod";
const bankingURL = process.env.QB_BANKING_URL || "https://qbo.intuit.com/app/banking";
const auditURL = "https://qbo.intuit.com/app/auditlog";
const timeoutMs = Number(process.env.QB_TIMEOUT_MS || "600000");
const spaceKey = process.env.QB_EGO_SPACE || "qb-login";
// An operator-designated Space (QB_EGO_SPACE/QB_EGO_SPACE_ID set) is a shared
// resource — e.g. an authenticated exec Space serving remint. Keep it alive
// on success; only the self-created default is completed after capture.
const keepSpaceOnSuccess = process.env.QB_KEEP_CAPTURE_SPACE === "true" ||
  (process.env.QB_EGO_SPACE !== undefined && process.env.QB_EGO_SPACE !== "");

function fail(msg, code) {
  cliLog(JSON.stringify({ ok: false, error: msg }));
  process.exit(code || 2);
}

function isAuthURL(url) {
  if (!url) return false;
  let parsed;
  try { parsed = new URL(url); } catch (_) { return false; }
  return parsed.protocol === "https:" && parsed.hostname === "qbo.intuit.com" &&
    !parsed.username && !parsed.password && parsed.pathname.startsWith("/app/") &&
    !url.includes("sign-in") &&
    !url.includes("UNAUTHENTICATED");
}

function headerGet(headers, name) {
  if (!headers) return "";
  if (headers[name]) return String(headers[name]);
  const want = name.toLowerCase();
  for (const [k, v] of Object.entries(headers)) {
    if (String(k).toLowerCase() === want) return String(v);
  }
  return "";
}

function isAPIKey(v) {
  return String(v || "").trim().startsWith("Intuit_APIKey");
}

// The first-party credential rides Intuit_APIKey on qbo.intuit.com requests
// without an `apikey` header (e.g. /api/neo/.../ipd/signedAuthData; /ats/
// historically). apikey-bearing olb/v4-graphql calls use a different key.
function isCredentialRequest(url, headers) {
  if (!isAPIKey(headerGet(headers, "authorization"))) return false;
  if (String(url || "").includes("/ats/")) return true;
  if (!String(url || "").includes("://qbo.intuit.com/")) return false;
  return headerGet(headers, "apikey") === "";
}

// apikey-bearing qbo.intuit.com requests (/olb/, /api/v4/graphql) carry the
// secondary credential trio (apikey/authtype/intuit_appid) the primary
// request no longer carries. Harvested alongside so typed fields fill.
function isAPIKeyRequest(url, headers) {
  return String(url || "").includes("://qbo.intuit.com/") &&
    headerGet(headers, "apikey") !== "";
}

// Neo banking traffic with apikey has its own key even on qbo.intuit.com.
// Preserve that host's current headers alongside the service-host keys.
function serviceHost(url, headers) {
  if (!isAPIKey(headerGet(headers, "authorization"))) return "";
  if (String(url || "").startsWith("https://qbo.intuit.com/api/neo/") &&
      headerGet(headers, "apikey") !== "") return "qbo.intuit.com";
  const m = String(url || "").match(/^https?:\/\/([a-z0-9.-]+\.api\.intuit\.com)\//i);
  return m ? m[1] : "";
}

async function main() {
  if (!outPath) fail("QB_CAPTURE_OUT is required", 2);

  let task;
  if (/^\d+$/.test(spaceKey)) {
    // User-control/ownership failures are a stop, never permission to retake it.
    task = await h.switchTaskSpace(Number(spaceKey));
  } else {
    task = await h.useOrCreateTaskSpace(spaceKey);
  }
  let closed = false;
  const close = async (keep) => {
    if (closed) return;
    closed = true;
    try { await h.cdp("Fetch.disable", {}); } catch (_) {}
    try { await h.completeTaskSpace(task.id, { keep: !!keep }); } catch (_) {}
  };


  try {
    await h.openOrReuseTab(loginURL, { wait: true, timeout: 30 });
    const deadline = Date.now() + timeoutMs;
    // The user completes sign-in in this Space while we poll. Reads may
    // transiently fail while they hold the Space; tolerate that and keep
    // waiting until the deadline — never fail fast on a poll error.
    while (Date.now() < deadline) {
      let info = null;
      try {
        info = await h.pageInfo();
      } catch (_) {
        await h.wait(2);
        continue;
      }
      if (info && info.dialog) {
        await h.wait(1);
        continue;
      }
      if (info && isAuthURL(info.url)) break;
      await h.wait(1);
    }
    let info = null;
    try {
      info = await h.pageInfo();
    } catch (_) {}
    if (!info || !isAuthURL(info.url)) {
      await close(true);
      fail("timed out waiting for authenticated qbo.intuit.com/app tab in ego Space", 2);
    }

    await h.cdp("Network.enable", {});
    await h.cdp("Fetch.enable", {
      patterns: [
        { urlPattern: "*://qbo.intuit.com/*", requestStage: "Request" },
        { urlPattern: "*://*.api.intuit.com/*", requestStage: "Request" },
      ],
    });
    await h.drainEvents();
    try {
      await h.gotoAndWait(bankingURL, { timeout: 45 });
    } catch (_) {
      // Fetch pause or slow SPA; keep draining events.
    }

    let headers = null;
    let apiHeaders = null;
    let auditAuthorization = "";
    const hostHeaders = {};
    // After the primary request lands, keep draining briefly for the
    // apikey-bearing sibling and service-host traffic so the credential
    // fills in one pass.
    const secondaryGraceMs = 8000;
    let primaryAt = 0;
    const observeEvents = async () => {
      let evs = null;
      try { evs = await h.drainEvents(); }
      catch (_) { return; }
      const list = Array.isArray(evs) ? evs : [];
      for (const ev of list) {
        if (!ev || !ev.method) continue;
        let req = null;
        if (ev.method === "Fetch.requestPaused") {
          req = ev.params && ev.params.request;
          const rid = ev.params && ev.params.requestId;
          if (rid) {
            try { await h.cdp("Fetch.continueRequest", { requestId: rid }); } catch (_) {}
          }
        } else if (ev.method === "Network.requestWillBeSent") {
          req = ev.params && ev.params.request;
        }
        if (!req) continue;
        const sh = serviceHost(req.url, req.headers);
        if (sh) hostHeaders[sh] = req.headers;
        if (String(req.url || "").includes("audit.api.intuit.com") &&
            isAPIKey(headerGet(req.headers, "authorization"))) {
          auditAuthorization = headerGet(req.headers, "authorization");
        }
        if (!apiHeaders && isAPIKeyRequest(req.url, req.headers)) {
          apiHeaders = req.headers;
        }
        if (!headers && isCredentialRequest(req.url, req.headers)) {
          headers = req.headers;
          primaryAt = Date.now();
        }
      }
    };
    while (Date.now() < deadline) {
      if (headers && (apiHeaders || Date.now() - primaryAt > secondaryGraceMs)) break;
      await observeEvents();
      if (!headers || !apiHeaders) await h.wait(0.25);
    }
    if (!headers) {
      await close(true);
      fail("no ATS Intuit_APIKey request observed in ego Space", 2);
    }

    if (process.env.QB_CAPTURE_AUDIT === "true") {
      try { await h.gotoAndWait(auditURL, { timeout: 30 }); } catch (_) {}
      const auditDeadline = Math.min(deadline, Date.now() + 12000);
      while (!auditAuthorization && Date.now() < auditDeadline) {
        await observeEvents();
        if (!auditAuthorization) await h.wait(0.25);
      }
      // Keep the retained/login tab on the banking page used by other qb commands.
      try { await h.gotoAndWait(bankingURL, { timeout: 30 }); } catch (_) {}
    }

    let cookies = [];
    try {
      const jar = await h.cdp("Network.getCookies", { urls: ["https://qbo.intuit.com/"] });
      const raw = (jar && jar.cookies) || [];
      cookies = raw.filter((c) => {
        const d = String(c.domain || "").toLowerCase();
        return d.includes("intuit.com") || d.includes("quickbooks");
      }).map((c) => ({
        name: c.name,
        value: c.value,
        domain: c.domain,
        path: c.path || "/",
        expires: c.expires > 0 ? new Date(c.expires * 1000).toISOString() : null,
        secure: !!c.secure,
        http_only: !!c.httpOnly,
      }));
    } catch (_) {}
    // Identity mining: the tab is hydrated here, so the SPA bootstrap
    // carries companyName/email/realmId (verified shapes). Best-effort:
    // Canonical shapes live in auth.IdentityJS (Go); this copy must match.
    let mined = null;
    const expr = `(() => {
      try {
        const html = document.documentElement.innerHTML;
        const pick = (re) => {
          const m = html.match(re);
          return m ? m[1].slice(0, 160) : "";
        };
        return {
          company: pick(/"companyName":"([^"\\\\]{1,120})"/),
          email: pick(/"email":"([^"\\\\]{1,160})"/),
          realm: pick(/"realmId":"([0-9]{1,32})"/),
        };
      } catch (_) {
        return { company: "", email: "", realm: "" };
      }
    })()`;
    try {
      mined = await js(expr);
    } catch (_) {}
    if (!mined || typeof mined !== "object") {
      try {
        mined = await h.evaluate(expr);
      } catch (_) {}
    }
    let identity = { company: "", email: "", realm: "" };
    if (mined && typeof mined === "object") {
      identity = {
        company: String(mined.company || "").slice(0, 160),
        email: String(mined.email || "").slice(0, 160),
        realm: String(mined.realm || "").slice(0, 32),
      };
    }

    const stored = {};
    for (const [k, v] of Object.entries(headers)) {
      if (String(k).toLowerCase() === "cookie") continue;
      if (v == null || v === "") continue;
      stored[k] = String(v);
    }
    const storedApi = {};
    for (const [k, v] of Object.entries(apiHeaders || {})) {
      if (String(k).toLowerCase() === "cookie") continue;
      if (v == null || v === "") continue;
      storedApi[k] = String(v);
    }
    const storedHosts = {};
    for (const [host, hmap] of Object.entries(hostHeaders)) {
      const clean = {};
      for (const [k, v] of Object.entries(hmap || {})) {
        if (String(k).toLowerCase() === "cookie") continue;
        if (v == null || v === "") continue;
        clean[k] = String(v);
      }
      if (Object.keys(clean).length) storedHosts[host] = clean;
    }

    const fs = require("fs");
    fs.writeFileSync(outPath, JSON.stringify({ headers: stored, api_headers: storedApi, host_headers: storedHosts, audit_authorization: auditAuthorization, cookies, identity }, null, 2));
    fs.chmodSync(outPath, 0o600);
    cliLog(JSON.stringify({
      ok: true,
      header_count: Object.keys(stored).length,
      cookie_count: cookies.length,
      audit_captured: !!auditAuthorization,
    }));
    await close(keepSpaceOnSuccess);
  } catch (err) {
    try { await close(true); } catch (_) {}
    fail(String(err && err.message ? err.message : err), 2);

  }
}

main();
