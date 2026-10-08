// Manage page: templates, custom create with self-reported, delay, direct assignment, urgency + head-count scaling, list.
let templates = [];
let users = [];
let selectedSkills = new Set();
let currentFilter = "all"; // "all" | "active" | "scheduled"

function renderSpiciness(n) {
  document.getElementById("spiciness-val").textContent = n > 0 ? "🌶️".repeat(n) : "not urgent";
}

async function init() {
  const [{ templates: tpls }, { skills }, { users: uList }] = await Promise.all([
    API.get("/api/templates"),
    API.get("/api/skills"),
    API.get("/api/users").catch(() => ({ users: [] })),
  ]);
  templates = tpls;
  users = uList || [];

  const tbox = document.getElementById("templates");
  templates.forEach((t) => tbox.appendChild(el("button", { type: "button", class: "chip", onclick: () => loadTemplate(t) }, t.name)));

  const sbox = document.getElementById("skills");
  skills.forEach((s) => {
    const chip = el("button", { type: "button", class: "chip", "aria-pressed": "false", onclick: () => toggleSkill(chip, s) }, s);
    sbox.appendChild(chip);
  });

  // Populate direct assignee dropdown
  const assigneeSelect = document.getElementById("assignee");
  if (assigneeSelect) {
    users.forEach((u) => {
      const opt = el("option", { value: u.discord_id }, u.handle || u.discord_id);
      assigneeSelect.appendChild(opt);
    });
  }

  const spice = document.getElementById("spiciness");
  spice.addEventListener("input", (e) => renderSpiciness(+e.currentTarget.value));
  document.getElementById("chore-form").addEventListener("submit", submit);
  document.getElementById("headcount").addEventListener("input", recalcFromTemplate);

  // Self-reported toggle
  const selfRep = document.getElementById("self_reported");
  if (selfRep) {
    selfRep.addEventListener("change", (e) => toggleSelfReported(e.target.checked));
  }

  // Delay select
  const delaySel = document.getElementById("delay_sel");
  const delayCustom = document.getElementById("delay_custom");
  if (delaySel && delayCustom) {
    delaySel.addEventListener("change", () => {
      delayCustom.hidden = delaySel.value !== "custom";
      if (!delayCustom.hidden) delayCustom.focus();
    });
  }

  // AI Summary button
  const summaryBtn = document.getElementById("btn-summary");
  if (summaryBtn) {
    summaryBtn.addEventListener("click", triggerSummary);
  }

  // Filter tabs
  const tabAll = document.getElementById("tab-all");
  const tabActive = document.getElementById("tab-active");
  const tabSched = document.getElementById("tab-scheduled");
  if (tabAll && tabActive && tabSched) {
    tabAll.addEventListener("click", () => setFilter("all"));
    tabActive.addEventListener("click", () => setFilter("active"));
    tabSched.addEventListener("click", () => setFilter("scheduled"));
  }

  connectWS(onMessage);
  loadCurrent();
}

function setFilter(filter) {
  currentFilter = filter;
  document.getElementById("tab-all").classList.toggle("on", filter === "all");
  document.getElementById("tab-active").classList.toggle("on", filter === "active");
  document.getElementById("tab-scheduled").classList.toggle("on", filter === "scheduled");
  loadCurrent();
}

function toggleSelfReported(checked) {
  const workersWrap = document.getElementById("workers-wrap");
  const assigneeDelayWrap = document.getElementById("assignee-delay-wrap");
  const timeoutHeadcountWrap = document.getElementById("timeout-headcount-wrap");
  const skillsWrap = document.getElementById("skills-wrap");
  const deadlineUrgencyWrap = document.getElementById("deadline-urgency-wrap");
  const submitBtn = document.getElementById("btn-submit");

  if (checked) {
    if (workersWrap) workersWrap.hidden = true;
    if (assigneeDelayWrap) assigneeDelayWrap.hidden = true;
    if (timeoutHeadcountWrap) timeoutHeadcountWrap.hidden = true;
    if (skillsWrap) skillsWrap.hidden = true;
    if (deadlineUrgencyWrap) deadlineUrgencyWrap.hidden = true;
    document.getElementById("workers").value = 1;
    if (submitBtn) submitBtn.textContent = "⚡ Save completed chore";
  } else {
    if (workersWrap) workersWrap.hidden = false;
    if (assigneeDelayWrap) assigneeDelayWrap.hidden = false;
    if (timeoutHeadcountWrap) timeoutHeadcountWrap.hidden = false;
    if (skillsWrap) skillsWrap.hidden = false;
    if (deadlineUrgencyWrap) deadlineUrgencyWrap.hidden = false;
    if (submitBtn) submitBtn.textContent = "Post chore to the board";
  }
}

async function triggerSummary() {
  const btn = document.getElementById("btn-summary");
  if (!confirm("Trigger AI chore summary and publish it to the Discord channel?")) return;
  if (btn) { btn.disabled = true; btn.textContent = "Generating summary…"; }
  try {
    const res = await API.post("/api/summary");
    showToast(res.message || "AI summary published to Discord! ✨");
  } catch (err) {
    showError(err);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = "✨ AI Summary to Discord"; }
  }
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

