// term-rest-client web UI. Plain JavaScript, no build step: the page edits a
// copy of the workspace and PUTs it back; every request is sent by the Go
// process (POST /api/send streams NDJSON, /api/socket bridges WebSockets).
"use strict";

try { document.documentElement.dataset.theme = localStorage.getItem("trc.theme") || "dark"; } catch (e) { /* storage blocked */ }

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"];
const BODY_TYPES = [["none", "None"], ["json", "JSON"], ["text", "Text"], ["xml", "XML"], ["form", "Form"], ["graphql", "GraphQL"]];
const AUTH_TYPES = [["none", "No auth"], ["basic", "Basic"], ["bearer", "Bearer token"], ["apikey", "API key"]];
const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
const MOD = isMac ? "⌘" : "Ctrl";

// ---------- tiny DOM helpers ----------

function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null || v === false) continue;
      if (k === "class") el.className = v;
      else if (k === "style" && typeof v === "object") Object.assign(el.style, v);
      else if (k.startsWith("on")) el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (k === "value") el.value = v;
      else if (k === "checked") el.checked = !!v;
      else if (k === "html") el.innerHTML = v;
      else if (k === "dataset") Object.assign(el.dataset, v);
      else el.setAttribute(k, v === true ? "" : v);
    }
  }
  append(el, kids);
  return el;
}
function append(el, kids) {
  for (const k of kids) {
    if (k == null || k === false) continue;
    if (Array.isArray(k)) append(el, k);
    else el.append(k instanceof Node ? k : String(k));
  }
}
const $ = (id) => document.getElementById(id);
const clone = (v) => (v == null ? v : JSON.parse(JSON.stringify(v)));
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const ICONS = {
  chev: '<path d="m9 6 6 6-6 6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  more: '<circle cx="5" cy="12" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="19" cy="12" r="1.3"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  box: '<path d="M21 8 12 3 3 8v8l9 5 9-5z"/><path d="m3 8 9 5 9-5M12 13v8"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
  code: '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>',
  save: '<path d="M5 3h11l5 5v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/><path d="M7 3v5h8M7 21v-7h10v7"/>',
  send: '<path d="M22 2 11 13M22 2l-7 20-4-9-9-4z"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2"/>',
  plug: '<path d="M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0zM12 18v4"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6"/>',
  edit: '<path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  dup: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M4 16V5a1 1 0 0 1 1-1h11"/>',
  download: '<path d="M12 3v12m0 0-4-4m4 4 4-4M5 21h14"/>',
  upload: '<path d="M12 21V9m0 0-4 4m4-4 4 4M5 3h14"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  alert: '<path d="M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4m0-4h.01"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  layers: '<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
  wand: '<path d="M15 4V2m0 8V8m-4-4H9m10 0h-2M5 21l11-11M18.5 6.5l-1-1"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  cols: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>',
  rows: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 12h18"/>',
  schema: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8 7.5l3 8M16 7.5l-3 8"/>',
  play: '<path d="m7 4 13 8-13 8z"/>',
};
function icon(name, cls) {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 24 24");
  s.setAttribute("fill", "none");
  s.setAttribute("stroke", "currentColor");
  s.setAttribute("stroke-width", "2");
  s.setAttribute("stroke-linecap", "round");
  s.setAttribute("stroke-linejoin", "round");
  s.setAttribute("class", cls || "icon-svg");
  s.innerHTML = ICONS[name] || "";
  return s;
}
function btn(label, opts = {}) {
  const b = h("button", { class: "btn " + (opts.cls || ""), title: opts.title, onclick: opts.onclick, type: "button", disabled: opts.disabled });
  if (opts.icon) b.append(icon(opts.icon));
  if (label) b.append(h("span", null, label));
  return b;
}

// ---------- formatting ----------

