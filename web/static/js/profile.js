// Profile page: view identity + workload, log out-of-scope work.
let myUid = null;

async function init() {
  const me = await API.get("/api/me").catch(() => null);
  if (!me || (!me.discord_id && !me.name && !me.profile)) { location.href = "/"; return; }
  const profile = me.profile || me;
  myUid = me.discord_id;

  document.getElementById("name").textContent = profile.name || me.name;
  document.getElementById("handle").textContent = (profile.discord_handle || me.handle) ? "@" + (profile.discord_handle || me.handle) : "";

  document.getElementById("manual-form").addEventListener("submit", addManual);
  document.getElementById("signout").addEventListener("click", signOut);
  await Promise.all([loadWorkload(), loadManual()]);
}

function signOut() {
  location.href = "/auth/logout";
}

async function loadWorkload() {
  const { people } = await API.get("/api/people");
  const me = people.find((p) => p.discord_id === myUid);
  const box = document.getElementById("workload");
  if (!me) { box.innerHTML = '<p class="muted">no workload data yet.</p>'; return; }
  const max = Math.max(1, ...people.map((p) => p.workload_min));
  const pct = Math.round((me.workload_min / max) * 100);
  box.innerHTML = "";
  box.appendChild(el("p", { style: "font-family:var(--main-font);font-weight:700" }, `${fmtMin(me.workload_min) || "0 min"} of chores so far 🧹`));
  box.appendChild(el("div", { class: "bar" }, el("span", { style: `width:${pct}%` })));
  if (me.capabilities?.length) box.appendChild(el("p", { class: "muted", style: "margin-top:8px;font-family:var(--main-font);font-size:.85rem" }, "skills:: " + me.capabilities.join(", ")));
}

async function loadManual() {
  const { entries } = await API.get("/api/me/manual-work");
  const list = document.getElementById("manual-list");
  list.innerHTML = "";
  if (!entries.length) { list.appendChild(el("li", { class: "muted" }, "no entries yet.")); return; }
  entries.forEach((e) =>
    list.appendChild(el("li", {},
      el("span", {}, (e.self_reported ? "⚡ " : "") + e.description),
      el("span", { class: "muted", style: "font-family:var(--main-font)" }, `${e.minutes} min`))));
}

async function addManual(ev) {
  ev.preventDefault();
  const body = {
    description: document.getElementById("desc").value.trim(),
    minutes: parseInt(document.getElementById("mins").value, 10),
  };
  try {
    await API.post("/api/me/manual-work", body);
    document.getElementById("desc").value = "";
    showToast("Logged ✓");
    await Promise.all([loadManual(), loadWorkload()]);
  } catch (e) { showError(e); }
}

init();
