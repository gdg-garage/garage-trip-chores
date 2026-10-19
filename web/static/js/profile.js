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
  document.getElementById("api-key-form").addEventListener("submit", createApiKey);
  document.getElementById("copy-key-btn").addEventListener("click", copyNewKey);
  document.getElementById("signout").addEventListener("click", signOut);
  await Promise.all([loadWorkload(), loadManual(), loadApiKeys()]);
}

function signOut() {
  location.href = "/auth/logout";
}

async function loadApiKeys() {
  const container = document.getElementById("api-keys-list");
  if (!container) return;
  try {
    const res = await API.get("/api/me/api-keys");
    const keys = res?.api_keys || [];
    container.innerHTML = "";
    if (keys.length === 0) {
      container.innerHTML = '<p class="muted" style="font-size:.85rem">no API keys created yet.</p>';
      return;
    }
    const ul = el("ul", { class: "leader", style: "margin:0" });
    keys.forEach((k) => {
      const lastUsed = k.last_used ? `last used: ${new Date(k.last_used).toLocaleDateString()}` : "never used";
      const li = el("li", { style: "display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid var(--panel-border)" },
        el("div", {},
          el("strong", { style: "font-family:var(--main-font)" }, k.name || "API Key"),
          el("span", { class: "badge", style: "margin-left:8px;font-family:monospace;font-size:.8rem" }, k.key),
          el("div", { class: "muted", style: "font-size:.78rem;margin-top:2px" }, `${lastUsed} · created ${new Date(k.created).toLocaleDateString()}`)
        ),
        el("button", {
          class: "ghost small",
          style: "color:#f87171",
          onclick: () => deleteApiKey(k.id, k.name),
        }, "revoke")
      );
      ul.appendChild(li);
    });
    container.appendChild(ul);
  } catch (err) {
    console.debug("Failed to load api keys", err);
  }
}

async function createApiKey(ev) {
  ev.preventDefault();
  const nameInput = document.getElementById("key-name");
  const name = nameInput.value.trim() || "API Key";
  try {
    const res = await API.post("/api/me/api-keys", { name });
    nameInput.value = "";
    const box = document.getElementById("new-key-box");
    const valInput = document.getElementById("new-key-value");
    if (box && valInput && res.key) {
      valInput.value = res.key;
      box.style.display = "block";
    }
    showToast("API Key generated ✓");
    await loadApiKeys();
  } catch (err) {
    showError(err);
  }
}

async function copyNewKey() {
  const valInput = document.getElementById("new-key-value");
  if (!valInput || !valInput.value) return;
  try {
    await navigator.clipboard.writeText(valInput.value);
    showToast("Copied to clipboard! 📋");
  } catch (e) {
    valInput.select();
    document.execCommand("copy");
    showToast("Copied! 📋");
  }
}

async function deleteApiKey(id, name) {
  if (!confirm(`Revoke API key "${name || id}"? Any script using it will lose access.`)) return;
  try {
    await API.delete(`/api/me/api-keys/${id}`);
    showToast("Key revoked ✓");
    await loadApiKeys();
  } catch (err) {
    showError(err);
  }
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
