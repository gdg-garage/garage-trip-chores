// Chore feed: live list, claim with funny ack, urgent + suggested highlighting.
const state = { chores: new Map(), suggestions: new Map(), myUid: null, myName: null };
// suggestions Map stores full objects: { top: [discord_id,…], ranked: [{discord_id, name,…}] }

async function init() {
  const { profile, discord_id } = await API.get("/api/me");
  if (!profile) { location.href = "/"; return; }
  state.myUid = discord_id;
  state.myName = profile.name;
  document.getElementById("greeting").textContent = `Hi ${profile.name} — grab a chore when you can 💪`;

  connectWS(onMessage, (up) => {
    const c = document.getElementById("conn");
    c.className = "conn " + (up ? "up" : "down");
    c.textContent = up ? "● live" : "● offline";
  });
}

function _storeSuggestions(id, raw) {
  // raw may be a top-id array (legacy snapshot shape) or a {top, ranked} object
  if (!raw) return;
  if (Array.isArray(raw)) {
    const prev = state.suggestions.get(id) || {};
    state.suggestions.set(id, { top: raw, ranked: prev.ranked || [] });
  } else {
    state.suggestions.set(id, raw);
  }
}

function onMessage(msg) {
  switch (msg.type) {
    case "snapshot":
      state.chores.clear(); state.suggestions.clear();
      msg.chores.forEach((c) => state.chores.set(c.id, c));
      // Snapshot ships suggestions as {id: top_array} — store as full objects
      // with empty ranked (the chip names load from top-3 cross-ref with person cache)
      Object.entries(msg.suggestions || {}).forEach(([id, val]) => _storeSuggestions(+id, val));
      break;
    case "task_done":
      state.chores.delete(msg.chore?.id);
      break;
    default:
      if (msg.chore && msg.chore.id != null) {
        if (msg.chore.active === false) state.chores.delete(msg.chore.id);
        else state.chores.set(msg.chore.id, msg.chore);
        if (msg.suggestions != null) _storeSuggestions(msg.chore.id, msg.suggestions);
        // Announce new chores (screen readers) and ping me if one wants me.
        if (msg.type === "task_created") {
          announce(`New chore: ${msg.chore.name}`);
          const sug = state.suggestions.get(msg.chore.id);
          const top = Array.isArray(sug) ? sug : (sug?.top || []);
          if (top.includes(state.myUid)) showToast("🔔 A new chore suggests YOU!");
        }
      }
  }
  render();
}

function sortChores(list) {
  return list.sort((a, b) =>
    (a.urgent === b.urgent ? 0 : a.urgent ? -1 : 1) ||
    ((a.minutes_to_deadline ?? 1e9) - (b.minutes_to_deadline ?? 1e9)) ||
    (b.estimated_time_min - a.estimated_time_min));
}

function render() {
  const wrap = document.getElementById("chores");
  wrap.innerHTML = "";
  const list = sortChores([...state.chores.values()].filter((c) => c.active !== false));
  document.getElementById("empty").hidden = list.length > 0;
  list.forEach((c) => wrap.appendChild(choreCard(c)));
}

function _topSuggestions(choreId) {
  const sug = state.suggestions.get(choreId);
  if (!sug) return [];
  // sug may be {top, ranked} (from task events) or just a top array (snapshot)
  const top = Array.isArray(sug) ? sug : (sug?.top || []);
  const ranked = sug?.ranked || [];
  // Build name-lookup from ranked when available
  const byId = Object.fromEntries(ranked.map((p) => [p.discord_id, p.name]));
  return top
    .filter((id) => id !== state.myUid)          // don't suggest yourself
    .slice(0, 3)
    .map((id) => ({ discord_id: id, name: byId[id] || null }));
}

