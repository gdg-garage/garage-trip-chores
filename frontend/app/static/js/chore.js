// Chore detail: full info + complete ranked suggestion list, live-updated.
const id = window.TASK_ID;
let myUid = null;

async function init() {
  try { const { discord_id } = await API.get("/api/me"); myUid = discord_id; } catch {}
  await load();
  connectWS((msg) => {
    // Refresh when anything touches this chore or the workload shifts.
    const touchesThis = msg.chore && msg.chore.id === id;
    if (touchesThis || ["snapshot", "workload_updated", "task_assigned", "task_done", "task_updated", "profile_updated"].includes(msg.type)) load();
  });
}

async function load() {
  let data;
  try { data = await API.get(`/api/chores/${id}`); }
  catch (e) {
    document.getElementById("detail").innerHTML =
      `<div class="card"><p class="muted">This chore is no longer available.</p><p><a href="/feed">← Back</a></p></div>`;
    document.getElementById("suggestions").innerHTML = "";
    return;
  }
  renderDetail(data.chore);
  renderSuggestions(data.suggestions, data.chore);
}

function renderDetail(c) {
  const isDone = Boolean(c.completed);
  const iClaimed = (c.claimers || []).some((p) => p.discord_id === myUid);
  const myWorklog = (c.worklogs || []).find((w) => w.user_id === myUid);
  const hasWorked = Boolean(myWorklog);

  const badges = el("div", { class: "badges" },
    el("span", { class: `badge size-${c.size}` }, `${c.size} · ${fmtMin(c.estimated_time_min)}`),
    isDone ? el("span", { class: "badge claimed", style: "background:#0f5132;color:#d1e7dd;border-color:transparent" }, "✓ Completed") : null,
    c.is_delayed ? el("span", { class: "badge delayed", style: "background:#442b6a;color:#d8b4fe" }, `⏳ Scheduled: in ${c.minutes_to_publish || 0}m`) : null,
    c.self_reported ? el("span", { class: "badge", style: "background:#1e3a47;color:#7dd3fc" }, "⚡ Self-reported") : null,
    c.urgent && !isDone ? el("span", { class: "badge urgent" }, "URGENT") : null,
    el("span", { class: "badge" }, `${c.claimed_count}/${c.necessary_workers} claimed`),
    ...(c.necessary_capabilities || []).map((s) => el("span", { class: "badge skill" }, "needs " + s)),
    c.minutes_to_deadline != null && !isDone ? el("span", { class: "badge" }, `⏰ ${fmtMin(Math.max(0, c.minutes_to_deadline))} left`) : null,
  );

  let claimers;
  if ((c.claimers || []).length) {
    claimers = el("div", { style: "margin:.4em 0" },
      el("span", { class: "muted" }, isDone ? "Workers: " : "On it: "),
      ...c.claimers.map((p) => el("span", { class: "assignee" },
        p.name,
        !isDone ? el("button", { class: "assignee-x", title: "Remove assignment", onclick: () => unassign(p.discord_id, p.name) }, "✕") : null)));
  } else {
    claimers = el("p", { class: "muted" }, isDone ? "No workers recorded." : "Nobody yet — be the hero.");
  }

  // Work logs section for completed chores
  let worklogsSection = null;
  if (isDone && (c.worklogs || []).length) {
    worklogsSection = el("div", { style: "margin:8px 0;padding:10px;background:#151d2a;border-radius:10px;border:1px solid #38425f" },
      el("strong", { style: "display:block;margin-bottom:6px" }, "Work logged:"),
      ...c.worklogs.map((w) => el("div", { style: "display:flex;justify-content:space-between;align-items:center;padding:3px 0" },
        el("span", {}, `${w.user_id === myUid ? "You" : w.user_id}${w.self_reported ? " (self-reported)" : ""}`),
        el("span", { class: "muted" },
          `${w.time_spent_min} min `,
          (w.user_id === myUid || !myUid) ? el("button", { class: "ghost small", style: "padding:2px 6px;margin-left:6px", onclick: () => editTime(w.user_id, w.time_spent_min) }, "✎") : null))));
  }

  // Action buttons
  const buttons = [];
  if (!isDone) {
    const btn = iClaimed
      ? el("button", { class: "secondary", onclick: unclaim }, "✓ You're on it — tap to drop")
      : el("button", { onclick: (e) => claim(e.target) }, "Claim it");
    const done = el("button", { class: "ghost", onclick: markDone }, "Mark done");
    const auto = c.fully_claimed ? null : el("button", { class: "secondary", onclick: (e) => autoAssign(e.target) }, "🎯 Auto-assign best fit");
    buttons.push(btn, auto, done);
  } else {
    // Completed chore actions
    if (myUid && !hasWorked) {
      buttons.push(el("button", { class: "secondary", onclick: iHelped }, "🤝 I helped on this chore"));
    }
  }

  const card = el("div", { class: `card chore ${c.urgent && !isDone ? "urgent" : ""} ${isDone ? "done" : ""}` },
    el("h1", { style: "margin:.1em 0" }, c.name),
    badges,
    claimers,
    worklogsSection,
    buttons.length ? el("div", { class: "row", style: "gap:8px;margin-top:12px;flex-wrap:wrap" }, ...buttons) : null,
  );
  const wrap = document.getElementById("detail");
  wrap.innerHTML = ""; wrap.appendChild(card);
}