function humanBytes(n) {
  if (n == null) return "";
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(n < 10240 ? 1 : 0) + " KB";
  return (n / 1024 / 1024).toFixed(1) + " MB";
}
function humanMs(ms) {
  if (ms == null) return "";
  if (ms < 1) return ms.toFixed(2) + " ms";
  if (ms < 1000) return Math.round(ms) + " ms";
  if (ms < 60000) return (ms / 1000).toFixed(ms < 10000 ? 2 : 1) + " s";
  return Math.floor(ms / 60000) + "m " + Math.round((ms % 60000) / 1000) + "s";
}
function ago(t) {
  const d = (Date.now() - new Date(t).getTime()) / 1000;
  if (d < 45) return "just now";
  if (d < 3600) return Math.round(d / 60) + "m ago";
  if (d < 86400) return Math.round(d / 3600) + "h ago";
  return new Date(t).toLocaleDateString();
}
function statusClass(s) { return s >= 500 ? "s5" : s >= 400 ? "s4" : s >= 300 || (s > 0 && s < 200) ? "s3" : s >= 200 ? "s2" : "serr"; }
function headerList(hdrs) {
  const out = [];
  for (const k of Object.keys(hdrs || {}).sort((a, b) => a.localeCompare(b))) for (const v of hdrs[k]) out.push([k, v]);
  return out;
}
function headerText(hdrs) { return headerList(hdrs).map(([k, v]) => k + ": " + v).join("\n") + "\n"; }
function headerJSON(hdrs) {
  const o = {};
  for (const [k, vs] of Object.entries(hdrs || {})) o[k] = vs.length === 1 ? vs[0] : vs;
  return JSON.stringify(o, null, 2) + "\n";
}
function headerValue(hdrs, name) {
  for (const [k, v] of Object.entries(hdrs || {})) if (k.toLowerCase() === name.toLowerCase()) return v[0];
  return "";
}
function tryJSON(text) {
  const t = (text || "").trim();
  if (!t || (t[0] !== "{" && t[0] !== "[")) return undefined;
  try { return JSON.parse(t); } catch (e) { return undefined; }
}
function highlightJSON(text) {
  return esc(text).replace(/(&quot;(?:\\.|[^\\&]|&(?!quot;))*?&quot;)(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|([{}\[\],])/g,
    (m, str, colon, lit, num, p) => {
      if (str) return colon ? `<span class="j-key">${str}</span><span class="j-p">${colon}</span>` : `<span class="j-str">${str}</span>`;
      if (lit) return `<span class="j-lit">${lit}</span>`;
      if (num) return `<span class="j-num">${num}</span>`;
      return `<span class="j-p">${p}</span>`;
    });
}
function varSpans(text, vars) {
  return esc(text).replace(/\{\{\s*([^{}]+?)\s*\}\}/g, (m, name) => {
    const known = name.startsWith("$") || Object.prototype.hasOwnProperty.call(vars, name);
    return `<span class="var${known ? "" : " missing"}">${m}</span>`;
  });
}
function parseKV(text, sep) {
  const out = [];
  for (let line of (text || "").split("\n")) {
    line = line.trim();
    if (!line) continue;
    let disabled = false;
    if (line.startsWith("#")) { disabled = true; line = line.slice(1).trim(); }
    const i = line.indexOf(sep.trim());
    const key = (i < 0 ? line : line.slice(0, i)).trim();
    const value = i < 0 ? "" : line.slice(i + sep.trim().length).trim();
    out.push({ key, value, disabled });
  }
  return out;
}
function formatKV(kvs, sep) {
  return kvs.filter((kv) => kv.key || kv.value).map((kv) => (kv.disabled ? "# " : "") + kv.key + sep + kv.value).join("\n");
}

// ---------- state ----------

const S = {
  ws: null, history: [], info: {}, languages: [],
  tabs: [], active: null, sideTab: "collections", filter: "",
  open: new WeakMap(), // collection/folder -> expanded
  selHist: -1,
};
let tabSeq = 0;

function normReq(r) {
  r = r || {};
  r.name = r.name || "";
  r.method = r.method || "GET";
  r.url = r.url || "";
  r.params = r.params || [];
  r.headers = r.headers || [];
  r.auth = Object.assign({ type: "none", in: "header" }, r.auth || {});
  if (!r.auth.type) r.auth.type = "none";
  r.body_type = r.body_type || "none";
  r.body = r.body || "";
  r.graphql_variables = r.graphql_variables || "";
  r.pre_request = r.pre_request || "";
  r.tests = r.tests || "";
  return r;
}
function canon(r) {
  r = normReq(clone(r));
  const kv = (l) => l.filter((x) => x.key || x.value).map((x) => [x.key, x.value, !!x.disabled]);
  const a = r.auth;
  return JSON.stringify([r.name, r.type || "", r.method, r.url, kv(r.params), kv(r.headers), a.type, a.username || "", a.password || "", a.token || "",
    a.key || "", a.value || "", a.in || "", r.body_type, r.body, r.graphql_variables, r.pre_request, r.tests]);
}
function newRequest(name) { return normReq({ name: name || "New request", method: "GET", url: "", auth: { type: "none", in: "header" }, body_type: "none" }); }
function label(r) { return r.type === "websocket" ? "WS" : (r.method || "GET"); }
function vars() {
  const m = {};
  const add = (l) => (l || []).forEach((kv) => { if (!kv.disabled && kv.key) m[kv.key] = kv.value; });
  add(S.ws.variables);
  const env = (S.ws.environments || []).find((e) => e.name === S.ws.active_environment);
  if (env) add(env.variables);
  return m;
}

// walk visits every request: fn(req, trail) where trail is [collection, folder...].
function walk(fn) {
  const rec = (c, trail) => {
    const t = trail.concat([c]);
    (c.requests || []).forEach((r) => fn(r, t));
    (c.folders || []).forEach((f) => rec(f, t));
  };
  (S.ws.collections || []).forEach((c) => rec(c, []));
}
function trailOf(req) { let found = null; walk((r, t) => { if (r === req) found = t; }); return found; }
function sourceOf(tab) {
  const t = tab.origin && trailOf(tab.origin);
  return t ? t.map((c) => c.name).concat([tab.origin.name]).join(" / ") : "";
}
// paths identify requests across a workspace reload (by index).
function pathOf(req) {
  let out = null;
  const rec = (c, p) => {
    (c.requests || []).forEach((r, i) => { if (r === req) out = p.concat(["r" + i]); });
    (c.folders || []).forEach((f, i) => rec(f, p.concat(["f" + i])));
  };
  (S.ws.collections || []).forEach((c, i) => rec(c, ["c" + i]));
  return out;
}
function byPath(p) {
  if (!p) return null;
  let c = (S.ws.collections || [])[+p[0].slice(1)];
  for (const s of p.slice(1, -1)) c = c && (c.folders || [])[+s.slice(1)];
  return c && (c.requests || [])[+p[p.length - 1].slice(1)] || null;
}

// ---------- API ----------

async function api(method, path, body) {
  const res = await fetch(path, { method, headers: body ? { "Content-Type": "application/json" } : {}, body: body ? JSON.stringify(body) : undefined });
  let data = null;
  try { data = await res.json(); } catch (e) { /* empty */ }
  if (!res.ok) throw new Error((data && data.error) || res.status + " " + res.statusText);
  return data;
}
let saveChain = Promise.resolve();
let saveTimer = null;
function persist(now) {
  clearTimeout(saveTimer);
  const run = () => {
    const body = clone(S.ws);
    saveChain = saveChain.then(() => api("PUT", "/api/workspace", body)).catch((e) => toast("err", "Could not save", e.message));
  };
  if (now) run(); else saveTimer = setTimeout(run, 250);
}
// replaceWorkspace swaps in a workspace from the server, keeping open tabs
// pointed at the same saved requests.
function replaceWorkspace(ws) {
  const paths = S.tabs.map((t) => t.origin && pathOf(t.origin));
  S.ws = ws;
  S.ws.collections = S.ws.collections || [];
  S.tabs.forEach((t, i) => {
    if (!t.origin) return;
    const r = byPath(paths[i]);
    if (r) t.origin = r; else t.origin = null;
  });
}

// ---------- toasts ----------

function toast(kind, title, detail) {
  const t = h("div", { class: "toast " + kind },
    icon(kind === "ok" ? "check" : kind === "err" ? "alert" : kind === "warn" ? "alert" : "info", "ti"),
    h("div", null, title, detail ? h("small", null, detail) : null));
  $("toasts").append(t);
  const close = () => { t.classList.add("out"); setTimeout(() => t.remove(), 200); };
  t.addEventListener("click", close);
  setTimeout(close, kind === "err" ? 6000 : 2600);
}
async function copy(text, what) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (e) {
    const ta = h("textarea", { style: { position: "fixed", opacity: "0" } });
    ta.value = text;
    document.body.append(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
  }
  toast("ok", "Copied " + what, humanBytes(new Blob([text]).size));
}
function download(name, text, type) {
  const a = h("a", { href: URL.createObjectURL(new Blob([text], { type: type || "application/octet-stream" })), download: name });
  document.body.append(a);
  a.click();
  setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
}

// ---------- popovers and dialogs ----------

let openPop = null;
function closePop() { if (openPop) { openPop.remove(); openPop = null; } }
function popover(anchor, items, opts = {}) {
  closePop();
  const pop = h("div", { class: "pop" });
  for (const it of items) {
    if (it === "-") { pop.append(h("div", { class: "sep" })); continue; }
    if (it.heading) { pop.append(h("div", { class: "heading" }, it.heading)); continue; }
    pop.append(h("div", {
      class: "item" + (it.danger ? " danger" : ""),
      onclick: (e) => { e.stopPropagation(); closePop(); it.run(); },
    }, it.icon ? icon(it.icon) : null, it.html ? h("span", { html: it.html }) : h("span", null, it.label), it.kbd ? h("span", { class: "k" }, it.kbd) : null));
  }
  document.body.append(pop);
  const r = anchor.getBoundingClientRect ? anchor.getBoundingClientRect() : { left: anchor.x, right: anchor.x, top: anchor.y, bottom: anchor.y };
  const pw = pop.offsetWidth, ph = pop.offsetHeight;
  let left = opts.alignRight ? r.right - pw : r.left;
  let top = r.bottom + 6;
  if (left + pw > innerWidth - 8) left = innerWidth - pw - 8;
  if (top + ph > innerHeight - 8) top = Math.max(8, r.top - ph - 6);
  pop.style.left = Math.max(8, left) + "px";
  pop.style.top = top + "px";
  openPop = pop;
  return pop;
}
document.addEventListener("mousedown", (e) => { if (openPop && !openPop.contains(e.target)) closePop(); });

const dialogs = [];
function dialog({ title, sub, body, footer, wide, onclose, cls }) {
  const dlg = h("div", { class: "dialog" + (wide ? " wide" : "") },
    h("header", null, h("div", { class: "grow" }, h("h2", null, title), sub ? h("p", null, sub) : null),
      btn("", { cls: "ghost icon sm", icon: "x", onclick: () => close(), title: "Close (Esc)" })),
    h("div", { class: "body" }, body),
    footer ? h("footer", null, footer) : null);
  const ov = h("div", { class: "overlay" + (cls ? " " + cls : "") }, dlg);
  ov.addEventListener("mousedown", (e) => { if (e.target === ov) close(); });
  document.body.append(ov);
  const entry = { ov, close };
  dialogs.push(entry);
  function close() {
    const i = dialogs.indexOf(entry);
    if (i < 0) return;
    dialogs.splice(i, 1);
    ov.remove();
    if (onclose) onclose();
  }
  setTimeout(() => { const f = dlg.querySelector("[autofocus]") || dlg.querySelector("input, textarea, select"); if (f) f.focus(); }, 30);
  return entry;
}
function ask(title, { value = "", placeholder = "", sub, ok = "Save" } = {}) {
  return new Promise((resolve) => {
    let done = false;
    const input = h("input", { class: "input", value, placeholder, autofocus: true });
    const finish = (v) => { done = true; d.close(); resolve(v); };
    input.addEventListener("keydown", (e) => { if (e.key === "Enter") finish(input.value.trim()); });
    const d = dialog({
      title, sub, body: input,
      footer: [btn("Cancel", { onclick: () => d.close() }), btn(ok, { cls: "accent", onclick: () => finish(input.value.trim()) })],
      onclose: () => { if (!done) resolve(null); },
    });
    setTimeout(() => input.select(), 40);
  });
}
function confirmBox(title, sub, ok = "Delete", choices) {
  return new Promise((resolve) => {
    let done = false;
    const finish = (v) => { done = true; d.close(); resolve(v); };
    const buttons = choices ? choices.map(([label, value, cls]) => btn(label, { cls, onclick: () => finish(value) }))
      : [btn("Cancel", { onclick: () => finish(false) }), btn(ok, { cls: "accent", onclick: () => finish(true) })];
    const d = dialog({ title, sub, body: null, footer: buttons, onclose: () => { if (!done) resolve(choices ? null : false); } });
    setTimeout(() => buttons[buttons.length - 1].focus(), 40);
  });
}

// ---------- tabs (open requests) ----------

function openTab(req, origin, opts = {}) {
  if (origin) {
    const existing = S.tabs.find((t) => t.origin === origin);
    if (existing) { activate(existing); return existing; }
  }
  // Reuse an untouched, empty tab.
  const cur = activeTab();
  const blank = cur && !cur.origin && !cur.result && canon(cur.req) === canon(newRequest()) ? cur : null;
  const tab = {
    id: ++tabSeq, req: normReq(clone(req)), origin: origin || null, result: opts.result || null,
    reqTab: opts.reqTab || null, respTab: null, bodyView: "pretty", hdrView: "response",
  };
  if (blank) S.tabs.splice(S.tabs.indexOf(blank), 1, tab); else S.tabs.push(tab);
  activate(tab);
  return tab;
}
function activeTab() { return S.tabs.find((t) => t.id === S.active) || null; }
function activate(tab) {
  S.active = tab ? tab.id : null;
  renderAll();
  saveSession();
}
function isDirty(tab) { return tab.origin ? canon(tab.req) !== canon(tab.origin) : !!(tab.req.url || tab.req.body); }
async function closeTab(tab) {
  if (tab.origin && isDirty(tab)) {
    const choice = await confirmBox("Save changes to \"" + tab.req.name + "\"?", "Your edits are lost if you close without saving.", null,
      [["Cancel", null], ["Discard", "discard", "danger"], ["Save", "save", "accent"]]);
    if (!choice) return;
    if (choice === "save") save(tab);
  }
  stopTab(tab);
  const i = S.tabs.indexOf(tab);
  S.tabs.splice(i, 1);
  if (S.active === tab.id) {
    const next = S.tabs[Math.min(i, S.tabs.length - 1)];
    S.active = next ? next.id : null;
  }
  if (!S.tabs.length) openTab(newRequest());
  else { renderAll(); saveSession(); }
}
function saveSession() {
  try {
    const tabs = S.tabs.map((t) => ({ path: t.origin && pathOf(t.origin), req: t.req }));
    localStorage.setItem("trc.tabs", JSON.stringify({ tabs, active: S.tabs.findIndex((t) => t.id === S.active) }));
  } catch (e) { /* storage blocked */ }
}
function restoreSession() {
  let saved = null;
  try { saved = JSON.parse(localStorage.getItem("trc.tabs") || "null"); } catch (e) { /* ignore */ }
  if (saved && saved.tabs && saved.tabs.length) {
    for (const t of saved.tabs) {
      const origin = byPath(t.path);
      S.tabs.push({ id: ++tabSeq, req: normReq(t.req), origin, result: null, reqTab: null, respTab: null, bodyView: "pretty", hdrView: "response" });
    }
    const a = S.tabs[saved.active] || S.tabs[0];
    S.active = a.id;
    return;
  }
  let first = null;
  walk((r) => { if (!first) first = r; });
  if (first) openTab(first, first); else openTab(newRequest());
}
function changed(tab) {
  // Light refresh after an edit: tab strip, dirty marker, URL highlight.
  renderOpenTabs();
  const dirty = document.querySelector("#reqPane .dirty");
  if (dirty) dirty.style.visibility = isDirty(tab) ? "visible" : "hidden";
  sessionTimer();
}
let sessTimer = null;
function sessionTimer() { clearTimeout(sessTimer); sessTimer = setTimeout(saveSession, 400); }

// ---------- saving requests ----------

async function save(tab) {
  tab = tab || activeTab();
  if (!tab) return;
  if (tab.origin) {
    const fresh = clone(tab.req);
    for (const k of Object.keys(tab.origin)) delete tab.origin[k];
    Object.assign(tab.origin, fresh);
    persist(true);
    toast("ok", "Saved \"" + tab.req.name + "\"");
    renderAll();
    return;
  }
  saveAs(tab);
}
function containers() {
  const out = [];
  const rec = (c, trail) => {
    const t = trail.concat([c.name]);
    out.push({ c, label: t.join(" / ") });
    (c.folders || []).forEach((f) => rec(f, t));
  };
  (S.ws.collections || []).forEach((c) => rec(c, []));
  return out;
}
function saveAs(tab) {
  const list = containers();
  const name = h("input", { class: "input", value: tab.req.name === "New request" ? "" : tab.req.name, placeholder: "Request name", autofocus: true });
  const sel = h("select", { class: "select" }, list.map((x, i) => h("option", { value: i }, x.label)), h("option", { value: "new" }, "+ New collection…"));
  const newName = h("input", { class: "input", placeholder: "New collection name", style: { display: list.length ? "none" : "" } });
  if (!list.length) sel.value = "new";
  sel.addEventListener("change", () => { newName.style.display = sel.value === "new" ? "" : "none"; });
  const go = () => {
    let target;
    if (sel.value === "new") {
      const n = newName.value.trim() || "My collection";
      target = { name: n, requests: [] };
      S.ws.collections.push(target);
    } else target = list[+sel.value].c;
    tab.req.name = name.value.trim() || tab.req.name || "New request";
    const r = clone(tab.req);
    (target.requests = target.requests || []).push(r);
    tab.origin = r;
    S.open.set(target, true);
    persist(true);
    d.close();
    toast("ok", "Saved \"" + r.name + "\"", "in " + (sel.value === "new" ? target.name : list[+sel.value].label));
    renderAll();
    saveSession();
  };
  name.addEventListener("keydown", (e) => { if (e.key === "Enter") go(); });
  const d = dialog({
    title: "Save request", sub: "Pick where it lives. It's written to your workspace file right away.",
    body: [h("div", null, h("label", { class: "label" }, "Name"), name), h("div", null, h("label", { class: "label" }, "Collection or folder"), sel), newName],
    footer: [btn("Cancel", { onclick: () => d.close() }), btn("Save", { cls: "accent", icon: "save", onclick: go })],
  });
}

// ---------- sidebar ----------

function renderSidebar() {
  const side = $("sidebar");
  side.replaceChildren();
  const hcount = S.history.length;
  side.append(h("div", { class: "side-tabs" },
    h("button", { class: S.sideTab === "collections" ? "on" : "", onclick: () => { S.sideTab = "collections"; renderSidebar(); } }, "Collections"),
    h("button", { class: S.sideTab === "history" ? "on" : "", onclick: () => { S.sideTab = "history"; renderSidebar(); } }, "History", hcount ? h("span", { class: "count" }, hcount) : null)));
  const filter = h("input", { class: "input", placeholder: S.sideTab === "history" ? "Filter history" : "Filter requests", value: S.filter });
  filter.addEventListener("input", () => { S.filter = filter.value; renderTree(); });
  const tools = h("div", { class: "side-tools" }, filter);
  if (S.sideTab === "collections") tools.append(btn("", { cls: "icon", icon: "plus", title: "New collection", onclick: newCollection }));
  else tools.append(btn("", { cls: "icon", icon: "trash", title: "Clear history", onclick: clearHistory, disabled: !hcount }));
  side.append(tools);
  const tree = h("div", { class: "tree", id: "tree" });
  side.append(tree);
  renderTree();
}
function renderTree() {
  const tree = $("tree");
  if (!tree) return;
  tree.replaceChildren();
  if (S.sideTab === "history") return renderHistory(tree);
  const q = S.filter.trim().toLowerCase();
  const tab = activeTab();
  const match = (r) => !q || r.name.toLowerCase().includes(q) || (r.url || "").toLowerCase().includes(q) || label(r).toLowerCase() === q;
  const hasMatch = (c) => (c.requests || []).some(match) || (c.folders || []).some(hasMatch);
  const rec = (c, depth, parentList, isColl) => {
    if (q && !hasMatch(c)) return null;
    const open = q ? true : S.open.has(c) ? S.open.get(c) : isColl;
    const count = countReqs(c);
    const row = h("div", {
      class: "node " + (isColl ? "collection" : "folder"), style: { paddingLeft: 6 + depth * 14 + "px" },
      onclick: () => { S.open.set(c, !open); renderTree(); },
      oncontextmenu: (e) => { e.preventDefault(); containerMenu(e, c, parentList, isColl); },
    }, icon("chev", "icon-svg chev" + (open ? " open" : "")), icon(isColl ? "box" : "folder", "folder-ico"), h("span", { class: "name" }, c.name),
      h("span", { class: "count" }, count),
      moreBtn((b) => containerMenu(b, c, parentList, isColl)));
    const kids = h("div", { class: "children" });
    if (open) {
      (c.folders || []).forEach((f) => { const n = rec(f, depth + 1, c.folders, false); if (n) kids.append(n); });
      (c.requests || []).forEach((r) => {
        if (!match(r)) return;
        kids.append(h("div", {
          class: "node" + (tab && tab.origin === r ? " sel" : ""), style: { paddingLeft: 26 + depth * 14 + "px" },
          title: r.url, onclick: () => openTab(r, r),
          oncontextmenu: (e) => { e.preventDefault(); requestMenu(e, r, c); },
          ondblclick: () => renameRequest(r),
        }, h("span", { class: "method m-" + label(r) }, label(r)), h("span", { class: "name" }, r.name), moreBtn((b) => requestMenu(b, r, c))));
      });
      if (!(c.folders || []).length && !(c.requests || []).length) {
        kids.append(h("div", { class: "hint", style: { padding: "4px 0 6px " + (34 + depth * 14) + "px" } }, "Empty. ", h("a", { href: "#", onclick: (e) => { e.preventDefault(); newRequestIn(c); } }, "Add a request")));
      }
    }
    return h("div", null, row, kids);
  };
  const cols = S.ws.collections || [];
  cols.forEach((c) => { const n = rec(c, 0, cols, true); if (n) tree.append(n); });
  if (!cols.length) {
    tree.append(h("div", { class: "empty-side" }, h("div", null, "No collections yet."), h("div", null, "Save a request, or import from cURL or Postman."),
      btn("Import", { icon: "download", onclick: importDialog })));
  } else if (q && !tree.children.length) tree.append(h("div", { class: "empty-side" }, "Nothing matches \"" + S.filter + "\"."));
}
function countReqs(c) { return (c.requests || []).length + (c.folders || []).reduce((n, f) => n + countReqs(f), 0); }
function moreBtn(fn) {
  const b = h("button", { class: "btn ghost sm icon more", title: "More", type: "button" }, icon("more"));
  b.addEventListener("click", (e) => { e.stopPropagation(); b.classList.add("open"); fn(b); });
  return b;
}
function containerMenu(anchor, c, parentList, isColl) {
  const items = [
    { label: "New request", icon: "plus", run: () => newRequestIn(c) },
    { label: "New folder", icon: "folder", run: async () => {
      const n = await ask("New folder", { placeholder: "Folder name", ok: "Create" });
      if (!n) return;
      (c.folders = c.folders || []).push({ name: n, requests: [] });
      S.open.set(c, true);
      persist(true); renderTree();
    } },
    { label: "Run all requests", icon: "play", run: () => runFolder(c) },
    "-",
    { label: "Rename", icon: "edit", run: async () => {
      const n = await ask("Rename " + (isColl ? "collection" : "folder"), { value: c.name });
      if (!n) return;
      c.name = n; persist(true); renderAll();
    } },
    { label: "Export to Postman", icon: "upload", run: () => exportContainer(c) },
    "-",
    { label: "Delete", icon: "trash", danger: true, run: async () => {
      const n = countReqs(c);
      if (!(await confirmBox("Delete \"" + c.name + "\"?", n ? `It holds ${n} request${n === 1 ? "" : "s"}. This can't be undone.` : "It's empty."))) return;
      parentList.splice(parentList.indexOf(c), 1);
      S.tabs.forEach((t) => { if (t.origin && !trailOf(t.origin)) t.origin = null; });
      persist(true); renderAll();
    } },
  ];
  popover(anchor.clientX != null ? { x: anchor.clientX, y: anchor.clientY } : anchor, items);
}
function requestMenu(anchor, r, c) {
  popover(anchor.clientX != null ? { x: anchor.clientX, y: anchor.clientY } : anchor, [
    { label: "Open", icon: "chev", run: () => openTab(r, r) },
    { label: "Open in new tab (unsaved copy)", icon: "dup", run: () => { const t = openTab(Object.assign(clone(r), { name: r.name + " (copy)" }), null); t.origin = null; } },
    "-",
    { label: "Rename", icon: "edit", run: () => renameRequest(r) },
    { label: "Duplicate", icon: "dup", run: () => {
      const d = clone(r); d.name = r.name + " copy";
      c.requests.splice(c.requests.indexOf(r) + 1, 0, d);
      persist(true); renderTree(); openTab(d, d);
    } },
    { label: "Copy as cURL", icon: "copy", run: () => copyCode(r, "curl") },
    "-",
    { label: "Delete", icon: "trash", danger: true, run: async () => {
      if (!(await confirmBox("Delete \"" + r.name + "\"?", "This can't be undone."))) return;
      c.requests.splice(c.requests.indexOf(r), 1);
      S.tabs.forEach((t) => { if (t.origin === r) t.origin = null; });
      persist(true); renderAll();
    } },
  ]);
}
async function renameRequest(r) {
  const n = await ask("Rename request", { value: r.name });
  if (!n) return;
  r.name = n;
  S.tabs.forEach((t) => { if (t.origin === r) t.req.name = n; });
  persist(true); renderAll();
}
function newRequestIn(c) {
  const r = newRequest();
  (c.requests = c.requests || []).push(r);
  S.open.set(c, true);
  persist(true);
  openTab(r, r);
  setTimeout(() => { const u = $("url"); if (u) u.focus(); }, 30);
}
async function newCollection() {
  const n = await ask("New collection", { placeholder: "e.g. Payments API", ok: "Create" });
  if (!n) return;
  const c = { name: n, requests: [] };
  S.ws.collections.push(c);
  S.open.set(c, true);
  persist(true); renderTree();
}
async function exportContainer(c) {
  // The export API takes the index path of the collection and folders.
  const path = [];
  const find = (list, target, p) => {
    for (let i = 0; i < list.length; i++) {
      if (list[i] === target) { path.push(...p, i); return true; }
      if (find(list[i].folders || [], target, p.concat([i]))) return true;
    }
    return false;
  };
  find(S.ws.collections, c, []);
  try {
    await saveChain;
    const res = await api("POST", "/api/export", { folders: path });
    download(res.filename, res.data, "application/json");
    toast("ok", "Exported " + res.filename, (res.notes || []).join(" ") || "Import it in Postman with File → Import.");
  } catch (e) { toast("err", "Export failed", e.message); }
}

function renderHistory(tree) {
  const q = S.filter.trim().toLowerCase();
  if (!S.history.length) {
    tree.append(h("div", { class: "empty-side" }, icon("clock"), h("div", null, "Nothing sent yet."), h("div", null, "Every request you send shows up here with its response.")));
    return;
  }
  let day = "";
  S.history.forEach((e, i) => {
    if (q && !(e.url || "").toLowerCase().includes(q) && !(e.method || "").toLowerCase().includes(q) && !(e.source || "").toLowerCase().includes(q)) return;
    const d = new Date(e.time);
    const label2 = d.toDateString() === new Date().toDateString() ? "Today" : d.toDateString() === new Date(Date.now() - 864e5).toDateString() ? "Yesterday" : d.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
    if (label2 !== day) { day = label2; tree.append(h("div", { class: "hist-day" }, day)); }
    let path = e.url;
    try { const u = new URL(e.url); path = u.pathname + u.search; } catch (err) { /* keep */ }
    const m = e.request && e.request.type === "websocket" ? "WS" : e.method;
    const del = btn("", { cls: "ghost sm icon more", icon: "x", title: "Remove from history" });
    del.addEventListener("click", async (ev) => {
      ev.stopPropagation();
      try { S.history = (await api("DELETE", "/api/history/" + i)).history || []; renderSidebar(); } catch (err) { toast("err", "Could not delete", err.message); }
    });
    tree.append(h("div", { class: "hist" + (S.selHist === i ? " sel" : ""), title: (e.source ? e.source + "\n" : "") + e.url + "\n" + d.toLocaleString(), onclick: () => openHistory(e, i) },
      h("span", { class: "method m-" + m }, m),
      h("span", { class: "status-dot " + (e.error ? "serr" : statusClass(e.status)) }, e.error ? "ERR" : e.status || "·"),
      h("span", { class: "path" }, path),
      h("span", { class: "row", style: { gap: "2px" } }, h("span", { class: "time" }, ago(e.time)), del)));
  });
}
function openHistory(e, i) {
  S.selHist = i;
  const origin = (() => { let found = null; if (e.source) walk((r, t) => { if (t.map((c) => c.name).concat([r.name]).join(" / ") === e.source && !found) found = r; }); return found; })();
  const req = normReq(clone(e.request));
  const result = e.error ? { kind: "http", error: e.error, env: e.environment, requestHeaders: e.request_headers, method: e.method, url: e.url, fromHistory: e.time }
    : {
      kind: "http", status: e.status, statusText: e.status_text, proto: e.proto, headers: e.response_headers || {}, body: e.bodyText || "", base64: e.bodyBase64,
      size: e.body_size, truncated: e.body_truncated, durationMs: e.duration_ms, url: e.final_url || e.url, method: e.method, env: e.environment,
      requestHeaders: e.request_headers, tests: e.tests || [], fromHistory: e.time,
    };
  const tab = { id: ++tabSeq, req, origin: null, result, reqTab: null, respTab: null, bodyView: "pretty", hdrView: "response" };
  if (origin && canon(origin) === canon(req)) tab.origin = origin;
  S.tabs.push(tab);
  activate(tab);
}
async function clearHistory() {
  if (!(await confirmBox("Clear history?", `Removes all ${S.history.length} entries from the history file.`, "Clear"))) return;
  try { S.history = (await api("DELETE", "/api/history")).history || []; S.selHist = -1; renderSidebar(); toast("ok", "History cleared"); } catch (e) { toast("err", "Could not clear history", e.message); }
}

// ---------- environments ----------

function renderEnvPick() {
  const box = $("envPick");
  box.replaceChildren();
  const envs = S.ws.environments || [];
  const sel = h("select", { class: "select", style: { width: "180px", height: "32px" }, title: "Active environment" },
    h("option", { value: "" }, "No environment"), envs.map((e) => h("option", { value: e.name }, e.name)));
  sel.value = S.ws.active_environment || "";
  sel.addEventListener("change", () => { S.ws.active_environment = sel.value; persist(true); renderAll(); toast("info", sel.value ? "Using " + sel.value : "No environment", "Globals still apply."); });
  box.append(icon("globe", "icon-svg"), sel, btn("", { cls: "ghost icon", icon: "layers", title: "Variables and environments", onclick: () => envDialog() }));
  box.firstChild.style.color = "var(--dim)";
}
function kvEditor(list, onChange, opts = {}) {
  // list is edited in place; a trailing ghost row adds entries.
  const wrap = h("div", { class: "kv" });
  const draw = () => {
    wrap.replaceChildren(h("div", { class: "kv-row head" }, h("div"), h("div", null, opts.k || "Key"), h("div", null, opts.v || "Value"), h("div")));
    const rows = list.concat([{ key: "", value: "", ghost: true }]);
    rows.forEach((kv, i) => {
      const cb = h("input", { type: "checkbox", class: "checkbox", checked: !kv.disabled && !kv.ghost, disabled: kv.ghost, title: "Send this one" });
      const k = h("input", { type: "text", value: kv.key, placeholder: kv.ghost ? "Add " + (opts.noun || "key") : "", spellcheck: "false" });
      const v = h("input", { type: "text", value: kv.value, placeholder: kv.ghost ? "Value" : "", spellcheck: "false" });
      const del = btn("", { cls: "ghost sm icon del", icon: "x", title: "Remove", onclick: () => { list.splice(i, 1); draw(); onChange(); } });
      const row = h("div", { class: "kv-row" + (kv.disabled ? " off" : "") + (kv.ghost ? " ghost" : "") }, h("div", { class: "cbx" }, cb), k, v, kv.ghost ? h("div") : h("div", { class: "cbx" }, del));
      const upd = () => {
        if (kv.ghost) {
          if (!k.value && !v.value) return;
          delete kv.ghost;
          list.push(kv);
          kv.key = k.value; kv.value = v.value;
          const focus = document.activeElement === k ? 1 : 2;
          draw();
          const r = wrap.children[list.length];
          const f = r && r.children[focus];
          if (f) { f.focus(); f.setSelectionRange(f.value.length, f.value.length); }
          onChange();
          return;
        }
        kv.key = k.value; kv.value = v.value; onChange();
      };
      k.addEventListener("input", upd);
      v.addEventListener("input", upd);
      cb.addEventListener("change", () => { kv.disabled = !cb.checked; row.classList.toggle("off", kv.disabled); onChange(); });
      wrap.append(row);
    });
  };
  draw();
  return wrap;
}
function envDialog(focusName) {
  let sel = focusName != null ? focusName : (S.ws.active_environment || null); // null = Globals
  const list = h("div", { class: "env-list" });
  const right = h("div", { style: { display: "flex", flexDirection: "column", gap: "12px", minWidth: 0 } });
  const draw = () => {
    list.replaceChildren(
      h("div", { class: "env-item" + (sel === null ? " sel" : ""), onclick: () => { sel = null; draw(); } }, icon("globe"), "Globals"),
      h("div", { class: "hist-day" }, "Environments"),
      (S.ws.environments || []).map((e) => h("div", { class: "env-item" + (sel === e.name ? " sel" : ""), onclick: () => { sel = e.name; draw(); } },
        icon("layers"), h("span", { class: "grow", style: { overflow: "hidden", textOverflow: "ellipsis" } }, e.name), S.ws.active_environment === e.name ? h("span", { class: "on", title: "Active" }) : null)),
      btn("New environment", { cls: "ghost sm", icon: "plus", onclick: async () => {
        const n = await ask("New environment", { placeholder: "e.g. Staging", ok: "Create" });
        if (!n) return;
        if ((S.ws.environments || []).some((e) => e.name === n)) return toast("warn", "\"" + n + "\" already exists");
        (S.ws.environments = S.ws.environments || []).push({ name: n, variables: [] });
        sel = n; persist(true); draw(); renderEnvPick();
      } }));
    right.replaceChildren();
    const env = sel === null ? null : (S.ws.environments || []).find((e) => e.name === sel);
    if (sel !== null && !env) { sel = null; return draw(); }
    const listRef = env ? (env.variables = env.variables || []) : (S.ws.variables = S.ws.variables || []);
    right.append(h("div", { class: "row" },
      h("div", { class: "grow" }, h("div", { style: { fontWeight: 650, fontSize: "14px" } }, env ? env.name : "Globals"),
        h("div", { class: "hint" }, env ? (S.ws.active_environment === env.name ? "Active. Its values override globals with the same name." : "Not active.") : "Available in every environment.")),
      env && S.ws.active_environment !== env.name ? btn("Use this one", { cls: "sm accent", onclick: () => { S.ws.active_environment = env.name; persist(true); draw(); renderAll(); } }) : null,
      env ? btn("", { cls: "sm ghost icon", icon: "edit", title: "Rename", onclick: async () => {
        const n = await ask("Rename environment", { value: env.name });
        if (!n || n === env.name) return;
        if (S.ws.active_environment === env.name) S.ws.active_environment = n;
        env.name = n; sel = n; persist(true); draw(); renderEnvPick();
      } }) : null,
      env ? btn("", { cls: "sm ghost icon danger", icon: "trash", title: "Delete", onclick: async () => {
        if (!(await confirmBox("Delete \"" + env.name + "\"?", "Its variables go with it."))) return;
        S.ws.environments.splice(S.ws.environments.indexOf(env), 1);
        if (S.ws.active_environment === env.name) S.ws.active_environment = "";
        sel = null; persist(true); draw(); renderAll();
      } }) : null));
    right.append(kvEditor(listRef, () => { persist(); renderUrlHL(); }, { k: "Variable", v: "Value", noun: "variable" }));
    right.append(h("div", { class: "hint" }, "Use them anywhere as ", h("code", null, "{{name}}"), ". Unticked variables are kept but not used. Tests can capture values with ", h("code", null, "set token = json.token"), "."));
  };
  draw();
  dialog({ title: "Variables", sub: "Globals and environments, saved to your workspace as you type.", wide: true, body: h("div", { class: "env-layout" }, list, right), onclose: () => renderAll() });
}

// ---------- request editor ----------

function renderOpenTabs() {
  const bar = $("openTabs");
  bar.replaceChildren();
  for (const t of S.tabs) {
    const busy = t.sending || (t.sock && t.sock.state !== "closed");
    const el = h("div", {
      class: "otab" + (t.id === S.active ? " on" : "") + (isDirty(t) ? " dirtyTab" : ""), title: (sourceOf(t) || "Unsaved") + "\n" + t.req.url,
      onclick: () => activate(t), onauxclick: (e) => { if (e.button === 1) closeTab(t); },
    }, h("span", { class: "method m-" + label(t.req) }, label(t.req)), h("span", { class: "name" }, t.req.name || "Untitled"),
      busy ? h("span", { class: "spinner busy" }) : null,
      h("span", { class: "dirty", title: "Unsaved changes" }),
      h("button", { class: "x", title: "Close", type: "button", onclick: (e) => { e.stopPropagation(); closeTab(t); } }, icon("x")));
    bar.append(el);
  }
  bar.append(h("button", { class: "newtab", title: "New request (" + MOD + "+Alt+N)", onclick: () => openTab(newRequest()) }, icon("plus")));
  const on = bar.querySelector(".otab.on");
  if (on) on.scrollIntoView({ block: "nearest", inline: "nearest" });
}

function renderRequest() {
  const pane = $("reqPane");
  pane.replaceChildren();
  const tab = activeTab();
  if (!tab) return;
  const r = tab.req;
  const ws = r.type === "websocket";
  const trail = tab.origin && trailOf(tab.origin);

  const title = h("input", { class: "title", value: r.name, spellcheck: "false", size: Math.max(8, r.name.length + 1) });
  title.addEventListener("input", () => { r.name = title.value; title.size = Math.max(8, title.value.length + 1); changed(tab); });
  const crumbs = h("div", { class: "crumbs" },
    trail ? trail.map((c) => [h("span", null, c.name), h("span", { class: "sep" }, "/")]) : h("span", { class: "hint" }, "Unsaved /"),
    title, h("span", { class: "dirty", title: "Unsaved changes", style: { visibility: isDirty(tab) ? "visible" : "hidden" } }),
    h("div", { class: "grow" }),
    btn("Code", { cls: "ghost sm", icon: "code", title: "Code snippet: curl, Python, JavaScript, Go, HTTPie", onclick: () => codeDialog(tab) }),
    btn("Save", { cls: "sm", icon: "save", title: "Save (" + MOD + "+S)", onclick: () => save(tab) }),
    (() => { const b = btn("", { cls: "ghost sm icon", icon: "more", title: "More" }); b.onclick = () => popover(b, [
      { label: "Save as…", icon: "save", run: () => saveAs(tab) },
      { label: "Duplicate into a new tab", icon: "dup", run: () => { const t = openTab(Object.assign(clone(r), { name: r.name + " (copy)" })); t.origin = null; renderAll(); } },
      { label: "Copy as cURL", icon: "copy", run: () => copyCode(r, "curl") },
      tab.origin ? { label: "Revert to saved", icon: "clock", run: () => { tab.req = normReq(clone(tab.origin)); renderAll(); } } : null,
    ].filter(Boolean), { alignRight: true }); return b; })());

  // Method picker.
  const mp = h("div", { class: "method-pick m-" + label(r), title: "Method, or WebSocket" }, label(r), icon("chev", "icon-svg"));
  mp.lastChild.style.transform = "rotate(90deg)";
  mp.lastChild.style.width = "12px";
  mp.addEventListener("click", () => popover(mp, [{ heading: "HTTP" }].concat(METHODS.map((m) => ({
    html: `<span class="method m-${m}" style="text-align:left">${m}</span>`, run: () => { r.type = ""; r.method = m; renderAll(); },
  })), ["-", { heading: "Realtime" }, { html: '<span class="method m-WS" style="text-align:left">WS</span> WebSocket', run: () => {
    r.type = "websocket";
    if (/^https?:/.test(r.url)) r.url = r.url.replace(/^http/, "ws");
    renderAll();
  } }])));
  const url = h("input", { id: "url", value: r.url, spellcheck: "false", autocomplete: "off", placeholder: ws ? "wss://echo.websocket.events or {{baseUrl}}/ws" : "https://api.example.com/users or paste a curl command" });
  const hl = h("div", { class: "url-hl", id: "urlhl" });
  url.addEventListener("input", () => { r.url = url.value; renderUrlHL(); changed(tab); });
  url.addEventListener("scroll", () => { hl.scrollLeft = url.scrollLeft; });
  url.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); send(tab); } });
  url.addEventListener("paste", (e) => {
    const text = (e.clipboardData || window.clipboardData).getData("text");
    if (/^\s*curl\s/.test(text)) { e.preventDefault(); importCurlInto(tab, text); }
  });
  const sendBtn = h("button", { class: "send", id: "sendBtn", onclick: () => send(tab) });
  paintSend(tab, sendBtn);
  pane.append(h("div", { class: "req-head" }, crumbs, h("div", { class: "urlbar" }, h("div", { class: "urlbox" }, mp, h("div", { class: "url-field" }, hl, url)), sendBtn)));

  // Section tabs.
  const enabled = (l) => l.filter((x) => !x.disabled && x.key).length;
  const sections = [
    ["params", "Params", enabled(r.params) || null],
    ["headers", "Headers", enabled(r.headers) || null],
    ["auth", "Auth", r.auth.type !== "none" ? "dot" : null],
    ["body", ws ? "Message" : "Body", r.body_type !== "none" && !ws ? "dot" : ws && r.body ? "dot" : null],
    ["pre", "Pre-request", r.pre_request.trim() ? "dot" : null],
    ["tests", "Tests", r.tests.trim() ? r.tests.split("\n").filter((l) => l.trim() && !l.trim().startsWith("#")).length : null],
  ];
  if (!tab.reqTab) tab.reqTab = r.body_type !== "none" || (ws && r.body) ? "body" : enabled(r.params) ? "params" : "params";
  const tabs = h("div", { class: "tabs" }, sections.map(([id, name, badge]) => h("button", { class: tab.reqTab === id ? "on" : "", onclick: () => { tab.reqTab = id; renderRequest(); } },
    name, badge === "dot" ? h("span", { class: "dotb" }) : badge ? h("span", { class: "badge" }, badge) : null)));
  const body = h("div", { class: "tab-body" });
  pane.append(tabs, body);
  const refreshTabs = () => { changed(tab); };
  switch (tab.reqTab) {
    case "params":
      body.append(kvEditor(r.params, refreshTabs, { k: "Query parameter", noun: "parameter" }),
        h("div", { class: "hint" }, "Added to the URL's query string when sent. ", h("code", null, "{{variables}}"), " work in keys and values."));
      break;
    case "headers":
      body.append(kvEditor(r.headers, refreshTabs, { k: "Header", noun: "header" }),
        h("div", { class: "hint" }, "Content-Type is set from the body type unless you set it here. Auth headers come from the Auth tab."));
      break;
    case "auth": body.append(authEditor(tab)); break;
    case "body": body.append(ws ? wsMessageEditor(tab) : bodyEditor(tab)); break;
    case "pre":
      body.append(scriptEditor(tab, "pre_request", "# runs before the request is sent\nset orderId = order-{{$randomInt}}\nset auth = Bearer {{token}}",
        [["set name = value", "Set a variable (where it lives already, else the active environment)"], ["unset name", "Remove a variable"], ["{{$randomInt}}", "Also {{$guid}}, {{$timestamp}}"]]));
      break;
    case "tests":
      body.append(scriptEditor(tab, "tests", "status == 200\njson.data.id exists\ntime < 1000\nset token = json.token",
        ws ? [["events >= 1", "Messages received"], ["event[-1].json.type == pong", "Look inside the last message"], ["event[0] contains \"hello\"", "First message"]]
          : [["status == 200", "Also <, >=, !=…"], ["header Content-Type contains json", "Headers by name"], ["json.items.length > 0", "JSON paths"],
            ["body contains \"ok\"", "Raw body"], ["time < 500", "Milliseconds"], ["set token = json.token", "Capture into a variable"], ["events >= 3", "Streams: event count"]]));
      break;
  }
  renderUrlHL();
}
function paintSend(tab, b) {
  b = b || $("sendBtn");
  if (!b) return;
  b.replaceChildren();
  b.className = "send";
  const ws = tab.req.type === "websocket";
  if (ws && tab.sock && tab.sock.state === "open") { b.classList.add("stop"); b.append(icon("x"), "Disconnect"); }
  else if (ws && tab.sock && tab.sock.state === "connecting") { b.classList.add("stop"); b.append(h("span", { class: "spinner busy", style: { borderTopColor: "#fff" } }), "Cancel"); }
  else if (tab.sending) { b.classList.add("stop"); b.append(icon("stop"), "Stop"); }
  else if (ws) b.append(icon("plug"), "Connect");
  else b.append(icon("send"), "Send");
  b.title = ws ? "Connect (" + MOD + "+Enter)" : "Send (" + MOD + "+Enter)";
}
function renderUrlHL() {
  const tab = activeTab();
  const hl = $("urlhl");
  if (!tab || !hl) return;
  hl.innerHTML = varSpans(tab.req.url, vars()) + " ";
  const u = $("url");
  if (u) hl.scrollLeft = u.scrollLeft;
}
function authEditor(tab) {
  const a = tab.req.auth;
  const wrap = h("div", { style: { display: "flex", flexDirection: "column", gap: "16px" } });
  const draw = () => {
    wrap.replaceChildren();
    const seg = h("div", { class: "seg" }, AUTH_TYPES.map(([id, name]) => h("button", { class: a.type === id ? "on" : "", onclick: () => { a.type = id; draw(); changed(tab); renderRequestTabsOnly(); } }, name)));
    wrap.append(seg);
    const field = (labelText, key, opts = {}) => {
      const i = h("input", { class: "input mono", value: a[key] || "", type: opts.secret ? "password" : "text", placeholder: opts.ph || "", spellcheck: "false" });
      i.addEventListener("input", () => { a[key] = i.value; changed(tab); });
      const row = [h("label", { class: "label", style: { margin: 0 } }, labelText)];
      if (opts.secret) {
        const eye = btn("", { cls: "ghost sm icon", icon: "search", title: "Show or hide" });
        eye.onclick = () => { i.type = i.type === "password" ? "text" : "password"; };
        row.push(h("div", { class: "row" }, i, eye));
      } else row.push(i);
      return row;
    };
    if (a.type === "none") wrap.append(h("div", { class: "help" }, "This request sends no credentials. Pick Basic, Bearer or API key to add them; values may use ", h("code", null, "{{variables}}"), "."));
    if (a.type === "basic") wrap.append(h("div", { class: "auth-grid" }, field("Username", "username"), field("Password", "password", { secret: true })));
    if (a.type === "bearer") wrap.append(h("div", { class: "auth-grid" }, field("Token", "token", { ph: "{{token}}", secret: true })), h("div", { class: "hint" }, "Sent as Authorization: Bearer <token>."));
    if (a.type === "apikey") {
      const where = h("div", { class: "seg" }, [["header", "Header"], ["query", "Query parameter"]].map(([id, n]) => h("button", { class: (a.in || "header") === id ? "on" : "", onclick: () => { a.in = id; draw(); changed(tab); } }, n)));
      wrap.append(h("div", { class: "auth-grid" }, field("Key", "key", { ph: "X-API-Key" }), field("Value", "value", { ph: "{{apiKey}}", secret: true }), h("label", { class: "label", style: { margin: 0 } }, "Add to"), where));
    }
  };
  draw();
  return wrap;
}
function renderRequestTabsOnly() {
  // Re-render the section tab labels (badges) without touching the editors.
  const tab = activeTab();
  if (!tab) return;
  const keep = tab.reqTab;
  const scroll = document.querySelector("#reqPane .tab-body");
  const st = scroll ? scroll.scrollTop : 0;
  const focusId = document.activeElement && document.activeElement.id;
  renderRequest();
  tab.reqTab = keep;
  const nb = document.querySelector("#reqPane .tab-body");
  if (nb) nb.scrollTop = st;
  if (focusId && $(focusId)) $(focusId).focus();
}
function textArea(value, onInput, opts = {}) {
  const ta = h("textarea", { class: "textarea fill", spellcheck: "false", placeholder: opts.placeholder || "", id: opts.id });
  ta.value = value || "";
  ta.addEventListener("input", () => onInput(ta.value));
  ta.addEventListener("keydown", (e) => {
    if (e.key === "Tab" && !e.shiftKey) {
      e.preventDefault();
      const s = ta.selectionStart, en = ta.selectionEnd;
      ta.setRangeText("  ", s, en, "end");
      onInput(ta.value);
    }
  });
  return ta;
}
function bodyEditor(tab) {
  const r = tab.req;
  const wrap = h("div", { style: { display: "flex", flexDirection: "column", gap: "12px", flex: 1, minHeight: 0 } });
  const seg = h("div", { class: "seg" }, BODY_TYPES.map(([id, name]) => h("button", {
    class: r.body_type === id ? "on" : "", onclick: () => {
      if (id === "graphql" && r.method === "GET") r.method = "POST";
      r.body_type = id; changed(tab); renderRequestTabsOnly();
    },
  }, name)));
  const tools = h("div", { class: "row", style: { flexWrap: "wrap" } }, seg, h("div", { class: "grow" }));
  wrap.append(tools);
  if (r.body_type === "none") {
    wrap.append(h("div", { class: "help" }, "No body. Pick JSON, Text, XML, Form (urlencoded) or GraphQL to send one. ", r.method === "GET" ? "GET requests usually don't have one." : ""));
    return wrap;
  }
  if (r.body_type === "form") {
    const list = parseKV(r.body, "=");
    wrap.append(kvEditor(list, () => { r.body = formatKV(list, "="); changed(tab); }, { k: "Field", noun: "field" }),
      h("div", { class: "hint" }, "Sent as application/x-www-form-urlencoded."));
    return wrap;
  }
  if (r.body_type === "graphql") {
    tools.append(btn("Schema", { cls: "sm", icon: "schema", title: "Browse the schema and insert queries", onclick: () => schemaDialog(tab) }),
      btn("Prettify", { cls: "sm ghost", title: "Format the variables JSON", icon: "wand", onclick: () => {
        const v = tryJSON(r.graphql_variables);
        if (v === undefined) return toast("warn", "Variables aren't valid JSON");
        r.graphql_variables = JSON.stringify(v, null, 2); renderRequestTabsOnly(); changed(tab);
      } }));
    const q = textArea(r.body, (v) => { r.body = v; changed(tab); }, { id: "gqlQuery", placeholder: "query {\n  countries(continent: \"EU\") {\n    name\n    capital\n  }\n}\n\nOpen Schema to browse fields and insert a query." });
    const v = textArea(r.graphql_variables, (val) => { r.graphql_variables = val; changed(tab); }, { id: "gqlVars", placeholder: "{\n  \"code\": \"IN\"\n}\n\nOptional: a JSON object with the query's $variables." });
    wrap.append(h("div", { class: "split2" }, h("div", null, h("label", { class: "label" }, "Query"), q), h("div", null, h("label", { class: "label" }, "Variables · JSON"), v)),
      h("div", { class: "hint" }, r.method === "GET" ? "GET sends the query in the URL." : "Sent as a JSON POST: {query, variables}."));
    return wrap;
  }
  if (r.body_type === "json") {
    tools.append(btn("Prettify", { cls: "sm ghost", icon: "wand", title: "Format JSON", onclick: () => {
      const v = tryJSON(r.body);
      if (v === undefined) return toast("warn", "The body isn't valid JSON", "{{variables}} outside quotes can't be formatted.");
      r.body = JSON.stringify(v, null, 2); renderRequestTabsOnly(); changed(tab);
    } }));
  }
  const status = h("span", { class: "hint" });
  const check = () => {
    if (r.body_type !== "json" || !r.body.trim()) { status.textContent = ""; return; }
    try { JSON.parse(r.body.replace(/\{\{[^{}]+\}\}/g, "0")); status.textContent = "Valid JSON"; status.style.color = "var(--ok)"; } catch (e) { status.textContent = e.message.replace(/^JSON\.parse: /, ""); status.style.color = "var(--warn)"; }
  };
  tools.insertBefore(status, tools.children[2] || null);
  const ph = { json: '{\n  "name": "Ada",\n  "email": "{{email}}"\n}', text: "Plain text", xml: "<user>\n  <name>Ada</name>\n</user>" }[r.body_type];
  wrap.append(textArea(r.body, (v) => { r.body = v; check(); changed(tab); }, { id: "bodyText", placeholder: ph }));
  check();
  return wrap;
}
function wsMessageEditor(tab) {
  const r = tab.req;
  return h("div", { style: { display: "flex", flexDirection: "column", gap: "12px", flex: 1 } },
    h("div", { class: "help" }, "This message is sent as soon as the connection opens (leave it empty to send nothing). After connecting, type more messages in the composer under the response."),
    textArea(r.body, (v) => { r.body = v; r.body_type = v ? "text" : "none"; changed(tab); }, { id: "bodyText", placeholder: '{"type": "subscribe", "channel": "ticker"}' }));
}
function scriptEditor(tab, key, placeholder, snippets) {
  const r = tab.req;
  const ta = textArea(r[key], (v) => { r[key] = v; changed(tab); }, { placeholder, id: "script-" + key });
  const help = h("div", { class: "help" }, h("div", { style: { marginBottom: "6px" } }, "One instruction per line; # starts a comment. Click to insert:"),
    h("div", { style: { display: "flex", flexWrap: "wrap", gap: "6px 14px" } }, snippets.map(([code, what]) => h("span", null,
      h("code", { title: "Insert", onclick: () => { const v = ta.value; ta.value = v + (v && !v.endsWith("\n") ? "\n" : "") + code; r[key] = ta.value; changed(tab); ta.focus(); } }, code), " ", h("span", { class: "hint" }, what)))));
  return h("div", { style: { display: "flex", flexDirection: "column", gap: "12px", flex: 1, minHeight: 0 } }, ta, help);
}