function assignRow(c) {
  if (c.fully_claimed) return null;
  const topPeople = _topSuggestions(c.id);

  // Auto-assign button (always shown for unclaimed chores)
  const autoBtn = el("button", {
    class: "secondary small",
    onclick: (e) => autoAssignFeed(c.id, e.currentTarget),
  }, "🎯 Auto-assign");

  // Person chips — only when we have names from ranked data
  const namedChips = topPeople
    .filter((p) => p.name)                         // skip if name not yet resolved
    .map((p) => el("button", {
      class: "ghost small feed-assign-chip",
      title: `Assign to ${p.name}`,
      onclick: (e) => assignToFeed(c.id, p.discord_id, p.name, e.currentTarget),
    }, p.name.split(" ")[0].slice(0, 12)));         // first name, max 12 chars

  if (namedChips.length === 0 && topPeople.length > 0) {
    // Have top ids but no names yet (snapshot-only state) — show only auto-assign
    // and fetch ranked data to hydrate chips on next broadcast
    _fetchSuggestions(c.id);
  }

  const children = [autoBtn, ...namedChips];
  return el("div", { class: "feed-assign-row" }, ...children);
}

async function _fetchSuggestions(choreId) {
  // Lazily fetch ranked suggestions when only top-ids are in state (snapshot case).
  // The result updates state so the next render shows named chips.
  try {
    const sug = await API.get(`/api/chores/${choreId}/suggestions`);
    state.suggestions.set(choreId, sug);
    render();
  } catch {}
}

function choreCard(c) {
  const sug = state.suggestions.get(c.id);
  const top = Array.isArray(sug) ? sug : (sug?.top || []);
  const suggested = top.includes(state.myUid);
  const iClaimed = (c.claimers || []).some((p) => p.discord_id === state.myUid);

  const badges = el("div", { class: "badges" },
    el("span", { class: `badge size-${c.size}` }, `${c.size} · ${fmtMin(c.estimated_time_min)}`),
    c.urgent ? el("span", { class: "badge urgent" }, "URGENT") : null,
    suggested ? el("span", { class: "badge suggest" }, "⭐ Suggested for you") : null,
    ...(c.necessary_capabilities || []).map((s) => el("span", { class: "badge skill" }, "needs " + s)),
    el("span", { class: "badge" }, `${c.claimed_count}/${c.necessary_workers} claimed`),
    c.minutes_to_deadline != null ? el("span", { class: "badge" }, `⏰ ${fmtMin(Math.max(0, c.minutes_to_deadline))} left`) : null,
  );

  const claimers = (c.claimers || []).length
    ? el("p", { class: "muted", style: "margin:.3em 0 0;font-size:.85rem" }, "On it: " + c.claimers.map((p) => p.name).join(", "))
    : null;

  const btn = iClaimed
    ? el("button", { class: "secondary", onclick: () => unclaim(c.id) }, "✓ You're on it — tap to drop")
    : el("button", { onclick: (e) => claim(c.id, e.currentTarget) }, "Claim it");

  const done = el("button", { class: "ghost small", onclick: (e) => markDone(c.id, e.currentTarget) }, "Mark done");

  const assign = assignRow(c);

  const card = el("div", { class: `card chore ${c.urgent ? "urgent" : ""} ${suggested ? "suggested" : ""}` },
    el("h3", {}, el("a", { href: `/chores/${c.id}`, style: "color:inherit" }, c.name)),
    badges,
    claimers,
    el("div", { class: "row", style: "margin-top:12px;gap:8px" }, btn, done),
    assign,
  );
  return card;
}

async function claim(id, btn) {
  btn.disabled = true;
  try {
    const { ack } = await API.post(`/api/chores/${id}/claim`);
    showToast(ack);
  } catch (e) { showError(e); btn.disabled = false; }
}
async function unclaim(id) {
  if (!confirm("Drop this chore? It'll go back on the board for someone else.")) return;
  try { await API.post(`/api/chores/${id}/unclaim`); } catch (e) { showError(e); }
}
async function markDone(id, btn) {
  if (!confirm("Mark this chore done? This can't be undone.")) return;
  if (btn) btn.disabled = true;
  try { await API.post(`/api/chores/${id}/done`); showToast("Nice — chore done! 🎊"); }
  catch (e) { showError(e); if (btn) btn.disabled = false; }
}
async function autoAssignFeed(id, btn) {
  btn.disabled = true;
  try {
    const r = await API.post(`/api/chores/${id}/assign`, {});
    showToast(r.ack);
  } catch (e) { showError(e); btn.disabled = false; }
}
async function assignToFeed(id, discord_id, name, btn) {
  btn.disabled = true;
  try {
    const r = await API.post(`/api/chores/${id}/assign`, { discord_id });
    showToast(r.ack);
  } catch (e) { showError(e); btn.disabled = false; }
}

init();
