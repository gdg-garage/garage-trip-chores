// Shared helpers: fetch wrappers, WebSocket connection, small DOM utils.

const API = {
  async get(path) {
    const r = await fetch(path, { headers: { "Accept": "application/json" } });
    if (!r.ok) throw await apiError(r);
    return r.json();
  },
  async send(method, path, body) {
    const r = await fetch(path, {
      method,
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!r.ok) throw await apiError(r);
    return r.json();
  },
  post(path, body) { return this.send("POST", path, body); },
  put(path, body) { return this.send("PUT", path, body); },
  del(path) { return this.send("DELETE", path); },
};

async function safeJson(r) { try { return await r.json(); } catch { return null; } }

// Build an Error that carries the HTTP status, so humanError can translate it.
async function apiError(r) {
  const e = new Error((await safeJson(r))?.detail || r.statusText || "Request failed");
  e.status = r.status;
  return e;
}

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") e.className = v;
    else if (k === "html") e.innerHTML = v;
    else if (k.startsWith("on") && typeof v === "function") e.addEventListener(k.slice(2), v);
    else if (v !== null && v !== undefined) e.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c == null) continue;
    e.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return e;
}

// Auto-reconnecting WebSocket with a heartbeat + watchdog so a silently-dropped
// connection (idle TV, proxy timeout, half-open socket) is detected and
// reconnected — reconnecting re-fetches a fresh snapshot, so anything missed
// during the gap shows up. `onMessage(data)` receives parsed JSON.
const WS_PING_MS = 20000;   // client ping cadence
const WS_STALE_MS = 45000;  // no traffic for this long => assume dead, reconnect
function connectWS(onMessage, onStatus) {
  let ws, closed = false, backoff = 500, lastRx = Date.now();
  let pingTimer, watchdog;
  const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";

  function clearTimers() { clearInterval(pingTimer); clearInterval(watchdog); }

  function open() {
    ws = new WebSocket(url);
    ws.onopen = () => {
      backoff = 500; lastRx = Date.now(); onStatus && onStatus(true);
      clearTimers();
      // Send a lightweight ping so proxies see traffic and the server can tell
      // we're alive; the server also pings us so lastRx stays fresh.
      pingTimer = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) { try { ws.send("ping"); } catch (e) {} }
      }, WS_PING_MS);
      // If nothing has arrived for a while, the socket is probably dead even if
      // no close event fired — force a reconnect.
      watchdog = setInterval(() => {
        if (Date.now() - lastRx > WS_STALE_MS) { try { ws.close(); } catch (e) {} }
      }, WS_PING_MS / 2);
    };
    ws.onclose = () => {
      clearTimers();
      onStatus && onStatus(false);
      if (!closed) setTimeout(open, backoff = Math.min(backoff * 2, 8000));
    };
    ws.onerror = () => ws.close();
    ws.onmessage = (ev) => {
      lastRx = Date.now();
      let data;
      try { data = JSON.parse(ev.data); } catch (e) { return; }
      if (data && data.type === "ping") return;  // heartbeat, not app data
      try { onMessage(data); } catch (e) { console.error(e); }
    };
  }
  open();
  return { close() { closed = true; clearTimers(); ws && ws.close(); } };
}

// Turn a raw API/network Error into a message a person can act on. We never
// surface the server's own string (status codes, "Unknown chore", OAuthError…).
function humanError(err) {
  if (err && (err.name === "TypeError" || /failed to fetch|networkerror/i.test(err.message || ""))) {
    return "Can't reach the server — check your connection and try again.";
  }
  const status = err && err.status;
  const low = (err && err.message ? String(err.message) : "").toLowerCase();

  if (status === 401 || low.includes("login required")) return "You've been signed out. Log in again to pick up where you left off.";
  if (status === 403) return "You don't have access to do that.";
  if (status === 404 || low.includes("unknown chore")) return "That chore isn't on the board anymore — it may already be done.";
  if (status === 409 || low.includes("already") || low.includes("conflict")) return "Someone just beat you to it — that one's taken.";
  if (low.includes("no eligible person")) return "Nobody's free and qualified for this one right now.";
  if (status >= 500 || low.includes("upstream") || low.includes("bad gateway")) return "The chores service is having a moment. Give it a few seconds and try again.";
  return "That didn't go through. Give it another try.";
}

// showToast(msg)                 → success (green)
// showToast(msg, 1500)           → success, custom duration
// showToast(msg, { error: true })→ problem (red)
function showToast(msg, opts) {
  const o = typeof opts === "number" ? { ms: opts } : (opts || {});
  const ms = o.ms || 3200;
  let t = document.querySelector(".toast");
  if (!t) { t = el("div", { class: "toast", role: "status", "aria-live": "polite" }); document.body.appendChild(t); }
  t.classList.toggle("error", !!o.error);
  t.textContent = msg;
  requestAnimationFrame(() => t.classList.add("show"));
  clearTimeout(t._h);
  t._h = setTimeout(() => t.classList.remove("show"), ms);
}

// Show a caught error as a friendly, red toast.
function showError(err) { console.error(err); showToast(humanError(err), { error: true }); }

// Announce a message to screen readers without any visible UI (e.g. a new chore
// arriving over the WebSocket). Uses a single shared visually-hidden live region.
function announce(msg) {
  let live = document.getElementById("a11y-live");
  if (!live) {
    live = el("div", { id: "a11y-live", class: "sr-only", "aria-live": "polite", "aria-atomic": "true" });
    document.body.appendChild(live);
  }
  live.textContent = "";                 // reset so identical consecutive messages re-announce
  requestAnimationFrame(() => (live.textContent = msg));
}

const fmtMin = (m) => (m == null ? "" : m >= 60 ? `${Math.round(m / 6) / 10} h` : `${m} min`);