// ---------- sending ----------

async function send(tab) {
  tab = tab || activeTab();
  if (!tab) return;
  if (tab.req.type === "websocket") return toggleSocket(tab);
  if (tab.sending) return stopTab(tab);
  if (!tab.req.url.trim()) { toast("warn", "Enter a URL first"); $("url") && $("url").focus(); return; }
  const id = Math.random().toString(36).slice(2) + Date.now().toString(36);
  const ctrl = new AbortController();
  tab.sending = { id, ctrl };
  tab.result = { kind: "http", live: true, events: [], started: performance.now() };
  tab.respTab = null;
  renderOpenTabs(); paintSend(tab); renderResponse();
  const ticker = setInterval(() => { if (tab === activeTab() && tab.result && tab.result.live) paintRespHead(tab); }, 100);
  let finished = false;
  try {
    const res = await fetch("/api/send", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ request: tab.req, source: sourceOf(tab), id }), signal: ctrl.signal });
    if (!res.ok) {
      let msg = res.status + " " + res.statusText;
      try { msg = (await res.json()).error || msg; } catch (e) { /* not JSON */ }
      throw new Error(msg);
    }
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      let i;
      while ((i = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, i);
        buf = buf.slice(i + 1);
        if (line.trim()) { onSendEvent(tab, JSON.parse(line)); if (/"kind":"(done|error)"/.test(line)) finished = true; }
      }
    }
    if (!finished) throw new Error("the connection to term-rest-client closed early");
  } catch (e) {
    if (!finished) {
      const r = tab.result || {};
      if (e.name === "AbortError") { r.error = r.error || "Stopped"; r.stopped = true; } else r.error = e.message;
      r.live = false;
      tab.result = r;
    }
  } finally {
    clearInterval(ticker);
    tab.sending = null;
    if (tab.result) tab.result.live = false;
    renderOpenTabs(); paintSend(tab);
    if (tab === activeTab()) renderResponse();
  }
}
function onSendEvent(tab, ev) {
  const r = tab.result;
  switch (ev.kind) {
    case "start":
      Object.assign(r, { method: ev.method, url: ev.url, env: ev.env, missing: ev.missing || [], warnings: ev.warnings || [], requestHeaders: ev.requestHeaders || {} });
      break;
    case "head":
      Object.assign(r, { status: ev.status, statusText: ev.statusText, proto: ev.proto, headers: ev.headers || {}, stream: ev.stream });
      if (tab === activeTab()) { tab.respTab = "events"; renderResponse(); }
      return;
    case "events":
      r.events.push(...ev.events);
      r.size = ev.size;
      r.elapsed = ev.elapsedMs;
      if (tab === activeTab() && (tab.respTab || "events") === "events") appendEvents(tab, ev.events);
      else if (tab === activeTab()) paintRespHead(tab);
      return;
    case "done": {
      const live = r;
      tab.result = Object.assign({ kind: "http", method: live.method, env: live.env, missing: live.missing, warnings: live.warnings, requestHeaders: live.requestHeaders }, ev.response, { tests: ev.tests || [] });
      if (ev.history) { S.history = ev.history; if (S.sideTab === "history") renderTree(); }
      if (ev.workspace) { replaceWorkspace(ev.workspace); renderEnvPick(); renderUrlHL(); }
      const t = tab.result.tests;
      if (t.length) {
        const failed = t.filter((x) => !x.passed).length;
        if (failed) toast("err", failed + " of " + t.length + " tests failed");
      }
      break;
    }
    case "error":
      Object.assign(r, { error: ev.error, env: ev.env, live: false });
      if (ev.history) { S.history = ev.history; if (S.sideTab === "history") renderTree(); }
      break;
  }
  if (tab === activeTab()) renderResponse();
}
function stopTab(tab) {
  if (tab.sending) {
    api("POST", "/api/stop", { id: tab.sending.id }).then((r) => { if (!r.stopped && tab.sending) tab.sending.ctrl.abort(); }).catch(() => tab.sending && tab.sending.ctrl.abort());
  }
  if (tab.sock && tab.sock.conn) closeSocket(tab);
}

