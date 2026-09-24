// Existing-tab capture. Never claims spaces, opens tabs, or intercepts requests.
// The selected banking tab is reloaded to observe fresh authenticated traffic.
const fs = await import('node:fs/promises');
const task = await taskSpace(process.env.QB_EGO_SPACE || 'qb-gql');
function qboURL(value) {
  try {
    const u = new URL(value);
    return u.protocol === 'https:' && u.hostname === 'qbo.intuit.com' &&
      !u.username && !u.password && u.pathname === '/app/banking';
  } catch { return false; }
}
const tabs = (await task.tabs()).filter(t => qboURL(t.url) &&
  (!captureTarget || t.label === captureTarget || t.targetId === captureTarget));
if (tabs.length !== 1) throw new Error('Select exactly one existing QBO banking tab using --target-id');
const tab = tabs[0];
const page = tab.label ? task.page(tab.label) : await task.adopt(tab.targetId);
const info = await page.info();
if (!qboURL(info.url)) throw new Error('Selected tab is no longer QBO banking');
if (info.dialog) throw new Error('Dismiss the browser dialog before capturing');
const before = await page.evaluate(identityExpression);
if (!before.realm || !before.email) throw new Error('Existing tab lacks a verifiable company and principal');
const header = (h, name) => String(Object.entries(h || {}).find(([k]) => k.toLowerCase() === name)?.[1] || '');
let primary, secondary;
const hosts = {};
function observe(url, h) {
  let u;
  try { u = new URL(url); } catch { return; }
  if (u.protocol !== 'https:' || u.username || u.password) return;
  const first = u.hostname === 'qbo.intuit.com';
  if (!first && !u.hostname.endsWith('.api.intuit.com')) return;
  const key = header(h, 'authorization').startsWith('Intuit_APIKey');
  if (first && key && !header(h, 'apikey')) primary = h;
  if (first && header(h, 'apikey')) secondary = h;
  if (key && (!first || (u.pathname.startsWith('/api/neo/') && header(h, 'apikey')))) hosts[u.hostname] = h;
}
await page.cdp('Network.enable', {});
await page.events();
await page.cdp('Page.reload', {});
const deadline = Date.now() + Math.min(Number(process.env.QB_TIMEOUT_MS || 90000), 90000);
const requests = new Map();
const extras = new Map();
let capturedAt = 0;
while (Date.now() < deadline) {
  for (const e of await page.events()) {
    const p = e.params || {};
    if (e.method === 'Network.requestWillBeSent') {
      requests.set(p.requestId, p.request.url);
      observe(p.request.url, p.request.headers);
      if (extras.has(p.requestId)) observe(p.request.url, extras.get(p.requestId));
    } else if (e.method === 'Network.requestWillBeSentExtraInfo') {
      extras.set(p.requestId, p.headers);
      if (requests.has(p.requestId)) observe(requests.get(p.requestId), p.headers);
    }
  }
  if (primary && !capturedAt) capturedAt = Date.now();
  if (primary && secondary && Date.now() - capturedAt >= 8000) break;
  await new Promise(resolve => setTimeout(resolve, 250));
}
if (!primary) throw new Error('No fresh primary authorization observed; saved credentials unchanged');
const after = await page.evaluate(identityExpression);
if (!qboURL(await page.url()) || before.realm !== after.realm ||
    before.email.trim().toLowerCase() !== (after.email || '').trim().toLowerCase()) {
  throw new Error('Company or principal changed during capture; saved credentials unchanged');
}
const jar = await page.cdp('Network.getCookies', {urls: ['https://qbo.intuit.com/']});
const cookies = (jar.cookies || []).filter(c => ['intuit.com', 'qbo.intuit.com'].includes(c.domain.toLowerCase().replace(/^\./, ''))).map(c => ({
  name: c.name, value: c.value, domain: c.domain, path: c.path || '/', secure: c.secure,
  http_only: c.httpOnly, host_only: !c.domain.startsWith('.'),
  expires: c.expires > 0 ? new Date(c.expires * 1000).toISOString() : '0001-01-01T00:00:00Z'
}));
if (!cookies.length) throw new Error('Empty QBO cookie jar');
await fs.writeFile(process.env.QB_CAPTURE_OUT, JSON.stringify({headers: primary,
  api_headers: secondary, host_headers: hosts, cookies, identity: after}), {mode: 0o600});
console.log(JSON.stringify({captured: true, cookie_count: cookies.length, secondary: !!secondary}));
