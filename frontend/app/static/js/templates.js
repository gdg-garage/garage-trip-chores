// Template manager: list + add/edit/delete chore templates.
let selectedSkills = new Set();
let scales = false;

async function init() {
  const { skills } = await API.get("/api/skills");
  const sbox = document.getElementById("skills");
  skills.forEach((s) => {
    const chip = el("button", { type: "button", class: "chip", "aria-pressed": "false", onclick: () => toggleSkill(chip, s) }, s);
    sbox.appendChild(chip);
  });

  document.getElementById("scales").addEventListener("click", (e) => {
    scales = !scales;
    e.currentTarget.classList.toggle("on", scales);
    e.currentTarget.setAttribute("aria-pressed", scales ? "true" : "false");
    document.getElementById("perperson-wrap").hidden = !scales;
  });
  document.getElementById("tpl-form").addEventListener("submit", save);
  document.getElementById("cancel-edit").addEventListener("click", resetForm);

  await load();
}

function toggleSkill(chip, s) {
  if (selectedSkills.has(s)) { selectedSkills.delete(s); chip.classList.remove("on"); }
  else { selectedSkills.add(s); chip.classList.add("on"); }
}

async function load() {
  const { templates } = await API.get("/api/templates");
  const box = document.getElementById("list");
  box.innerHTML = "";
  if (!templates.length) { box.innerHTML = '<p class="muted">No templates yet.</p>'; return; }
  templates.forEach((t) => {
    const details = `${t.necessary_workers} worker(s) · ${fmtMin(t.estimated_time_min)}` +
      (t.scales_with_headcount ? ` (+${t.per_person_min}/person)` : "") +
      (t.necessary_capabilities.length ? ` · needs ${t.necessary_capabilities.join(", ")}` : "");
    box.appendChild(el("div", { class: "card chore", style: "margin-bottom:10px" },
      el("div", { class: "row" },
        el("div", {},
          el("h3", { style: "margin:0" }, t.name),
          el("p", { class: "muted", style: "margin:.2em 0" }, details)),
        el("div", { class: "row", style: "gap:6px" },
          el("button", { class: "secondary small", onclick: () => edit(t) }, "✏️ Edit"),
          el("button", { class: "danger small", onclick: () => del(t) }, "Delete")))));
  });
}

function edit(t) {
  document.getElementById("edit-key").value = t.key;
  document.getElementById("form-title").textContent = "Edit: " + t.name;
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
  scales = t.scales_with_headcount;
  const scalesBtn = document.getElementById("scales");
  scalesBtn.classList.toggle("on", scales);
  scalesBtn.setAttribute("aria-pressed", scales ? "true" : "false");
  document.getElementById("perperson-wrap").hidden = !scales;
  document.getElementById("perperson").value = t.per_person_min;
  document.getElementById("cancel-edit").hidden = false;
  window.scrollTo({ top: 0, behavior: "smooth" });
}

function resetForm() {
  document.getElementById("tpl-form").reset();
  document.getElementById("edit-key").value = "";
  document.getElementById("form-title").textContent = "Add a template";
  selectedSkills.clear(); scales = false;
  document.querySelectorAll("#skills .chip, #scales").forEach((c) => {
    c.classList.remove("on");
    c.setAttribute("aria-pressed", "false");
  });
  document.getElementById("perperson-wrap").hidden = true;
  document.getElementById("cancel-edit").hidden = true;
}

async function save(ev) {
  ev.preventDefault();
  const body = {
    name: document.getElementById("name").value.trim(),
    necessary_workers: parseInt(document.getElementById("workers").value, 10),
    estimated_time_min: parseInt(document.getElementById("time").value, 10),
    assignment_timeout_min: parseInt(document.getElementById("timeout").value, 10),
    necessary_capabilities: [...selectedSkills],
    scales_with_headcount: scales,
    per_person_min: scales ? parseInt(document.getElementById("perperson").value, 10) || 0 : 0,
  };
  const key = document.getElementById("edit-key").value;
  try {
    if (key) await API.put(`/api/templates/${encodeURIComponent(key)}`, body);
    else await API.post("/api/templates", body);
    showToast("Template saved ✓");
    resetForm();
    await load();
  } catch (e) { showError(e); }
}

async function del(t) {
  if (!confirm(`Delete template “${t.name}”?`)) return;
  try { await API.del(`/api/templates/${encodeURIComponent(t.key)}`); await load(); }
  catch (e) { showError(e); }
}

init();