// ---------- WebSocket ----------

function toggleSocket(tab) {
  if (tab.sock && tab.sock.state !== "closed") return closeSocket(tab);
  if (!tab.req.url.trim()) { toast("warn", "Enter a ws:// or wss:// URL first"); return; }
  const conn = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/api/socket");
  const sock = { conn, state: "connecting", started: Date.now() };
  tab.sock = sock;
  tab.result = { kind: "ws", state: "connecting", messages: [], tests: [] };
  tab.respTab = "messages";
  conn.onopen = () => conn.send(JSON.stringify({ type: "connect", request: tab.req }));
  conn.onmessage = (m) => onSocketEvent(tab, sock, JSON.parse(m.data));
  conn.onclose = () => {
    if (sock.state !== "closed") {
      sock.state = "closed";
      if (tab.sock === sock && tab.result && tab.result.state !== "closed") { tab.result.state = "closed"; tab.result.how = tab.result.how || "connection to term-rest-client closed"; }
      refreshSock(tab);
    }
  };
  refreshSock(tab);
}
function onSocketEvent(tab, sock, ev) {
  const r = tab.result;
  if (tab.sock !== sock) return;
  switch (ev.type) {
    case "connected":
      sock.state = "open";
      Object.assign(r, { state: "open", url: ev.url, env: ev.env, missing: ev.missing || [], requestHeaders: ev.requestHeaders || {}, status: ev.status, headers: ev.headers || {}, protocol: ev.protocol, opened: Date.now() });
      break;
    case "message":
      r.messages.push(ev);
      if (tab === activeTab() && tab.respTab === "messages") { appendMessage(tab, ev); paintRespHead(tab); return; }
      break;
    case "closed":
      sock.state = "closed";
      Object.assign(r, { state: "closed", how: ev.how, durationMs: ev.durationMs, tests: ev.tests || [] });
      try { sock.conn.close(); } catch (e) { /* closed */ }
      break;
    case "error":
      if (sock.state === "connecting") { sock.state = "closed"; r.state = "closed"; r.error = ev.error; try { sock.conn.close(); } catch (e) { /* closed */ } }
      else toast("err", ev.error);
      break;
  }
  refreshSock(tab);
}
function closeSocket(tab) {
  const s = tab.sock;
  if (!s) return;
  if (s.state === "open") { s.conn.send(JSON.stringify({ type: "close" })); return; }
  s.state = "closed";
  if (tab.result) { tab.result.state = "closed"; tab.result.how = "cancelled"; }
  try { s.conn.close(); } catch (e) { /* closed */ }
  refreshSock(tab);
}
function sendSocketMessage(tab, text) {
  if (!tab.sock || tab.sock.state !== "open") return toast("warn", "Connect first");
  tab.sock.conn.send(JSON.stringify({ type: "send", message: text, request: tab.req }));
}
function refreshSock(tab) {
  renderOpenTabs();
  if (tab === activeTab()) { paintSend(tab); renderResponse(); }
}

