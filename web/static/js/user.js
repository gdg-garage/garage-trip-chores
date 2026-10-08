// User detail: chores in progress (top) and completed, live-updated.
const uid = window.USER_ID;

async function init() {
  await load();
  connectWS((msg) => {
    if (["snapshot", "task_done", "task_claimed", "task_created", "profile_updated", "workload_updated"].includes(msg.type)) load();
  });
}

async function load() {
  let d;
  try {
    d = await API.get(`/api/users/${encodeURIComponent(uid)}`);
  } catch (e) {
    document.getElementById("sections").hidden = true;
    document.getElementById("head").innerHTML =
      '<div class="card"><p class="muted">Couldn\'t load this member right now.</p>' +
      '<p><a href="/leaderboard">← Back to leaderboard</a></p></div>';
    return;
  }
  const head = document.getElementById("head");
  head.innerHTML = "";
  head.appendChild(el("div", { class: "card" },
    el("h1", { style: "margin:.1em 0" }, d.name),
    d.handle ? el("p", { class: "muted", style: "margin:0" }, "@" + d.handle) : null,
    el("p", { class: "muted", style: "margin:.4em 0 0" },
      `${d.performed.length} done · ${fmtMin(d.time_spent_min) || "0 min"} spent · ${d.performing.length} in progress`)));

  document.getElementById("sections").hidden = false;
  document.getElementById("h-performing").textContent = `in_progress::chores; (${d.performing.length})`;
  document.getElementById("h-performed").textContent = `completed::chores; (${d.performed.length})`;
  renderList("performing", d.performing, "not working on anything right now.");
  renderList("performed", d.performed, "nothing completed yet. 🧹");
}

function renderList(id, chores, emptyMsg) {
  const box = document.getElementById(id);
  box.innerHTML = "";
  if (!chores.length) { box.appendChild(el("p", { class: "muted" }, emptyMsg)); return; }
  chores.forEach((c) => {
    const badges = el("div", { class: "badges" },
      el("span", { class: `badge size-${c.size}` }, `size::${c.size} · ${fmtMin(c.estimated_time_min)}`),
      c.urgent && !c.completed ? el("span", { class: "badge urgent" }, "urgent::🔥") : null,
      c.is_delayed ? el("span", { class: "badge delayed", style: "background:#442b6a;color:#d8b4fe" }, `⏳ in_${c.minutes_to_publish || 0}m;`) : null,
      c.self_reported ? el("span", { class: "badge", style: "background:#1e3a47;color:#7dd3fc" }, "⚡ self_reported;") : null,
      ...(c.necessary_capabilities || []).map((s) => el("span", { class: "badge skill" }, "needs::" + s)),
      c.completed ? el("span", { class: "badge claimed" }, "✓ done;") : null,
      c.completed ? el("span", { class: "badge" }, `⏱ ${fmtMin(c.total_time_min) || "0 min"} spent`) : null,
      c.completed ? el("button", { class: "ghost small", title: "Override time spent", onclick: () => editTime(c) }, "✎") : null);
    box.appendChild(el("div", { class: `card chore ${c.urgent && !c.completed ? "urgent" : ""} ${c.completed ? "done" : ""}` },
      el("h3", { style: "margin:0" }, el("a", { href: `/chores/${c.id}`, style: "color:inherit" }, c.name)),
      badges));
  });
}

async function editTime(c) {
  const raw = prompt(`Time ${c.name} — minutes spent:`, c.total_time_min);
  if (raw == null) return;                       // cancelled
  const mins = parseInt(raw, 10);
  if (isNaN(mins) || mins < 0) { showError(new Error("Enter a non-negative number of minutes.")); return; }
  try {
    await API.post(`/api/chores/${c.id}/time`, { discord_id: uid, time_spent_min: mins });
    showToast("Time updated ✓");
    await load();
  } catch (e) { showError(e); }
}

init();
