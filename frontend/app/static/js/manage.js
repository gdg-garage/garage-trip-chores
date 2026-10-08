// Manage page: templates, custom create with 🌶️ urgency + head-count scaling, list.
let templates = [];
let selectedSkills = new Set();

function renderSpiciness(n) {
  document.getElementById("spiciness-val").textContent = n > 0 ? "🌶️".repeat(n) : "not urgent";
}

async function init() {
  const [{ templates: tpls }, { skills }] = await Promise.all([
    API.get("/api/templates"), API.get("/api/skills"),
  ]);
  templates = tpls;

  const tbox = document.getElementById("templates");
  templates.forEach((t) => tbox.appendChild(el("button", { type: "button", class: "chip", onclick: () => loadTemplate(t) }, t.name)));

  const sbox = document.getElementById("skills");
  skills.forEach((s) => {
    const chip = el("button", { type: "button", class: "chip", "aria-pressed": "false", onclick: () => toggleSkill(chip, s) }, s);
    sbox.appendChild(chip);
  });

  const spice = document.getElementById("spiciness");
  spice.addEventListener("input", (e) => renderSpiciness(+e.currentTarget.value));
  document.getElementById("chore-form").addEventListener("submit", submit);
  document.getElementById("headcount").addEventListener("input", recalcFromTemplate);

  connectWS(onMessage);
  loadCurrent();
}

function toggleSkill(chip, s) {
  if (selectedSkills.has(s)) { selectedSkills.delete(s); chip.classList.remove("on"); }
  else { selectedSkills.add(s); chip.classList.add("on"); }
  chip.setAttribute("aria-pressed", selectedSkills.has(s) ? "true" : "false");
}

function loadTemplate(t) {
  document.getElementById("form-title").textContent = "Create: " + t.name;
  document.getElementById("template_key").value = t.key;
  document.getElementById("name").value = t.name;
  document.getElementById("workers").value = t.necessary_workers;
  document.getElementById("time").value = t.estimated_time_min;
  document.getElementById("timeout").value = t.assignment_timeout_min;

  selectedSkills = new Set(t.necessary_capabilities || []);
  document.querySelectorAll("#skills .chip").forEach((c) => {
    const on = selectedSkills.has(c.textContent);
    c.classList.toggle("on", on);
    c.setAttribute("aria-pressed", on ? "true" : "false");
  });

  document.getElementById("headcount-wrap").hidden = !t.scales_with_headcount;
  recalcFromTemplate();
}

function currentTemplate() {
  return templates.find((t) => t.key === document.getElementById("template_key").value);
}

// Mirror the server's head-count scaling so the manager sees the real time.
function recalcFromTemplate() {
  const t = currentTemplate();
  if (!t || !t.scales_with_headcount) return;
  const hc = parseInt(document.getElementById("headcount").value, 10) || 0;
  document.getElementById("time").value = t.estimated_time_min + t.per_person_min * hc;
}

async function submit(ev) {
  ev.preventDefault();
  const btn = ev.submitter || ev.target.querySelector('button[type="submit"]');
  if (btn && btn.disabled) return;          // guard against rapid double-submits
  const deadlineRaw = document.getElementById("deadline").value;
  const body = {
    name: document.getElementById("name").value.trim(),
    necessary_workers: parseInt(document.getElementById("workers").value, 10),
    estimated_time_min: parseInt(document.getElementById("time").value, 10),
    assignment_timeout_min: parseInt(document.getElementById("timeout").value, 10),
    necessary_capabilities: [...selectedSkills],
    deadline: deadlineRaw ? new Date(deadlineRaw).toISOString() : null,
    spiciness: parseInt(document.getElementById("spiciness").value, 10) || 0,
    template_key: document.getElementById("template_key").value || null,
    headcount: parseInt(document.getElementById("headcount").value, 10) || null,
  };
  const label = btn ? btn.textContent : "";
  if (btn) { btn.disabled = true; btn.textContent = "Posting…"; }
  try {
    await API.post("/api/chores", body);
    showToast("Posted to the board ✓");
    resetForm();
  } catch (e) {
    showError(e);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = label; }
  }
}

function resetForm() {
  document.getElementById("chore-form").reset();
  document.getElementById("template_key").value = "";
  document.getElementById("form-title").textContent = "Create a chore";
  document.getElementById("headcount-wrap").hidden = true;
  selectedSkills.clear();
  document.getElementById("spiciness").value = 0;
  renderSpiciness(0);
  document.querySelectorAll("#skills .chip").forEach((c) => {
    c.classList.remove("on");
    c.setAttribute("aria-pressed", "false");
  });
}

function onMessage(msg) {
  if (["task_created", "task_done", "task_claimed", "task_updated"].includes(msg.type)) loadCurrent();
}

async function loadCurrent() {
  const { chores } = await API.get("/api/chores");
  const box = document.getElementById("current");
  box.innerHTML = "";
  if (!chores.length) { box.innerHTML = '<p class="muted">Board is empty.</p>'; return; }
  chores.forEach((c) => {
    const row = el("div", { class: `card chore ${c.urgent ? "urgent" : ""}`, style: "margin-bottom:10px" },
      el("div", { class: "row" },
        el("div", {},
          el("h3", { style: "margin:0" }, c.name),
          el("p", { class: "muted", style: "margin:.2em 0" },
            `${c.size} · ${fmtMin(c.estimated_time_min)} · ${c.claimed_count}/${c.necessary_workers} claimed` +
            (c.necessary_capabilities.length ? ` · needs ${c.necessary_capabilities.join(", ")}` : ""))),
        el("button", { class: "danger small", onclick: () => del(c.id) }, "Delete")));
    box.appendChild(row);
  });
}

async function del(id) {
  if (!confirm("Remove this chore from the board?")) return;
  try { await API.del(`/api/chores/${id}`); } catch (e) { showError(e); }
}

init();