// ---------- response ----------

function renderResponse() {
  const pane = $("respPane");
  pane.replaceChildren();
  const tab = activeTab();
  if (!tab) return;
  const r = tab.result;
  pane.append(h("div", { class: "resp-head", id: "respHead" }));
  if (!r) {
    const ws = tab.req.type === "websocket";
    pane.append(h("div", { class: "resp-body" }, h("div", { class: "empty" }, h("div", null,
      h("div", { class: "art" }, icon(ws ? "plug" : "send", "icon-svg")),
      h("h3", null, ws ? "Not connected" : "No response yet"),
      h("p", null, ws ? "Connect to open the socket. Messages appear here live." : "Send the request to see the response here."),
      h("p", { style: { marginTop: "14px" } }, h("kbd", null, MOD), " ", h("kbd", null, "Enter"), h("span", { style: { margin: "0 8px" } }, "send"), h("kbd", null, MOD), " ", h("kbd", null, "S"), h("span", { style: { margin: "0 8px" } }, "save"), h("kbd", null, MOD), " ", h("kbd", null, "K"), h("span", { style: { marginLeft: "8px" } }, "search"))))));
    paintRespHead(tab);
    return;
  }
  if (r.kind === "ws") return renderSocket(tab, pane);
  if (r.live && r.status == null && !r.error) {
    pane.append(h("div", { class: "resp-body" }, h("div", { class: "sending" }, h("div", { class: "spinner" }), h("div", null, "Sending ", h("b", { class: "mono" }, r.method || tab.req.method), " ", h("span", { class: "mono" }, r.url || tab.req.url)), btn("Stop", { cls: "sm", icon: "stop", onclick: () => stopTab(tab) }))));
    paintRespHead(tab);
    return;
  }
  if (r.error && r.status == null) {
    pane.append(h("div", { class: "resp-body" }, h("div", { class: "errbox" }, h("b", null, r.stopped ? "Stopped" : "Request failed"), h("div", { class: "mono", style: { whiteSpace: "pre-wrap" } }, r.error),
      r.env != null ? h("div", { class: "hint", style: { marginTop: "8px" } }, "Environment: " + (r.env || "none (globals only)")) : null),
      hintsFor(r)));
    paintRespHead(tab);
    return;
  }
  const tabs = [["body", "Body"], ["headers", "Headers", headerList(r.headers).length]];
  if (r.stream || (r.events && r.events.length)) tabs.unshift(["events", "Events", r.events.length]);
  if (r.tests && r.tests.length || tab.req.tests.trim()) tabs.push(["tests", "Tests", r.tests ? `${r.tests.filter((t) => t.passed).length}/${r.tests.length}` : null]);
  if (!tab.respTab || !tabs.some((t) => t[0] === tab.respTab)) tab.respTab = tabs[0][0];
  pane.append(h("div", { class: "tabs", style: { paddingTop: "4px" } }, tabs.map(([id, name, badge]) => h("button", { class: tab.respTab === id ? "on" : "", onclick: () => { tab.respTab = id; renderResponse(); } }, name, badge != null ? h("span", { class: "badge" }, badge) : null))));
  const body = h("div", { class: "resp-body", id: "respBody" });
  pane.append(body);
  if ((r.missing || []).length) body.append(h("div", { class: "warnline" }, "Undefined variables sent as typed: " + r.missing.map((m) => "{{" + m + "}}").join(", ")));
  (r.warnings || []).forEach((w) => body.append(h("div", { class: "warnline" }, w)));
  if (r.fromHistory) body.append(h("div", { class: "warnline", style: { background: "var(--accent-bg)", color: "var(--fg-2)" } }, "From history, " + new Date(r.fromHistory).toLocaleString() + ". Send again for a fresh response."));
  switch (tab.respTab) {
    case "body": renderBody(tab, body); break;
    case "headers": renderHeaders(tab, body); break;
    case "tests": renderTests(r.tests || [], body); break;
    case "events": renderEvents(tab, body); break;
  }
  paintRespHead(tab);
}
function hintsFor(r) {
  const e = (r.error || "").toLowerCase();
  const tips = [];
  if (e.includes("connection refused")) tips.push("Nothing is listening at that address. If it's the demo API, run term-rest-client demo in another terminal.");
  if (e.includes("no such host")) tips.push("The host name doesn't resolve. Check the URL and the active environment's variables.");
  if (e.includes("unsupported protocol") || e.includes("missing protocol")) tips.push("Start the URL with http:// or https://, or define the variable it starts with.");
  if (e.includes("certificate")) tips.push("For self-signed certificates, turn off TLS verification in Settings.");
  if (e.includes("timeout") || e.includes("deadline")) tips.push("The server took too long. Raise the timeout in Settings.");
  if (!tips.length) return null;
  return h("div", { class: "help", style: { margin: "0 16px" } }, tips.map((t) => h("div", null, t)));
}
function paintRespHead(tab) {
  const head = $("respHead");
  if (!head || tab !== activeTab()) return;
  const r = tab.result;
  head.replaceChildren();
  if (!r) { head.append(h("span", { class: "hint", style: { fontWeight: 600, color: "var(--muted)" } }, "Response")); return; }
  if (r.kind === "ws") {
    if (r.state === "open") head.append(h("span", { class: "live" }, h("i"), "Connected"));
    else if (r.state === "connecting") head.append(h("span", { class: "live", style: { color: "var(--warn)" } }, h("span", { class: "spinner busy" }), "Connecting"));
    else head.append(h("span", { class: "pill " + (r.error ? "serr" : "s3") }, r.error ? "Failed" : "Closed"));
    if (r.status) head.append(h("span", { class: "pill " + statusClass(r.status) }, r.status, " ", r.status === 101 ? "Switching Protocols" : ""));
    const sent = r.messages.filter((m) => m.dir === "out").length, recv = r.messages.length - sent;
    head.append(h("span", { class: "metric" }, "↑ ", h("b", null, sent), "  ↓ ", h("b", null, recv)));
    const dur = r.state === "closed" ? r.durationMs : r.opened ? Date.now() - r.opened : null;
    if (dur != null) head.append(h("span", { class: "metric" }, "Time ", h("b", null, humanMs(dur))));
  } else {
    if (r.live && r.status == null) head.append(h("span", { class: "live", style: { color: "var(--accent)" } }, h("span", { class: "spinner busy" }), "Sending"), h("span", { class: "metric" }, h("b", null, humanMs(performance.now() - r.started))));
    else if (r.error && r.status == null) head.append(h("span", { class: "pill serr" }, r.stopped ? "Stopped" : "Error"));
    else {
      head.append(h("span", { class: "pill " + statusClass(r.status) }, r.status, " ", (r.statusText || "").replace(/^\d+\s*/, "")));
      if (r.live) head.append(h("span", { class: "live" }, h("i"), "Streaming"));
      else if (r.stopped) head.append(h("span", { class: "pill s3" }, "Stopped"));
      const t = r.live ? (r.elapsed != null ? r.elapsed : performance.now() - r.started) : r.durationMs;
      head.append(h("span", { class: "metric" }, "Time ", h("b", null, humanMs(t))), h("span", { class: "metric" }, "Size ", h("b", null, humanBytes(r.size || 0))));
      if (r.events && r.events.length) head.append(h("span", { class: "metric" }, "Events ", h("b", null, r.events.length)));
      if (r.tests && r.tests.length) {
        const ok = r.tests.filter((x) => x.passed).length;
        head.append(h("span", { class: "metric", style: { color: ok === r.tests.length ? "var(--ok)" : "var(--err)" } }, "Tests ", h("b", { style: { color: "inherit" } }, ok + "/" + r.tests.length)));
      }
    }
  }
  head.append(h("div", { class: "grow" }));
  if (!(r.live && r.status == null)) {
    const c = btn("Copy", { cls: "sm", icon: "copy", title: "Copy the body, headers, full response, curl…" });
    c.onclick = () => popover(c, copyChoices(tab), { alignRight: true });
    head.append(c);
    if (r.kind === "http" && r.status != null) head.append(btn("", { cls: "sm ghost icon", icon: "download", title: "Save the body to a file", onclick: () => saveBody(tab) }));
  }
}
function bodyBytesText(r) {
  if (!r.base64) return r.body || "";
  try { return atob(r.body); } catch (e) { return r.body; }
}
function saveBody(tab) {
  const r = tab.result;
  const ct = headerValue(r.headers, "content-type");
  const ext = /json/.test(ct) ? "json" : /html/.test(ct) ? "html" : /xml/.test(ct) ? "xml" : /png/.test(ct) ? "png" : /jpe?g/.test(ct) ? "jpg" : /event-stream/.test(ct) ? "txt" : "txt";
  let data = r.body || "";
  if (r.base64) { const bin = atob(r.body); data = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) data[i] = bin.charCodeAt(i); }
  const a = h("a", { href: URL.createObjectURL(new Blob([data], { type: ct || "application/octet-stream" })), download: (tab.req.name || "response").replace(/[^\w.-]+/g, "_") + "." + ext });
  document.body.append(a); a.click(); setTimeout(() => a.remove(), 500);
}
function copyChoices(tab) {
  const r = tab.result;
  const items = [];
  const add = (label2, text, what, ic) => items.push({ label: label2, icon: ic || "copy", run: () => copy(text, what) });
  if (r.kind === "ws") {
    if (r.messages.length) {
      add("Messages as JSON", JSON.stringify(r.messages.map((m) => ({ direction: m.dir === "out" ? "sent" : "received", time: new Date(m.at).toISOString(), data: m.data, binary: m.binary || undefined })), null, 2) + "\n", "the messages");
      const last = r.messages.filter((m) => m.dir === "in").pop();
      if (last) add("Last received message", last.data, "the last message");
    }
    if (r.headers && Object.keys(r.headers).length) { add("Handshake response headers", headerText(r.headers), "the headers"); add("…as JSON", headerJSON(r.headers), "the headers as JSON", "code"); }
    if (r.requestHeaders && Object.keys(r.requestHeaders).length) add("Handshake request headers", headerText(r.requestHeaders), "the request headers");
  } else if (r.status != null) {
    const body = r.body || "";
    add("Body as received (" + humanBytes(r.size || 0) + ")", r.base64 ? bodyBytesText(r) : body, "the response body");
    const j = tryJSON(body);
    if (j !== undefined) add("Body as pretty JSON", JSON.stringify(j, null, 2) + "\n", "the body as pretty JSON", "code");
    if (r.events && r.events.length) add("Events as JSON (" + r.events.length + ")", JSON.stringify(r.events.map((e) => ({ at_ms: Math.round(e.atMs), type: e.type || undefined, id: e.id || undefined, data: tryJSON(e.data) !== undefined ? JSON.parse(e.data) : e.data })), null, 2) + "\n", "the events", "code");
    items.push("-");
    add("Response headers", headerText(r.headers), "the response headers");
    add("Response headers as JSON", headerJSON(r.headers), "the response headers as JSON", "code");
    if (r.requestHeaders && Object.keys(r.requestHeaders).length) {
      add("Request headers", headerText(r.requestHeaders), "the request headers");
      add("Request headers as JSON", headerJSON(r.requestHeaders), "the request headers as JSON", "code");
    }
    items.push("-");
    add("Full response (like curl -i)", (r.proto || "HTTP/1.1") + " " + r.status + " " + (r.statusText || "").replace(/^\d+\s*/, "") + "\n" + headerText(r.headers) + "\n" + bodyBytesText(r), "the full response");
  } else if (r.error) add("Error message", r.error, "the error");
  items.push("-", { label: "Request as cURL", icon: "code", run: () => copyCode(tab.req, "curl") });
  return items;
}
const BIG = 400000;
function renderBody(tab, body) {
  const r = tab.result;
  const ct = headerValue(r.headers, "content-type").toLowerCase();
  const text = r.body || "";
  const json = text.length < 8e6 ? tryJSON(text) : undefined;
  const isImg = /^image\//.test(ct) && r.base64;
  const isHTML = /html/.test(ct);
  const views = [];
  if (json !== undefined) views.push(["pretty", "Pretty"], ["tree", "Tree"]);
  views.push(["raw", "Raw"]);
  if (isImg || isHTML) views.push(["preview", "Preview"]);
  if (!views.some((v) => v[0] === tab.bodyView)) tab.bodyView = isImg ? "preview" : views[0][0];
  const search = h("input", { class: "input", placeholder: "Find in body", style: { height: "28px", maxWidth: "220px" } });
  body.append(h("div", { class: "sub-tools" }, h("div", { class: "seg" }, views.map(([id, n]) => h("button", { class: tab.bodyView === id ? "on" : "", onclick: () => { tab.bodyView = id; renderResponse(); } }, n))),
    h("span", { class: "hint" }, ct || "no content type"), r.truncated ? h("span", { class: "pill s4" }, "truncated") : null, h("div", { class: "grow" }),
    tab.bodyView !== "preview" ? search : null));
  const view = h("div");
  body.append(view);
  if (!text && !r.base64) { view.append(h("div", { class: "empty", style: { height: "auto", paddingTop: "48px" } }, h("div", null, h("h3", null, "Empty body"), h("p", null, r.status === 204 ? "204 No Content" : "The server sent no bytes.")))); return; }
  if (tab.bodyView === "preview") {
    if (isImg) view.append(h("div", { class: "resp-pad" }, h("img", { class: "preview-img", src: "data:" + ct.split(";")[0] + ";base64," + r.body, alt: "Response image" })));
    else {
      const f = h("iframe", { sandbox: "", style: { width: "100%", height: "70vh", border: 0, background: "#fff" } });
      f.srcdoc = text;
      view.append(f);
    }
    return;
  }
  if (r.base64) { view.append(h("pre", { class: "code-block" }, "Binary body, " + humanBytes(r.size) + ". Save it with the download button.")); return; }
  const pre = h("pre", { class: "code-block" });
  const paint = (q) => {
    if (tab.bodyView === "tree") { view.replaceChildren(jsonTree(json, q)); return; }
    const src = tab.bodyView === "pretty" ? JSON.stringify(json, null, 2) : text;
    if (src.length > BIG) pre.textContent = src;
    else pre.innerHTML = tab.bodyView === "pretty" ? highlightJSON(src) : (json !== undefined && text.length < BIG ? highlightJSON(src) : esc(src));
    if (q) markText(pre, q);
    view.replaceChildren(pre);
  };
  search.addEventListener("input", () => paint(search.value));
  paint("");
}
function markText(root, q) {
  if (!q) return;
  const ql = q.toLowerCase();
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes = [];
  while (walker.nextNode()) nodes.push(walker.currentNode);
  let first = null, count = 0;
  for (const n of nodes) {
    const t = n.nodeValue, tl = t.toLowerCase();
    let i = tl.indexOf(ql);
    if (i < 0) continue;
    const frag = document.createDocumentFragment();
    let last = 0;
    while (i >= 0 && count < 2000) {
      frag.append(t.slice(last, i));
      const m = h("mark", { style: { background: "var(--warn-bg)", color: "var(--warn)", borderRadius: "3px", outline: "1px solid var(--warn)" } }, t.slice(i, i + q.length));
      if (!first) first = m;
      frag.append(m);
      count++;
      last = i + q.length;
      i = tl.indexOf(ql, last);
    }
    frag.append(t.slice(last));
    n.replaceWith(frag);
  }
  if (first) first.scrollIntoView({ block: "center" });
}
function jsonTree(value, q) {
  const root = h("div", { class: "jt" });
  let budget = 5000; // nodes rendered before collapsing the rest
  const ql = (q || "").toLowerCase();
  const prim = (v) => {
    if (v === null) return h("span", { class: "j-lit" }, "null");
    if (typeof v === "string") return h("span", { class: "j-str" }, JSON.stringify(v));
    if (typeof v === "number") return h("span", { class: "j-num" }, String(v));
    return h("span", { class: "j-lit" }, String(v));
  };
  const node = (key, v, depth, last, path) => {
    budget--;
    const line = h("div", { class: "jl" });
    const keyEl = key != null ? [h("span", { class: "j-key", title: path, style: { cursor: "copy" }, onclick: (e) => { e.stopPropagation(); copy(path, path); } }, typeof key === "number" ? key : JSON.stringify(key)), h("span", { class: "j-p" }, ": ")] : [];
    const comma = last ? "" : h("span", { class: "j-p" }, ",");
    if (v === null || typeof v !== "object") {
      line.append(...keyEl, prim(v), comma);
      if (ql && (String(key).toLowerCase().includes(ql) || String(v).toLowerCase().includes(ql))) line.style.background = "var(--warn-bg)";
      return line;
    }
    const arr = Array.isArray(v);
    const entries = arr ? v.map((x, i) => [i, x]) : Object.entries(v);
    const [o, c] = arr ? ["[", "]"] : ["{", "}"];
    if (!entries.length) { line.append(...keyEl, h("span", { class: "j-p" }, o + c), comma); return line; }
    const closed = (depth > 2 && !ql) || budget < 0;
    const wrap = h("div", { class: closed ? "closed" : "" });
    const tog = h("span", { class: "tog" }, closed ? "▸" : "▾");
    const n = entries.length;
    const summary = h("span", { class: "ellip", onclick: () => toggle() }, n + (arr ? (n === 1 ? " item" : " items") : (n === 1 ? " key" : " keys")));
    const inlineClose = h("span", { class: "inline-close" }, " ", h("span", { class: "j-p" }, c), comma ? h("span", { class: "j-p" }, ",") : null);
    const kids = h("div", { class: "kids" });
    let built = false;
    const build = () => {
      if (built) return;
      built = true;
      entries.forEach(([k, x], i) => kids.append(node(k, x, depth + 1, i === entries.length - 1, path + (arr ? "[" + k + "]" : /^[A-Za-z_$][\w$]*$/.test(k) ? "." + k : "[" + JSON.stringify(k) + "]"))));
    };
    const toggle = () => {
      const isClosed = wrap.classList.toggle("closed");
      tog.textContent = isClosed ? "▸" : "▾";
      summary.style.display = isClosed ? "" : "none";
      if (!isClosed) build();
    };
    tog.addEventListener("click", toggle);
    line.append(tog, ...keyEl, h("span", { class: "j-p" }, o), " ", summary, inlineClose);
    summary.style.display = closed ? "" : "none";
    if (!closed) build();
    wrap.append(line, kids, h("div", { class: "jl close" }, h("span", { class: "j-p" }, c), comma));
    return wrap;
  };
  root.append(node(null, value, 0, true, "json"));
  root.append(h("div", { class: "hint", style: { marginTop: "10px" } }, "Click a key to copy its path for tests, e.g. json.items[0].id"));
  return root;
}
function renderHeaders(tab, body) {
  const r = tab.result;
  const seg = h("div", { class: "seg" }, [["response", "Response"], ["request", "Request"]].map(([id, n]) => h("button", { class: tab.hdrView === id ? "on" : "", onclick: () => { tab.hdrView = id; renderResponse(); } }, n)));
  const hdrs = tab.hdrView === "request" ? r.requestHeaders || {} : r.headers || {};
  const what = tab.hdrView === "request" ? "the request headers" : "the response headers";
  body.append(h("div", { class: "sub-tools" }, seg, h("div", { class: "grow" }),
    btn("Copy", { cls: "sm ghost", icon: "copy", onclick: () => copy(headerText(hdrs), what) }),
    btn("Copy JSON", { cls: "sm ghost", icon: "code", onclick: () => copy(headerJSON(hdrs), what + " as JSON") })));
  const list = headerList(hdrs);
  if (tab.hdrView === "response" && r.proto) body.append(h("div", { class: "hint", style: { padding: "10px 16px 0" } }, r.proto + " · " + (r.url || "")));
  if (tab.hdrView === "request" && r.method) body.append(h("div", { class: "hint", style: { padding: "10px 16px 0" } }, r.method + " " + (r.url || "")));
  if (tab.hdrView === "request" && r.env != null) body.append(h("div", { class: "hint", style: { padding: "2px 16px 0" } }, "Environment: " + (r.env || "none (globals only)")));
  if (!list.length) { body.append(h("div", { class: "hint", style: { padding: "16px" } }, "No headers.")); return; }
  body.append(h("table", { class: "table" }, h("tbody", null, list.map(([k, v]) => h("tr", { title: "Click to copy", style: { cursor: "copy" }, onclick: () => copy(v, k) }, h("td", null, k), h("td", null, v))))));
}
function renderTests(tests, body) {
  if (!tests.length) { body.append(h("div", { class: "help", style: { margin: "16px" } }, "No tests ran. Write them in the request's Tests tab, one per line, e.g. status == 200.")); return; }
  const ok = tests.filter((t) => t.passed).length;
  const bar = h("div", { class: "summary-bar" }, h("i", { style: { width: "0%" } }));
  body.append(h("div", { class: "row", style: { padding: "14px 16px 0" } }, h("b", { style: { color: ok === tests.length ? "var(--ok)" : "var(--err)" } }, ok + " of " + tests.length + " passed")), bar);
  requestAnimationFrame(() => { bar.firstChild.style.width = (100 * ok / tests.length) + "%"; });
  tests.forEach((t) => body.append(h("div", { class: "test " + (t.capture ? "cap" : t.passed ? "pass" : "fail") },
    h("span", { class: "mark" }, t.capture ? "→" : t.passed ? "✓" : "✗"), h("span", null, t.source), t.message ? h("span", { class: "msg" }, t.message) : null)));
}
function renderEvents(tab, body) {
  const r = tab.result;
  body.append(h("div", { class: "sub-tools" }, h("span", { class: "hint" }, r.stream === "sse" ? "Server-Sent Events" : r.stream === "ndjson" ? "Newline-delimited JSON" : "Stream"),
    h("div", { class: "grow" }), r.live ? btn("Stop", { cls: "sm", icon: "stop", onclick: () => stopTab(tab) }) : null));
  const list = h("div", { id: "evList" });
  body.append(list);
  if (!r.events.length) list.append(h("div", { class: "hint", id: "evEmpty", style: { padding: "16px" } }, r.live ? "Waiting for the first event…" : "No events."));
  appendEvents(tab, r.events, true);
}
function appendEvents(tab, evs, initial) {
  const list = $("evList");
  if (!list) return;
  const empty = $("evEmpty");
  if (empty && evs.length) empty.remove();
  const body = $("respBody");
  const atBottom = body && body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  const frag = document.createDocumentFragment();
  for (const e of evs) {
    const j = tryJSON(e.data);
    const d = h("span", { class: "d" });
    if (j !== undefined && e.data.length < 20000) d.innerHTML = highlightJSON(e.data); else d.textContent = e.data;
    const row = h("div", { class: "ev" }, h("span", { class: "t" }, "+" + humanMs(e.atMs)), h("span", { class: "k" }, e.type || (e.id ? "#" + e.id : "data")), d);
    if (initial) row.style.animation = "none";
    frag.append(row);
  }
  list.append(frag);
  if (body && (atBottom || initial)) body.scrollTop = body.scrollHeight;
  paintRespHead(tab);
  const badge = document.querySelector("#respPane .tabs button.on .badge");
  if (badge && tab.respTab === "events") badge.textContent = tab.result.events.length;
}
function renderSocket(tab, pane) {
  const r = tab.result;
  const tabs = [["messages", "Messages", r.messages.length], ["headers", "Handshake", null]];
  if (r.tests && r.tests.length || (tab.req.tests.trim() && r.state === "closed")) tabs.push(["tests", "Tests", r.tests.length ? `${r.tests.filter((t) => t.passed).length}/${r.tests.length}` : null]);
  if (!tabs.some((t) => t[0] === tab.respTab)) tab.respTab = "messages";
  pane.append(h("div", { class: "tabs", style: { paddingTop: "4px" } }, tabs.map(([id, name, badge]) => h("button", { class: tab.respTab === id ? "on" : "", onclick: () => { tab.respTab = id; renderResponse(); } }, name, badge != null ? h("span", { class: "badge" }, badge) : null))));
  const body = h("div", { class: "resp-body", id: "respBody" });
  pane.append(body);
  if (r.error) body.append(h("div", { class: "errbox" }, h("b", null, "Could not connect"), h("div", { class: "mono", style: { whiteSpace: "pre-wrap" } }, r.error)));
  if ((r.missing || []).length) body.append(h("div", { class: "warnline" }, "Undefined variables: " + r.missing.map((m) => "{{" + m + "}}").join(", ")));
  if (tab.respTab === "headers") {
    const fake = { result: { env: r.env, headers: r.headers, requestHeaders: r.requestHeaders, proto: r.protocol ? "subprotocol " + r.protocol : "", url: r.url, method: "GET" } };
    renderHeaders(Object.assign(fake, { hdrView: tab.hdrView, get hdrViewSetter() { return null; } }), body);
    body.querySelectorAll(".seg button").forEach((b, i) => b.addEventListener("click", () => { tab.hdrView = i ? "request" : "response"; renderResponse(); }));
  } else if (tab.respTab === "tests") renderTests(r.tests || [], body);
  else {
    const list = h("div", { id: "msgList" });
    body.append(list);
    if (!r.messages.length && !r.error) list.append(h("div", { class: "hint", id: "msgEmpty", style: { padding: "16px" } }, r.state === "connecting" ? "Connecting…" : r.state === "open" ? "Connected. Nothing received yet." : "No messages."));
    r.messages.forEach((m) => appendMessage(tab, m, true));
    if (r.state === "closed" && !r.error) list.append(h("div", { class: "msg-row sys" }, h("span"), h("span"), h("span", null, "Closed: " + (r.how || "done") + (r.durationMs != null ? " after " + humanMs(r.durationMs) : ""))));
    body.scrollTop = body.scrollHeight;
  }
  // Composer.
  const ta = h("textarea", { class: "textarea", rows: 1, placeholder: r.state === "open" ? "Message, Enter to send (Shift+Enter for a new line)" : "Connect to send messages", disabled: r.state !== "open", spellcheck: "false" });
  ta.value = tab.draft || "";
  const sendIt = () => { if (!ta.value) return; sendSocketMessage(tab, ta.value); tab.draft = ""; ta.value = ""; ta.style.height = ""; };
  ta.addEventListener("input", () => { tab.draft = ta.value; ta.style.height = "auto"; ta.style.height = Math.min(160, ta.scrollHeight) + "px"; });
  ta.addEventListener("keydown", (e) => { if (e.key === "Enter" && !e.shiftKey && !e.metaKey && !e.ctrlKey) { e.preventDefault(); sendIt(); } });
  pane.append(h("div", { class: "composer" }, ta, btn("Send", { cls: "accent", icon: "send", onclick: sendIt, disabled: r.state !== "open" })));
  paintRespHead(tab);
  if (r.state === "open") setTimeout(() => { if (document.activeElement === document.body) ta.focus(); }, 20);
}
function appendMessage(tab, m, initial) {
  const list = $("msgList");
  if (!list) return;
  const empty = $("msgEmpty");
  if (empty) empty.remove();
  const body = $("respBody");
  const atBottom = body && body.scrollHeight - body.scrollTop - body.clientHeight < 40;
  const d = h("span", { class: "d" });
  if (tryJSON(m.data) !== undefined && m.data.length < 20000) d.innerHTML = highlightJSON(m.data); else d.textContent = m.binary ? "[binary " + humanBytes(m.size) + "] " + m.data : m.data;
  const row = h("div", { class: "msg-row", title: "Click to copy", style: { cursor: "copy" }, onclick: () => copy(m.data, "the message") },
    h("span", { class: "dir " + m.dir }, m.dir === "out" ? "↑" : "↓"), h("span", { class: "t" }, new Date(m.at).toLocaleTimeString(undefined, { hour12: false }) + "." + String(m.at % 1000).padStart(3, "0")), d);
  if (initial) row.style.animation = "none";
  list.append(row);
  if (body && (atBottom || initial || m.dir === "out")) body.scrollTop = body.scrollHeight;
  const badge = document.querySelector("#respPane .tabs button.on .badge");
  if (badge) badge.textContent = tab.result.messages.length;
}

