// ego-browser nodejs < ego_capture.js
// Isolated Space + Network/Fetch intercept of one ATS Intuit_APIKey request.
// Writes capture JSON to QB_CAPTURE_OUT. Never prints secret values.

const h = ego.helpers;
const outPath = process.env.QB_CAPTURE_OUT;
const loginURL = process.env.QB_LOGIN_URL ||
  "https://accounts.intuit.com/app/sign-in?app_group=QBO&asset_alias=Intuit.accounting.core.qbowebapp&app_environment=prod";
const bankingURL = process.env.QB_BANKING_URL || "https://qbo.intuit.com/app/banking";
const timeoutMs = Number(process.env.QB_TIMEOUT_MS || "600000");

function fail(msg, code) {
  cliLog(JSON.stringify({ ok: false, error: msg }));
  process.exit(code || 2);
}

function isAuthURL(url) {
  if (!url) return false;
  return url.includes("qbo.intuit.com/app/") &&
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

async function main() {
  if (!outPath) fail("QB_CAPTURE_OUT is required", 2);

  const task = await h.useOrCreateTaskSpace("qb-login");
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
      patterns: [{ urlPattern: "*://qbo.intuit.com/ats/*", requestStage: "Request" }],
    });
    await h.drainEvents();
    try {
      await h.gotoAndWait(bankingURL, { timeout: 45 });
    } catch (_) {
      // Fetch pause or slow SPA; keep draining events.
    }

    let headers = null;
    while (Date.now() < deadline && !headers) {
      let evs = null;
      try {
        evs = await h.drainEvents();
      } catch (_) {
        await h.wait(1);
        continue;
      }
      const list = Array.isArray(evs) ? evs : [];
      for (const ev of list) {
        if (!ev || !ev.method) continue;
        if (ev.method === "Fetch.requestPaused") {
          const req = ev.params && ev.params.request;
          const rid = ev.params && ev.params.requestId;
          if (rid) {
            try { await h.cdp("Fetch.continueRequest", { requestId: rid }); } catch (_) {}
          }
          if (req && String(req.url || "").includes("/ats/v1/") && isAPIKey(headerGet(req.headers, "Authorization"))) {
            headers = req.headers;
          }
        }
        if (ev.method === "Network.requestWillBeSent") {
          const req = ev.params && ev.params.request;
          if (req && String(req.url || "").includes("/ats/v1/") && isAPIKey(headerGet(req.headers, "Authorization"))) {
            headers = req.headers;
          }
        }
      }
      if (!headers) await h.wait(0.25);
    }
    if (!headers) {
      await close(true);
      fail("no ATS Intuit_APIKey request observed in ego Space", 2);
    }

    let cookies = [];
    try {
      const jar = await h.cdp("Network.getAllCookies", {});
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

    const fs = require("fs");
    fs.writeFileSync(outPath, JSON.stringify({ headers: stored, cookies, identity }, null, 2));
    fs.chmodSync(outPath, 0o600);
    cliLog(JSON.stringify({
      ok: true,
      header_count: Object.keys(stored).length,
      cookie_count: cookies.length,
    }));
    await close(false);
  } catch (err) {
    try { await close(true); } catch (_) {}
    fail(String(err && err.message ? err.message : err), 2);

  }
}

main();