function renderSuggestions(sug, chore) {
  const isDone = Boolean(chore.completed);
  const top = new Set(sug.top || []);
  const list = document.getElementById("suggestions");
  list.innerHTML = "";

  if (isDone) {
    list.appendChild(el("li", { class: "muted" }, "This chore is completed! Great job team."));
    return;
  }

  const ranked = sug.ranked || [];
  if (!ranked.length) { list.appendChild(el("li", { class: "muted" }, "No eligible people found.")); return; }
  const max = Math.max(1, ...ranked.map((p) => p.workload_min));
  const claimerIds = new Set((chore.claimers || []).map((c) => c.discord_id));
  ranked.forEach((p) => {
    const pct = Math.round((p.workload_min / max) * 100);
    const reason = !p.eligible ? "missing skill" : "";
    let action = null;
    if (claimerIds.has(p.discord_id)) action = el("span", { class: "badge claimed" }, "✓ on it");
    else if (p.eligible && !chore.fully_claimed) action = el("button", { class: "small secondary", onclick: () => assignTo(p.discord_id) }, "Assign");
    list.appendChild(el("li", { style: "align-items:center;gap:12px;opacity:" + (p.eligible ? 1 : 0.45) },
      el("div", { style: "flex:1" },
        el("div", { style: "display:flex;justify-content:space-between;align-items:center" },
          el("span", {}, (top.has(p.discord_id) ? "⭐ " : "") + p.name +
            (p.discord_id === myUid ? " (you)" : "")),
          el("span", { class: "muted" }, reason || `${p.workload_min} min`)),
        el("div", { class: "bar" }, el("span", { style: `width:${pct}%` }))),
      action));
  });
}

async function claim(btn) {
  btn.disabled = true;
  try { const { ack } = await API.post(`/api/chores/${id}/claim`); showToast(ack); await load(); }
  catch (e) { showError(e); btn.disabled = false; }
}
async function unclaim() {
  if (!confirm("Drop this chore? It'll go back on the board for someone else.")) return;
  try { await API.post(`/api/chores/${id}/unclaim`); await load(); } catch (e) { showError(e); }
}
async function markDone() {
  if (!confirm("Mark this chore done?")) return;
  try {
    await API.post(`/api/chores/${id}/done`);
    showToast("Chore done! 🎊");
    await load();
  }
  catch (e) { showError(e); }
}
async function iHelped() {
  try {
    await API.post(`/api/chores/${id}/help`);
    showToast("Logged your help! 🤝");
    await load();
  } catch (e) { showError(e); }
}
async function editTime(userId, currentMin) {
  const raw = prompt("Minutes spent on this chore:", currentMin);
  if (raw == null) return;
  const mins = parseInt(raw, 10);
  if (isNaN(mins) || mins < 0) { showError(new Error("Enter a valid number of minutes.")); return; }
  try {
    await API.post(`/api/chores/${id}/time`, { discord_id: userId, time_spent_min: mins });
    showToast("Time updated ✓");
    await load();
  } catch (e) { showError(e); }
}
async function assignTo(discord_id) {
  try { const r = await API.post(`/api/chores/${id}/assign`, { discord_id }); showToast(r.ack); await load(); }
  catch (e) { showError(e); }
}
async function unassign(discord_id, name) {
  if (!confirm(`Remove ${name} from this chore?`)) return;
  try { await API.post(`/api/chores/${id}/unassign`, { discord_id }); showToast(`Removed ${name}`); await load(); }
  catch (e) { showError(e); }
}
async function autoAssign(btn) {
  btn.disabled = true;
  try { const r = await API.post(`/api/chores/${id}/assign`, {}); showToast(r.ack); await load(); }
  catch (e) { showError(e); btn.disabled = false; }
}

init();