// ---------- code, import, schema, settings ----------

async function copyCode(req, lang) {
  try {
    const res = await api("POST", "/api/code", { request: req, lang, template: false });
    copy(res.text, lang === "curl" ? "as cURL" : "the " + lang + " code");
    if ((res.missing || []).length) toast("warn", "Undefined variables kept as text", res.missing.join(", "));
  } catch (e) { toast("err", "Could not build the snippet", e.message); }
}
function codeDialog(tab) {
  let lang = localStorage.getItem("trc.lang") || S.ws.settings.code_language || "curl";
  let template = false;
  const out = h("pre", { class: "code-block code-out" });
  const note = h("div", { class: "hint" });
  const seg = h("div", { class: "seg" });
  const tmpl = h("label", { class: "row hint", style: { cursor: "pointer" } }, h("input", { type: "checkbox", class: "checkbox" }), "Keep {{variables}}");
  tmpl.firstChild.addEventListener("change", () => { template = tmpl.firstChild.checked; load(); });
  let text = "";
  const load = async () => {
    seg.replaceChildren(...S.languages.map((l) => h("button", { class: l.id === lang ? "on" : "", onclick: () => { lang = l.id; try { localStorage.setItem("trc.lang", lang); } catch (e) { /* ignore */ } load(); } }, l.name)));
    out.textContent = "…";
    try {
      const res = await api("POST", "/api/code", { request: tab.req, lang, template });
      text = res.text;
      out.textContent = text;
      note.textContent = (res.missing || []).length ? "Undefined variables kept as typed: " + res.missing.join(", ") : template ? "Variables are left as placeholders the code fills in." : "Variables are resolved with the active environment.";
    } catch (e) { out.textContent = e.message; }
  };
  const d = dialog({
    title: "Code snippet", sub: tab.req.name, wide: true,
    body: [h("div", { class: "row" }, seg, h("div", { class: "grow" }), tmpl), out, note],
    footer: [btn("Close", { onclick: () => d.close() }), btn("Copy", { cls: "accent", icon: "copy", onclick: () => copy(text, "the snippet") })],
  });
  load();
}
async function importCurlInto(tab, text) {
  try {
    const res = await api("POST", "/api/import", { text });
    if (res.request) {
      const name = tab.req.name;
      tab.req = normReq(res.request);
      tab.req.name = name && name !== "New request" ? name : res.request.name || name;
      tab.reqTab = null;
      renderAll();
      toast("ok", "Pasted a curl command", (res.notes || []).join(" ") || res.summary);
    } else if (res.workspace) { replaceWorkspace(res.workspace); renderAll(); toast("ok", res.summary); }
  } catch (e) { toast("err", "Couldn't read that curl command", e.message); }
}
function importDialog(prefill) {
  const ta = h("textarea", { class: "textarea", rows: 14, placeholder: "Paste one or more curl commands, or a Postman collection / environment / globals export (JSON).\n\nYou can also drop a file here." });
  if (typeof prefill === "string") ta.value = prefill;
  const file = h("input", { type: "file", accept: ".json,.txt,.sh,.curl", style: { display: "none" } });
  file.addEventListener("change", async () => { if (file.files[0]) ta.value = await file.files[0].text(); });
  ta.addEventListener("dragover", (e) => e.preventDefault());
  ta.addEventListener("drop", async (e) => { e.preventDefault(); const f = e.dataTransfer.files[0]; if (f) ta.value = await f.text(); });
  const go = async () => {
    if (!ta.value.trim()) return toast("warn", "Paste something to import");
    try {
      const res = await api("POST", "/api/import", { text: ta.value });
      d.close();
      if (res.request) { openTab(normReq(Object.assign({ name: res.request.name || "Imported request" }, res.request))); toast("ok", res.summary, "Save it with " + MOD + "+S."); }
      else { replaceWorkspace(res.workspace); S.sideTab = "collections"; renderAll(); toast("ok", res.summary, (res.notes || []).slice(0, 3).join(" ")); }
    } catch (e) { toast("err", "Import failed", e.message); }
  };
  const d = dialog({
    title: "Import", sub: "cURL commands or Postman exports. Everything is added to your workspace.", wide: true,
    body: [ta, file],
    footer: [btn("Choose file…", { cls: "ghost", icon: "upload", onclick: () => file.click() }), h("div", { class: "grow" }), btn("Cancel", { onclick: () => d.close() }), btn("Import", { cls: "accent", icon: "download", onclick: go })],
  });
}
async function schemaDialog(tab) {
  const tree = h("div", { class: "stree" }, h("div", { class: "sending" }, h("div", { class: "spinner" }), "Fetching the schema…"));
  const detail = h("div", { class: "sdetail" }, h("div", { class: "hint" }, "Pick a field to see its arguments and type."));
  const d = dialog({ title: "GraphQL schema", sub: tab.req.url, wide: true, body: h("div", { class: "schema" }, tree, detail), footer: [btn("Close", { onclick: () => d.close() })] });
  let sc;
  try { sc = await api("POST", "/api/schema", { request: tab.req }); } catch (e) {
    tree.replaceChildren(h("div", { class: "errbox" }, h("b", null, "Couldn't fetch the schema"), e.message));
    return;
  }
  const open = new Set();
  let sel = null;
  const insert = (rootField) => {
    tab.req.body_type = "graphql";
    if (tab.req.method === "GET") tab.req.method = "POST";
    tab.req.body = rootField.query;
    tab.req.graphql_variables = rootField.variables || "";
    tab.reqTab = "body";
    d.close();
    renderAll();
    toast("ok", "Wrote a query for " + rootField.name, "Fill in the variables and send.");
  };
  const showDetail = (f, parentType, rootField) => {
    detail.replaceChildren();
    detail.append(h("h3", null, (parentType ? parentType + "." : "") + f.name, h("span", { class: "j-p" }, ": "), h("span", { style: { color: "var(--accent)" } }, f.typeString)));
    if (f.description) detail.append(h("p", { style: { color: "var(--muted)", margin: "6px 0 0" } }, f.description));
    if ((f.args || []).length) {
      detail.append(h("h4", null, "Arguments"));
      f.args.forEach((a) => {
        const t = (f.argTypes || {})[a.name] || "";
        detail.append(h("div", { class: "fl" }, a.name, ": ", h("span", { class: "ty" }, t), t.endsWith("!") ? h("span", { class: "req" }, "required") : null,
          a.defaultValue ? h("span", { class: "hint" }, " = " + a.defaultValue) : null, a.description ? h("div", { class: "hint", style: { fontFamily: "var(--font)" } }, a.description) : null));
      });
    }
    const t = sc.types[f.named];
    if (t && t.fields && t.fields.length) {
      detail.append(h("h4", null, t.name + " fields"));
      t.fields.slice(0, 60).forEach((x) => detail.append(h("div", { class: "fl" }, x.name, ": ", h("span", { class: "ty" }, x.typeString))));
    } else if (t) detail.append(h("h4", null, t.kind.toLowerCase() + " " + t.name));
    if (rootField) {
      detail.append(h("h4", null, "Query"), h("pre", { class: "code-block", style: { border: "1px solid var(--border)", borderRadius: "8px", padding: "10px" } }, rootField.query),
        btn(rootField === f ? "Insert this query" : "Insert the query for " + rootField.name, { cls: "accent", icon: "plus", onclick: () => insert(rootField) }));
    }
  };
  const draw = () => {
    tree.replaceChildren();
    const rec = (f, parentType, depth, rootField, key) => {
      const t = sc.types[f.named];
      const expandable = t && t.fields && t.fields.length && depth < 8;
      const isOpen = open.has(key);
      const row = h("div", { class: "sf" + (sel === key ? " sel" : ""), style: { paddingLeft: 6 + depth * 16 + "px" } },
        h("span", { class: "chev" + (isOpen ? " open" : "") }, expandable ? "▸" : ""),
        h("span", null, f.name), (f.args || []).length ? h("span", { class: "ar" }, "(" + f.args.map((a) => a.name).join(", ") + ")") : null,
        h("span", { class: "j-p" }, ":"), h("span", { class: "ty" }, f.typeString));
      row.addEventListener("click", () => { sel = key; if (expandable) { if (isOpen) open.delete(key); else open.add(key); } draw(); showDetail(f, parentType, rootField); });
      row.addEventListener("dblclick", () => insert(rootField));
      tree.append(row);
      if (isOpen && expandable) t.fields.forEach((x) => rec(x, t.name, depth + 1, rootField, key + "." + x.name));
    };
    for (const op of ["query", "mutation", "subscription"]) {
      const tn = sc.roots[op];
      const t = tn && sc.types[tn];
      if (!t) continue;
      tree.append(h("div", { class: "sroot" }, op + " · " + tn));
      t.fields.forEach((f) => rec(f, "", 0, f, op + ":" + f.name));
    }
  };
  draw();
}
function settingsDialog() {
  const st = S.ws.settings = S.ws.settings || {};
  const toggle = (key, title, sub, invert) => {
    const cb = h("input", { type: "checkbox", class: "checkbox", checked: invert ? !st[key] : !!st[key] });
    cb.addEventListener("change", () => { st[key] = invert ? !cb.checked : cb.checked; persist(true); });
    return h("label", { class: "row", style: { alignItems: "flex-start", gap: "12px", cursor: "pointer" } }, cb, h("div", null, h("div", { style: { fontWeight: 550 } }, title), h("div", { class: "hint" }, sub)));
  };
  const timeout = h("input", { class: "input", type: "number", min: 1, value: st.timeout_seconds || 30, style: { width: "120px" } });
  timeout.addEventListener("change", () => { st.timeout_seconds = Math.max(1, +timeout.value || 30); persist(true); });
  const i = S.info;
  const d = dialog({
    title: "Settings", sub: "Shared with the terminal app; saved in your workspace file.",
    body: [
      h("div", null, h("label", { class: "label" }, "Timeout (seconds)"), timeout, h("div", { class: "hint", style: { marginTop: "4px" } }, "Streams and WebSockets are followed for this long too, unless you stop them.")),
      toggle("disable_redirects", "Follow redirects", "Off returns 3xx responses as they are.", true),
      toggle("insecure_skip_verify", "Skip TLS verification", "For self-signed certificates. Don't leave it on for real APIs."),
      toggle("disable_history", "Pause history", "Requests aren't saved to history while this is on."),
      h("div", { class: "help" },
        h("div", null, h("b", null, "term-rest-client " + (i.version || "")), i.buildTime && i.buildTime !== "unknown" ? " · built " + i.buildTime : ""),
        h("div", null, "Workspace: ", h("code", null, i.dataPath || "in memory")), h("div", null, "History: ", h("code", null, i.historyPath || "in memory")),
        h("div", { style: { marginTop: "6px" } }, "Requests run in the term-rest-client process, not the browser, so there's no CORS and no browser-imposed headers.")),
    ],
    footer: [btn("Done", { cls: "accent", onclick: () => d.close() })],
  });
}
async function runFolder(c) {
  const list = [];
  const rec = (x, trail) => { (x.requests || []).forEach((r) => list.push({ r, path: trail.concat([x.name, r.name]).join(" / ") })); (x.folders || []).forEach((f) => rec(f, trail.concat([x.name]))); };
  rec(c, (trailOf((c.requests || [])[0]) || [c]).slice(0, -1).map((x) => x.name));
  const rows = h("div", { style: { border: "1px solid var(--border)", borderRadius: "10px", overflow: "hidden" } });
  const sum = h("div", { class: "hint" }, "Running " + list.length + " requests in order…");
  let stopped = false;
  const d = dialog({ title: "Run " + c.name, sub: "Each request is sent in order with its scripts and tests, like term-rest-client run.", wide: true, body: [sum, rows], footer: [btn("Close", { onclick: () => { stopped = true; d.close(); } })], onclose: () => { stopped = true; } });
  let passed = 0, failed = 0, errors = 0;
  for (const { r, path } of list) {
    if (stopped) break;
    const row = h("div", { class: "test", style: { alignItems: "center" } }, h("span", { class: "spinner busy" }), h("span", { class: "method m-" + label(r), style: { width: "auto" } }, label(r)), h("span", null, path));
    rows.append(row);
    if (r.type === "websocket") { row.firstChild.replaceWith(h("span", { class: "mark hint" }, "·")); row.append(h("span", { class: "msg", style: { color: "var(--dim)" } }, "WebSocket: open it to connect")); continue; }
    let res = null;
    try {
      const resp = await fetch("/api/send", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ request: r, source: path }) });
      const lines = (await resp.text()).trim().split("\n").map((l) => JSON.parse(l));
      res = lines[lines.length - 1];
    } catch (e) { res = { kind: "error", error: e.message }; }
    if (res.history) S.history = res.history;
    if (res.workspace) replaceWorkspace(res.workspace);
    if (res.kind === "error") {
      errors++;
      row.replaceChildren(h("span", { class: "mark", style: { color: "var(--err)" } }, "✗"), h("span", { class: "method m-" + label(r), style: { width: "auto" } }, label(r)), h("span", null, path), h("span", { class: "msg" }, res.error));
      continue;
    }
    const t = res.tests || [];
    const bad = t.filter((x) => !x.passed).length;
    passed += t.length - bad; failed += bad;
    row.replaceChildren(h("span", { class: "mark", style: { color: bad ? "var(--err)" : "var(--ok)" } }, bad ? "✗" : "✓"), h("span", { class: "method m-" + label(r), style: { width: "auto" } }, label(r)), h("span", null, path),
      h("span", { class: "msg", style: { color: "var(--muted)" } }, h("span", { class: "status-dot " + statusClass(res.response.status) }, res.response.status), " · ", humanMs(res.response.durationMs), t.length ? " · " + (t.length - bad) + "/" + t.length + " tests" : ""));
  }
  sum.textContent = (stopped ? "Stopped. " : "Done. ") + `${passed} tests passed, ${failed} failed` + (errors ? `, ${errors} request${errors === 1 ? "" : "s"} failed` : "") + ".";
  sum.style.color = failed || errors ? "var(--err)" : "var(--ok)";
  renderSidebar(); renderEnvPick();
}

