// Leaderboard: sortable table (default: time spent, descending). Live updates.
// Values come from the upstream /stats endpoint (worked_count / worked_min).
let rows = [];
let sortKey = "worked_min";
let sortDir = -1; // -1 desc, 1 asc

async function init() {
  document.querySelectorAll("th.sortable").forEach((th) =>
    th.addEventListener("click", () => setSort(th.dataset.sort)));
  await load();
  connectWS((msg) => {
    if (["snapshot", "task_done", "task_claimed", "task_created", "workload_updated", "profile_updated"].includes(msg.type)) load();
  });
}

async function load() {
  const data = await API.get("/api/leaderboard");
  rows = data.rows;
  render();
}

function setSort(key) {
  if (sortKey === key) { sortDir = -sortDir; }
  else { sortKey = key; sortDir = key === "name" ? 1 : -1; } // names default A→Z, numbers high→low
  render();
}

function sorted() {
  const r = [...rows];
  r.sort((a, b) => {
    let av = a[sortKey], bv = b[sortKey];
    if (sortKey === "name") { av = av.toLowerCase(); bv = bv.toLowerCase(); return av < bv ? -sortDir : av > bv ? sortDir : 0; }
    return (av - bv) * sortDir;
  });
  return r;
}

// Standing by effort (time, then chores, then name) — this is the person's
// *place*, so it travels with them no matter how the table is re-sorted.
function computeRanks() {
  const byEffort = [...rows].sort((a, b) =>
    (b.worked_min - a.worked_min) ||
    (b.worked_count - a.worked_count) ||
    a.name.toLowerCase().localeCompare(b.name.toLowerCase()));
  const m = new Map();
  byEffort.forEach((r, i) => m.set(r.discord_id, i + 1));
  return m;
}

const MEDALS = ["🥇", "🥈", "🥉"];

function render() {
  // arrows
  document.querySelectorAll("th.sortable").forEach((th) => {
    const a = th.querySelector(".arrow");
    a.textContent = th.dataset.sort === sortKey ? (sortDir === -1 ? "▼" : "▲") : "↕";
    a.style.opacity = th.dataset.sort === sortKey ? "1" : ".35";
  });

  const tbody = document.getElementById("rows");
  tbody.innerHTML = "";
  const list = sorted();
  document.getElementById("empty").hidden = list.length > 0;

  const rankOf = computeRanks();
  const maxTime = Math.max(1, ...rows.map((r) => r.worked_min));

  list.forEach((r) => {
    const rank = rankOf.get(r.discord_id);
    // Only crown people who've actually done something — no medal for 0 effort.
    const hasEffort = r.worked_min > 0 || r.worked_count > 0;
    const medal = hasEffort && rank <= 3 ? MEDALS[rank - 1] : null;

    const rankCell = el("td", { class: "rank" },
      medal
        ? el("span", { class: "medal" }, medal)
        : el("span", { class: hasEffort ? "" : "muted" }, "#" + rank));

    const active = r.assigned_count
      ? el("span", { class: "badge suggest", style: "margin-left:8px" }, `${r.assigned_count} in progress`)
      : null;

    // Effort bar — the purple gradient motif, scaled to the leader. An empty
    // track is an honest "hasn't done anything yet".
    const pct = Math.round((r.worked_min / maxTime) * 100);
    const bar = el("div", { class: "lb-bar" }, el("span", { style: `width:${pct}%` }));

    const nameCell = el("td", {},
      el("div", {}, el("strong", {}, r.name), active),
      bar);

    const cls = "clickable" + (hasEffort && rank === 1 ? " podium-1" : "");
    const tr = el("tr", { class: cls, onclick: () => (location.href = `/users/${encodeURIComponent(r.discord_id)}`) },
      rankCell, nameCell,
      el("td", { class: "num" }, String(r.worked_count)),
      el("td", { class: "num" }, fmtMin(r.worked_min) || "0 min"));
    tbody.appendChild(tr);
  });
}

init();
