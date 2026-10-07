// TV dashboard: read-only live board + leaderboard, sound on new/urgent chores.
const state = { chores: new Map(), suggestions: new Map() };
let muted = false;
let audioCtx = null;

async function init() {
  document.getElementById("mute").addEventListener("click", toggleMute);
  tickClock(); setInterval(tickClock, 1000);
  setInterval(loadLeader, 15000);
  connectWS(onMessage, (up) => {
    const c = document.getElementById("conn");
    c.className = "conn " + (up ? "up" : "down");
    c.textContent = up ? "● live" : "● offline";
  });
  loadLeader();
}

function tickClock() {
  const d = new Date();
  document.getElementById("clock").textContent = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function toggleMute() {
  muted = !muted;
  document.getElementById("mute").textContent = muted ? "🔇 Muted" : "🔊 Sound on";
  if (!muted) ensureAudio(); // unlock audio on user gesture
}

function ensureAudio() {
  if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  if (audioCtx.state === "suspended") audioCtx.resume();
}

// Two-tone chime; higher/urgent = more attention.
function chime(urgent) {
  if (muted || !audioCtx) return;
  const now = audioCtx.currentTime;
  const notes = urgent ? [880, 1175, 880] : [660, 990];
  notes.forEach((f, i) => {
    const o = audioCtx.createOscillator(), g = audioCtx.createGain();
    o.frequency.value = f; o.type = "triangle";
    g.gain.setValueAtTime(0.001, now + i * 0.16);
    g.gain.exponentialRampToValueAtTime(0.25, now + i * 0.16 + 0.02);
    g.gain.exponentialRampToValueAtTime(0.001, now + i * 0.16 + 0.15);
    o.connect(g).connect(audioCtx.destination);
    o.start(now + i * 0.16); o.stop(now + i * 0.16 + 0.16);
  });
}

function onMessage(msg) {
  switch (msg.type) {
    case "snapshot":
      state.chores.clear();
      msg.chores.forEach((c) => state.chores.set(c.id, c));
      Object.entries(msg.suggestions || {}).forEach(([id, t]) => state.suggestions.set(+id, t));
      break;
    case "task_done":
      state.chores.delete(msg.chore?.id);
      break;
    case "task_created":
      if (msg.chore) { state.chores.set(msg.chore.id, msg.chore); chime(msg.chore.urgent); }
      if (msg.suggestions) state.suggestions.set(msg.chore.id, msg.suggestions);
      loadLeader();
      break;
    default:
      if (msg.chore && msg.chore.id != null) {
        if (msg.chore.active === false) state.chores.delete(msg.chore.id);
        else state.chores.set(msg.chore.id, msg.chore);
        if (msg.suggestions) state.suggestions.set(msg.chore.id, msg.suggestions);
      }
      if (msg.type === "workload_updated" || msg.type === "task_claimed") loadLeader();
  }
  render();
}

function render() {
  const wrap = document.getElementById("chores");
  wrap.innerHTML = "";
  const list = [...state.chores.values()].filter((c) => c.active !== false)
    .sort((a, b) => (a.urgent === b.urgent ? 0 : a.urgent ? -1 : 1) ||
      (b.estimated_time_min - a.estimated_time_min));
  document.getElementById("empty").hidden = list.length > 0;
  list.forEach((c) => wrap.appendChild(card(c)));
}

function card(c) {
  return el("div", { class: `card chore ${c.urgent ? "urgent" : ""}` },
    el("h3", {}, el("a", { href: `/chores/${c.id}`, style: "color:inherit" }, c.name)),
    el("div", { class: "badges" },
      el("span", { class: `badge size-${c.size}` }, `${c.size} · ${fmtMin(c.estimated_time_min)}`),
      c.urgent ? el("span", { class: "badge urgent" }, "URGENT") : null,
      el("span", { class: "badge" }, `${c.claimed_count}/${c.necessary_workers} claimed`),
      ...(c.necessary_capabilities || []).map((s) => el("span", { class: "badge skill" }, s))),
    (c.claimers || []).length
      ? el("p", { class: "on-it" }, "🙌 On it: ", el("strong", {}, c.claimers.map((p) => p.name).join(", ")))
      : null);
}

async function loadLeader() {
  // Workload comes from /stats, names from /users — each from its own upstream
  // source, joined here by discord_id.
  const [{ stats }, { users, children_count }] = await Promise.all([
    API.get("/api/stats"),
    API.get("/api/users"),
  ]);
  const nameById = new Map(users.map((u) => [u.discord_id, u.handle || u.discord_id]));
  const people = Object.entries(stats)
    .map(([id, s]) => ({ name: nameById.get(id) || id, workload_min: Math.round((s.total_min || 0) * 10) / 10 }))
    .sort((a, b) => b.workload_min - a.workload_min);
  document.getElementById("kids").textContent =
    `${users.length} adults · ${children_count} little ones (not assignable, but they add to the load 🍽️)`;
  const max = Math.max(1, ...people.map((p) => p.workload_min));
  const list = document.getElementById("leader");
  list.innerHTML = "";
  people.slice(0, 15).forEach((p) => {
    list.appendChild(el("li", {},
      el("div", { style: "flex:1" },
        el("div", { style: "display:flex;justify-content:space-between" },
          el("span", {}, p.name), el("span", { class: "muted" }, `${p.workload_min} min`)),
        el("div", { class: "bar" }, el("span", { style: `width:${Math.round((p.workload_min / max) * 100)}%` })))));
  });
}

init();