// ---------- command palette ----------

function palette() {
  if (dialogs.some((d) => d.ov.classList.contains("palette"))) return;
  const input = h("input", { placeholder: "Search requests, history and actions…", autofocus: true, spellcheck: "false" });
  const list = h("div", { class: "list" });
  let hl = 0, flat = [];
  const actions = [
    { label: "New request", icon: "plus", kbd: MOD + " Alt N", run: () => openTab(newRequest()) },
    { label: "Import cURL or Postman", icon: "download", run: importDialog },
    { label: "New collection", icon: "box", run: newCollection },
    { label: "Variables and environments", icon: "layers", run: () => envDialog() },
    { label: "Code snippet for this request", icon: "code", run: () => activeTab() && codeDialog(activeTab()) },
    { label: "Toggle light / dark", icon: "sun", run: toggleTheme },
    { label: "Toggle side-by-side / stacked", icon: "cols", run: toggleLayout },
    { label: "Settings", icon: "wand", run: settingsDialog },
  ].concat((S.ws.environments || []).map((e) => ({ label: "Use environment: " + e.name, icon: "globe", run: () => { S.ws.active_environment = e.name; persist(true); renderAll(); } })));
  const draw = () => {
    const q = input.value.trim().toLowerCase();
    const words = q.split(/\s+/).filter(Boolean);
    const fits = (s) => words.every((w) => s.toLowerCase().includes(w));
    const reqs = [];
    walk((r, t) => { const where = t.map((c) => c.name).join(" / "); if (fits(r.name + " " + where + " " + r.url + " " + label(r))) reqs.push({ r, where }); });
    const acts = actions.filter((a) => fits(a.label));
    const hist = q ? S.history.filter((e) => fits(e.method + " " + e.url)).slice(0, 6) : [];
    flat = [];
    list.replaceChildren();
    const group = (name, items, make) => {
      if (!items.length) return;
      list.append(h("div", { class: "group" }, name));
      items.forEach((it) => { const el = make(it); const idx = flat.length; flat.push(el); el.addEventListener("mousemove", () => { if (hl !== idx) { hl = idx; paint(); } }); list.append(el); });
    };
    group("Requests", reqs.slice(0, 40), ({ r, where }) => {
      const el = h("div", { class: "opt" }, h("span", { class: "method m-" + label(r) }, label(r)), h("span", null, r.name), h("span", { class: "where" }, where));
      el.run = () => openTab(r, r); el.onclick = () => choose(el); return el;
    });
    group("Actions", acts, (a) => { const el = h("div", { class: "opt" }, icon(a.icon), h("span", null, a.label), a.kbd ? h("kbd", { style: { marginLeft: "auto" } }, a.kbd) : null); el.run = a.run; el.onclick = () => choose(el); return el; });
    group("History", hist, (e) => {
      const el = h("div", { class: "opt" }, h("span", { class: "method m-" + e.method }, e.method), h("span", { class: "mono", style: { fontSize: "12px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, e.url), h("span", { class: "where" }, ago(e.time)));
      el.run = () => openHistory(e, S.history.indexOf(e)); el.onclick = () => choose(el); return el;
    });
    if (!flat.length) list.append(h("div", { class: "hint", style: { padding: "18px", textAlign: "center" } }, "Nothing matches."));
    hl = Math.min(hl, Math.max(0, flat.length - 1));
    paint();
  };
  const paint = () => flat.forEach((el, i) => { el.classList.toggle("hl", i === hl); if (i === hl) el.scrollIntoView({ block: "nearest" }); });
  const choose = (el) => { d.close(); el.run(); };
  input.addEventListener("input", () => { hl = 0; draw(); });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); hl = Math.min(flat.length - 1, hl + 1); paint(); }
    if (e.key === "ArrowUp") { e.preventDefault(); hl = Math.max(0, hl - 1); paint(); }
    if (e.key === "Enter" && flat[hl]) { e.preventDefault(); choose(flat[hl]); }
  });
  const ov = h("div", { class: "overlay palette" }, h("div", { class: "dialog" }, input, list));
  ov.addEventListener("mousedown", (e) => { if (e.target === ov) d.close(); });
  document.body.append(ov);
  const d = { ov, close: () => { const i = dialogs.indexOf(d); if (i >= 0) dialogs.splice(i, 1); ov.remove(); } };
  dialogs.push(d);
  draw();
  setTimeout(() => input.focus(), 20);
}