function getDelayMinutes() {
  const delaySel = document.getElementById("delay_sel");
  if (!delaySel) return 0;
  if (delaySel.value === "custom") {
    const custom = parseInt(document.getElementById("delay_custom").value, 10);
    return isNaN(custom) || custom < 0 ? 0 : custom;
  }
  return parseInt(delaySel.value, 10) || 0;
}

async function submit(ev) {
  ev.preventDefault();
  const btn = ev.submitter || ev.target.querySelector('button[type="submit"]');
  if (btn && btn.disabled) return;          // guard against rapid double-submits

  const selfReported = document.getElementById("self_reported")?.checked || false;
  const delayMin = selfReported ? 0 : getDelayMinutes();
  const assigneeId = selfReported ? null : (document.getElementById("assignee")?.value || null);
  const deadlineRaw = selfReported ? null : document.getElementById("deadline").value;

  const body = {
    name: document.getElementById("name").value.trim(),
    necessary_workers: parseInt(document.getElementById("workers").value, 10) || 1,
    estimated_time_min: parseInt(document.getElementById("time").value, 10) || 10,
    assignment_timeout_min: parseInt(document.getElementById("timeout").value, 10) || 15,
    necessary_capabilities: selfReported ? [] : [...selectedSkills],
    deadline: deadlineRaw ? new Date(deadlineRaw).toISOString() : null,
    spiciness: selfReported ? 0 : (parseInt(document.getElementById("spiciness").value, 10) || 0),
    template_key: document.getElementById("template_key").value || null,
    headcount: parseInt(document.getElementById("headcount").value, 10) || null,
    delay_min: delayMin,
    self_reported: selfReported,
    assignee_id: assigneeId,
  };

  const label = btn ? btn.textContent : "";
  if (btn) { btn.disabled = true; btn.textContent = "Saving…"; }
  try {
    await API.post("/api/chores", body);
    showToast(selfReported ? "Self-reported chore logged! ⚡" : (delayMin > 0 ? "Chore scheduled with delay! ⏳" : "Posted to the board ✓"));
    resetForm();
    await loadCurrent();
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
  document.getElementById("delay_custom").hidden = true;
  selectedSkills.clear();
  document.getElementById("spiciness").value = 0;
  renderSpiciness(0);
  document.querySelectorAll("#skills .chip").forEach((c) => {
    c.classList.remove("on");
    c.setAttribute("aria-pressed", "false");
  });
  toggleSelfReported(false);
}

function onMessage(msg) {
  if (["task_created", "task_done", "task_claimed", "task_updated"].includes(msg.type)) loadCurrent();
}

async function loadCurrent() {
  const { chores } = await API.get("/api/chores");
  const box = document.getElementById("current");
  if (!box) return;

  const totalCount = chores.length;
  const schedCount = chores.filter((c) => c.is_delayed).length;
  const activeCount = chores.filter((c) => !c.is_delayed).length;

  const countAll = document.getElementById("count-all");
  const countActive = document.getElementById("count-active");
  const countSched = document.getElementById("count-scheduled");
  if (countAll) countAll.textContent = totalCount;
  if (countActive) countActive.textContent = activeCount;
  if (countSched) countSched.textContent = schedCount;

  let displayList = chores;
  if (currentFilter === "active") displayList = chores.filter((c) => !c.is_delayed);
  else if (currentFilter === "scheduled") displayList = chores.filter((c) => c.is_delayed);

  box.innerHTML = "";
  if (!displayList.length) {
    box.innerHTML = `<p class="muted">${currentFilter === "scheduled" ? "No scheduled chores queued." : "Board is empty."}</p>`;
    return;
  }

  displayList.forEach((c) => {
    const badges = el("div", { class: "badges", style: "margin:4px 0" },
      el("span", { class: `badge size-${c.size}` }, `${c.size} · ${fmtMin(c.estimated_time_min)}`),
      c.is_delayed ? el("span", { class: "badge delayed", style: "background:#442b6a;color:#d8b4fe" }, `⏳ Scheduled: publishes in ${c.minutes_to_publish || 0}m`) : null,
      c.self_reported ? el("span", { class: "badge", style: "background:#1e3a47;color:#7dd3fc" }, "⚡ Self-reported") : null,
      c.urgent ? el("span", { class: "badge urgent" }, "URGENT") : null,
      el("span", { class: "badge" }, `${c.claimed_count}/${c.necessary_workers} claimed`),
      ...(c.necessary_capabilities || []).map((s) => el("span", { class: "badge skill" }, "needs " + s)),
    );

    const row = el("div", { class: `card chore ${c.urgent ? "urgent" : ""} ${c.is_delayed ? "delayed" : ""}`, style: "margin-bottom:10px" },
      el("div", { class: "row" },
        el("div", {},
          el("h3", { style: "margin:0" }, el("a", { href: `/chores/${c.id}`, style: "color:inherit" }, c.name)),
          badges,
          el("p", { class: "muted", style: "margin:.2em 0;font-size:.85rem" },
            c.claimers?.length ? "On it: " + c.claimers.map((p) => p.name).join(", ") : "No workers assigned yet")),
        el("button", { class: "danger small", onclick: () => del(c.id) }, "Delete")));
    box.appendChild(row);
  });
}

async function del(id) {
  if (!confirm("Remove this chore from the board?")) return;
  try { await API.del(`/api/chores/${id}`); } catch (e) { showError(e); }
}

init();