// ---------- layout, theme, keys ----------

function toggleTheme() {
  const next = document.documentElement.dataset.theme === "light" ? "dark" : "light";
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem("trc.theme", next); } catch (e) { /* ignore */ }
  paintChrome();
}
function toggleLayout() {
  const wa = $("workarea");
  wa.classList.toggle("stacked");
  try { localStorage.setItem("trc.layout", wa.classList.contains("stacked") ? "stacked" : "cols"); } catch (e) { /* ignore */ }
  paintChrome();
}
function paintChrome() {
  const light = document.documentElement.dataset.theme === "light";
  $("themeBtn").replaceChildren(icon(light ? "moon" : "sun"));
  $("layoutBtn").replaceChildren(icon($("workarea").classList.contains("stacked") ? "cols" : "rows"));
}
function setupGutter() {
  const g = $("gutter"), wa = $("workarea");
  g.addEventListener("mousedown", (e) => {
    e.preventDefault();
    g.classList.add("dragging");
    const rect = wa.getBoundingClientRect();
    const stacked = wa.classList.contains("stacked");
    const move = (ev) => {
      const f = stacked ? (ev.clientY - rect.top) / rect.height : (ev.clientX - rect.left) / rect.width;
      const pct = Math.min(80, Math.max(20, f * 100));
      wa.style.setProperty("--split", pct + "%");
    };
    const up = () => {
      g.classList.remove("dragging");
      removeEventListener("mousemove", move); removeEventListener("mouseup", up);
      try { localStorage.setItem("trc.split", wa.style.getPropertyValue("--split")); } catch (err) { /* ignore */ }
    };
    addEventListener("mousemove", move); addEventListener("mouseup", up);
  });
  g.addEventListener("dblclick", () => wa.style.setProperty("--split", "50%"));
}
document.addEventListener("keydown", (e) => {
  const mod = e.metaKey || e.ctrlKey;
  if (e.key === "Escape") {
    if (openPop) { closePop(); return; }
    if (dialogs.length) { dialogs[dialogs.length - 1].close(); return; }
    const t = activeTab();
    if (t && (t.sending || (t.sock && t.sock.state !== "closed"))) { stopTab(t); return; }
  }
  if (dialogs.length && !(mod && e.key.toLowerCase() === "k")) return;
  if (mod && e.key === "Enter") { e.preventDefault(); send(); }
  else if (mod && e.key.toLowerCase() === "s") { e.preventDefault(); save(); }
  else if (mod && e.key.toLowerCase() === "k") { e.preventDefault(); if (dialogs.length) dialogs[dialogs.length - 1].close(); else palette(); }
  else if (mod && e.altKey && e.key.toLowerCase() === "n") { e.preventDefault(); openTab(newRequest()); }
  else if (mod && e.key.toLowerCase() === "n" && !e.shiftKey) { e.preventDefault(); openTab(newRequest()); }
  else if (mod && e.key.toLowerCase() === "w" && e.altKey) { e.preventDefault(); const t = activeTab(); if (t) closeTab(t); }
  else if (mod && e.key.toLowerCase() === "l") { e.preventDefault(); const u = $("url"); if (u) { u.focus(); u.select(); } }
  else if (e.ctrlKey && e.key === "Tab") {
    e.preventDefault();
    const i = S.tabs.findIndex((t) => t.id === S.active);
    const n = S.tabs[(i + (e.shiftKey ? -1 : 1) + S.tabs.length) % S.tabs.length];
    if (n) activate(n);
  }
});
// Dropping a Postman export or curl file anywhere imports it.
let dragDepth = 0;
addEventListener("dragenter", (e) => { if (e.dataTransfer && [...e.dataTransfer.types].includes("Files") && !dialogs.length) { dragDepth++; if (!$("dropHint")) document.body.append(h("div", { class: "drop-hint", id: "dropHint" }, "Drop a Postman export or curl file to import it")); } });
addEventListener("dragleave", () => { if (--dragDepth <= 0) { dragDepth = 0; const d = $("dropHint"); if (d) d.remove(); } });
addEventListener("dragover", (e) => e.preventDefault());
addEventListener("drop", async (e) => {
  e.preventDefault(); dragDepth = 0;
  const d = $("dropHint"); if (d) d.remove();
  const f = e.dataTransfer && e.dataTransfer.files[0];
  if (f && !dialogs.length) importDialog(await f.text());
});
addEventListener("beforeunload", (e) => { saveSession(); if (S.tabs.some((t) => t.origin && isDirty(t))) { e.preventDefault(); e.returnValue = ""; } });

function renderAll() {
  renderSidebar();
  renderEnvPick();
  renderOpenTabs();
  renderRequest();
  renderResponse();
}

async function boot() {
  $("kbdK").textContent = MOD + " K";
  try {
    const st = await api("GET", "/api/state");
    S.ws = st.workspace; S.ws.collections = S.ws.collections || []; S.ws.settings = S.ws.settings || {};
    S.history = st.history || []; S.info = st.info || {}; S.languages = st.languages || [];
  } catch (e) {
    document.body.replaceChildren(h("div", { class: "empty", style: { height: "100vh" } }, h("div", null, h("div", { class: "art" }, icon("alert")), h("h3", null, "Can't reach term-rest-client"), h("p", null, e.message), h("p", null, "Is term-rest-client --gui still running? Open the link it printed."))));
    return;
  }
  $("version").textContent = S.info.version && S.info.version !== "dev" ? "v" + S.info.version.replace(/^v/, "") : S.info.version || "";
  try {
    if (localStorage.getItem("trc.layout") === "stacked" || (!localStorage.getItem("trc.layout") && innerWidth < 1200)) $("workarea").classList.add("stacked");
    const sp = localStorage.getItem("trc.split"); if (sp) $("workarea").style.setProperty("--split", sp);
  } catch (e) { /* ignore */ }
  $("paletteBtn").onclick = palette;
  $("importBtn").onclick = () => importDialog();
  $("themeBtn").onclick = toggleTheme;
  $("layoutBtn").onclick = toggleLayout;
  $("settingsBtn").onclick = settingsDialog;
  setupGutter();
  paintChrome();
  restoreSession();
  renderAll();
}
if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot); else boot();
