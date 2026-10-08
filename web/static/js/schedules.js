// Scheduled Tasks Manager: list, add, edit, toggle, run, and delete scheduled recurring chores.

let selectedSkills = new Set();
let currentTab = "all";
let allSchedules = [];
let allTemplates = [];

async function init() {
  // Load skills
  try {
    const sData = await API.get("/api/skills").catch(() => []);
    const skills = Array.isArray(sData) ? sData : (sData.skills || []);
    const sbox = document.getElementById("skills");
    if (sbox) {
      sbox.innerHTML = "";
      skills.forEach((s) => {
        const chip = el("button", {
          type: "button",
          class: "chip",
          "aria-pressed": "false",
          onclick: () => toggleSkill(chip, s)
        }, s);
        sbox.appendChild(chip);
      });
    }
  } catch (err) {
    console.error("Failed to load skills", err);
  }

  // Load assignees
  try {
    const uData = await API.get("/api/users").catch(() => ({}));
    const users = Array.isArray(uData) ? uData : (uData.users || []);
    const aSelect = document.getElementById("assignee");
    if (aSelect) {
      users.forEach((u) => {
        const opt = el("option", { value: u.discord_id }, u.name || u.handle || u.discord_id);
        aSelect.appendChild(opt);
      });
    }
  } catch (err) {
    console.error("Failed to load users", err);
  }

  // Load templates for quick-fill
  try {
    const tData = await API.get("/api/templates").catch(() => []);
    allTemplates = Array.isArray(tData) ? tData : (tData.templates || []);
    const tSelect = document.getElementById("template-select");
    if (tSelect) {
      allTemplates.forEach((t) => {
        const opt = el("option", { value: t.key }, t.name + " (" + t.estimated_time_min + "m)");
        tSelect.appendChild(opt);
      });
      tSelect.addEventListener("change", () => {
        const key = tSelect.value;
        if (!key) return;
        const found = allTemplates.find((t) => t.key === key);
        if (found) prefillFromTemplate(found);
      });
    }
  } catch (err) {
    console.error("Failed to load templates", err);
  }

  let currentUser = null;
  try {
    const me = await API.get("/api/me");
    currentUser = me;
    if (me && me.discord_id) {
      const creatorInput = document.getElementById("creator_id");
      if (creatorInput) creatorInput.value = me.discord_id;
      const creatorDisplay = document.getElementById("creator-display");
      if (creatorDisplay) creatorDisplay.textContent = me.name || me.handle || me.discord_id;
    }
  } catch (err) {
    // Tablet session
  }

  // Populate creator dropdown (for tablet sessions)
  const creatorSelect = document.getElementById("schedule-creator-select");
  if (creatorSelect) {
    users.forEach((u) => {
      const opt = el("option", { value: u.discord_id }, u.name || u.handle || u.discord_id);
      creatorSelect.appendChild(opt);
    });
    if (!currentUser) {
      const wrap = document.getElementById("schedule-creator-wrap");
      if (wrap) wrap.hidden = false;
    }
    creatorSelect.addEventListener("change", () => {
      const val = creatorSelect.value;
      const creatorInput = document.getElementById("creator_id");
      if (creatorInput) creatorInput.value = val;
      const creatorDisplay = document.getElementById("creator-display");
      if (creatorDisplay) {
        creatorDisplay.textContent = val ? creatorSelect.options[creatorSelect.selectedIndex].text : "Select your name";
      }
    });
  }

  // Preset selector
  const presetSel = document.getElementById("cron-preset");
  const cronInput = document.getElementById("cron_expr");
  if (presetSel && cronInput) {
    presetSel.addEventListener("change", () => {
      if (presetSel.value !== "custom") {
        cronInput.value = presetSel.value;
        updateCronPreview();
      }
    });
    cronInput.addEventListener("input", () => {
      // Check if matches any preset
      const match = Array.from(presetSel.options).find((opt) => opt.value === cronInput.value.trim());
      presetSel.value = match ? match.value : "custom";
      updateCronPreview();
    });
  }

  // Filter tabs
  setupTabs();

  // Form listeners
  const form = document.getElementById("schedule-form");
  if (form) form.addEventListener("submit", saveSchedule);
  const cancelBtn = document.getElementById("cancel-edit");
  if (cancelBtn) cancelBtn.addEventListener("click", resetForm);

  updateCronPreview();
  await loadSchedules();
}

function setupTabs() {
  const tabs = {
    all: document.getElementById("tab-all"),
    active: document.getElementById("tab-active"),
    paused: document.getElementById("tab-paused"),
  };

  Object.entries(tabs).forEach(([key, btn]) => {
    if (!btn) return;
    btn.addEventListener("click", () => {
      currentTab = key;
      Object.values(tabs).forEach((b) => b && b.classList.remove("on"));
      btn.classList.add("on");
      renderSchedules();
    });
  });
}

function toggleSkill(chip, s) {
  if (selectedSkills.has(s)) {
    selectedSkills.delete(s);
    chip.classList.remove("on");
    chip.setAttribute("aria-pressed", "false");
  } else {
    selectedSkills.add(s);
    chip.classList.add("on");
    chip.setAttribute("aria-pressed", "true");
  }
}

function prefillFromTemplate(t) {
  document.getElementById("name").value = t.name;
  document.getElementById("template_key").value = t.key;
  document.getElementById("workers").value = t.necessary_workers || 1;
  document.getElementById("time").value = t.estimated_time_min || 10;
  document.getElementById("timeout").value = t.assignment_timeout_min || 15;

  selectedSkills = new Set(t.necessary_capabilities || []);
  document.querySelectorAll("#skills .chip").forEach((chip) => {
    const on = selectedSkills.has(chip.textContent);
    chip.classList.toggle("on", on);
    chip.setAttribute("aria-pressed", on ? "true" : "false");
  });
  showToast(`Loaded "${t.name}" settings into form`);
}

function updateCronPreview() {
  const expr = document.getElementById("cron_expr").value.trim();
  const preview = document.getElementById("cron-desc-preview");
  if (!preview) return;

  const presets = {
    "0 9 * * *": "Every morning at 09:00",
    "0 8 * * *": "Every morning at 08:00",
    "0 13 * * *": "Every afternoon at 13:00",
    "0 17 * * *": "Every afternoon at 17:00",
    "0 20 * * *": "Every evening at 20:00",
    "0 23 * * *": "Every night at 23:00",
    "0 */2 * * *": "Every 2 hours",
    "0 0 * * *": "Every day at midnight",
    "@daily": "Every day at midnight",
    "@hourly": "Every hour",
  };

  if (presets[expr]) {
    preview.textContent = presets[expr];
    return;
  }

  const parts = expr.split(/\s+/);
  if (parts.length === 5) {
    const [m, h, dom, mon, dow] = parts;
    if (!isNaN(m) && !isNaN(h) && dom === "*" && mon === "*") {
      const pad = (n) => String(n).padStart(2, "0");
      if (dow === "*") {
        preview.textContent = `Every day at ${pad(h)}:${pad(m)}`;
        return;
      }
      if (dow === "1-5") {
        preview.textContent = `Mon–Fri at ${pad(h)}:${pad(m)}`;
        return;
      }
      if (dow === "0,6" || dow === "6,0") {
        preview.textContent = `Weekends at ${pad(h)}:${pad(m)}`;
        return;
      }
    }
  }

  preview.textContent = "Cron: " + expr;
}

async function loadSchedules() {
  const container = document.getElementById("schedules-list");
  try {
    const data = await API.get("/api/schedules");
    allSchedules = Array.isArray(data) ? data : (data.schedules || []);
    renderSchedules();
  } catch (err) {
    console.error("Failed to load schedules", err);
    if (container) {
      container.innerHTML = '<p class="muted error">Failed to load scheduled tasks.</p>';
    }
  }
}

function renderSchedules() {
  const container = document.getElementById("schedules-list");
  if (!container) return;

  const countAll = allSchedules.length;
  const countActive = allSchedules.filter((s) => s.enabled).length;
  const countPaused = allSchedules.filter((s) => !s.enabled).length;

  const countAllEl = document.getElementById("count-all");
  const countActiveEl = document.getElementById("count-active");
  const countPausedEl = document.getElementById("count-paused");

  if (countAllEl) countAllEl.textContent = countAll;
  if (countActiveEl) countActiveEl.textContent = countActive;
  if (countPausedEl) countPausedEl.textContent = countPaused;

  let filtered = allSchedules;
  if (currentTab === "active") filtered = allSchedules.filter((s) => s.enabled);
  else if (currentTab === "paused") filtered = allSchedules.filter((s) => !s.enabled);

  container.innerHTML = "";
  if (!filtered.length) {
    const msg = currentTab === "all"
      ? "No scheduled tasks configured yet. Create one above to automate recurring chores!"
      : `No ${currentTab} scheduled tasks.`;
    container.innerHTML = `<p class="muted" style="padding:16px 0">${msg}</p>`;
    return;
  }

  filtered.forEach((s) => {
    container.appendChild(renderScheduleCard(s));
  });
}

function renderScheduleCard(s) {
  const card = el("div", {
    class: "card chore",
    style: "margin-bottom:12px;border:1px solid " + (s.enabled ? "#38425f" : "#2a3142") + ";background:" + (s.enabled ? "#192233" : "#121824")
  });

  // Header row
  const headerRow = el("div", {
    class: "row",
    style: "display:flex;justify-content:space-between;align-items:flex-start;gap:10px"
  });

  const titleWrap = el("div", {},
    el("div", { style: "display:flex;align-items:center;gap:8px" },
      el("h3", { style: "margin:0;font-size:1.15rem" }, s.name),
      s.enabled
        ? el("span", { class: "badge", style: "background:#1e3e2b;color:#50fa7b;border:1px solid #28543a;font-size:.75rem;padding:2px 8px;border-radius:12px" }, "🟢 Active")
        : el("span", { class: "badge", style: "background:#33261a;color:#ffb86c;border:1px solid #4a3622;font-size:.75rem;padding:2px 8px;border-radius:12px" }, "⏸️ Paused")
    )
  );

  if (s.description) {
    titleWrap.appendChild(el("p", { class: "muted", style: "margin:4px 0 0;font-size:.88rem" }, s.description));
  }
  headerRow.appendChild(titleWrap);

  // Meta row: Cron & Creator
  const metaGrid = el("div", {
    style: "display:grid;grid-template-columns:repeat(auto-fit, minmax(220px, 1fr));gap:8px;margin:10px 0;font-size:.88rem;background:#131a26;padding:10px 14px;border-radius:8px"
  },
    el("div", {},
      el("span", { class: "muted" }, "⏰ Schedule: "),
      el("code", { style: "background:#222d42;padding:2px 6px;border-radius:4px;font-family:monospace" }, s.cron_expr),
      el("span", { style: "color:#8be9fd;margin-left:6px" }, `(${s.cron_description || "Cron"})`)
    ),
    el("div", {},
      el("span", { class: "muted" }, "👤 Creator: "),
      el("strong", { style: "color:#f1fa8c" }, s.creator_name || s.creator_id || "Admin")
    ),
    el("div", {},
      el("span", { class: "muted" }, "⏭️ Next run: "),
      s.enabled
        ? el("strong", { style: "color:#50fa7b" }, formatTimestamp(s.next_run_at))
        : el("span", { class: "muted" }, "Paused")
    ),
    el("div", {},
      el("span", { class: "muted" }, "⏮️ Last run: "),
      el("span", {}, s.last_run_at ? formatTimestamp(s.last_run_at) : "Never")
    )
  );

  // Specs & Skills chips
  const specsRow = el("div", {
    style: "display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin:8px 0"
  },
    el("span", { class: "badge", style: "background:#212b3d;font-size:.8rem;padding:3px 8px;border-radius:4px" }, `👥 ${s.necessary_workers} worker(s)`),
    el("span", { class: "badge", style: "background:#212b3d;font-size:.8rem;padding:3px 8px;border-radius:4px" }, `⏱️ ${fmtMin(s.estimated_time_min)}`),
    el("span", { class: "badge", style: "background:#212b3d;font-size:.8rem;padding:3px 8px;border-radius:4px" }, `⏳ ${s.assignment_timeout_min}m timeout`)
  );

  if (s.assignee_id) {
    specsRow.appendChild(el("span", { class: "badge", style: "background:#283a54;color:#8be9fd;font-size:.8rem;padding:3px 8px;border-radius:4px" }, `🎯 Direct: ${s.assignee_name || s.assignee_id}`));
  }

  if (s.necessary_capabilities && s.necessary_capabilities.length) {
    s.necessary_capabilities.forEach((c) => {
      specsRow.appendChild(el("span", { class: "badge", style: "background:#332947;color:#bd93f9;font-size:.8rem;padding:3px 8px;border-radius:4px" }, `✨ ${c}`));
    });
  }

  // Action buttons
  const actionsRow = el("div", {
    class: "row",
    style: "display:flex;flex-wrap:wrap;justify-content:flex-end;gap:8px;margin-top:12px;padding-top:10px;border-top:1px solid #28334a"
  },
    el("button", {
      class: "primary small",
      style: "background:linear-gradient(135deg,#3b82f6,#2563eb);border:none",
      onclick: () => triggerNow(s)
    }, "⚡ Run now"),
    el("button", {
      class: "secondary small",
      onclick: () => toggleActive(s)
    }, s.enabled ? "⏸️ Pause" : "▶️ Resume"),
    el("button", {
      class: "secondary small",
      onclick: () => editSchedule(s)
    }, "✏️ Edit"),
    el("button", {
      class: "danger small",
      onclick: () => deleteSchedule(s)
    }, "Delete")
  );

  card.appendChild(headerRow);
  card.appendChild(metaGrid);
  card.appendChild(specsRow);
  card.appendChild(actionsRow);

  return card;
}

function formatTimestamp(isoStr) {
  if (!isoStr) return "Never";
  const d = new Date(isoStr);
  if (isNaN(d.getTime())) return isoStr;

  const now = new Date();
  const isToday = d.toDateString() === now.toDateString();
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  const isTomorrow = d.toDateString() === tomorrow.toDateString();

  const timeStr = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  if (isToday) return `Today at ${timeStr}`;
  if (isTomorrow) return `Tomorrow at ${timeStr}`;

  return `${d.toLocaleDateString([], { month: "short", day: "numeric" })} at ${timeStr}`;
}

function editSchedule(s) {
  document.getElementById("edit-id").value = s.id;
  document.getElementById("form-title").textContent = "Edit scheduled task: " + s.name;
  document.getElementById("name").value = s.name;
  document.getElementById("description").value = s.description || "";
  document.getElementById("cron_expr").value = s.cron_expr;
  document.getElementById("workers").value = s.necessary_workers || 1;
  document.getElementById("time").value = s.estimated_time_min || 10;
  document.getElementById("timeout").value = s.assignment_timeout_min || 15;
  document.getElementById("template_key").value = s.template_key || "";
  document.getElementById("enabled").checked = s.enabled;

  if (s.assignee_id) {
    document.getElementById("assignee").value = s.assignee_id;
  } else {
    document.getElementById("assignee").value = "";
  }

  // Match preset or set custom
  const presetSel = document.getElementById("cron-preset");
  const match = Array.from(presetSel.options).find((opt) => opt.value === s.cron_expr.trim());
  presetSel.value = match ? match.value : "custom";
  updateCronPreview();

  selectedSkills = new Set(s.necessary_capabilities || []);
  document.querySelectorAll("#skills .chip").forEach((chip) => {
    const on = selectedSkills.has(chip.textContent);
    chip.classList.toggle("on", on);
    chip.setAttribute("aria-pressed", on ? "true" : "false");
  });

  document.getElementById("creator_id").value = s.creator_id || "";
  const creatorDisplay = document.getElementById("creator-display");
  if (creatorDisplay) {
    creatorDisplay.textContent = s.creator_name || s.creator_id;
  }
  const creatorSel = document.getElementById("schedule-creator-select");
  if (creatorSel && s.creator_id) {
    creatorSel.value = s.creator_id;
  }

  document.getElementById("btn-submit").textContent = "Update scheduled task";
  document.getElementById("cancel-edit").hidden = false;
  window.scrollTo({ top: 0, behavior: "smooth" });
}

function resetForm() {
  document.getElementById("schedule-form").reset();
  document.getElementById("edit-id").value = "";
  document.getElementById("template_key").value = "";
  document.getElementById("form-title").textContent = "Add a scheduled task";
  document.getElementById("btn-submit").textContent = "Save scheduled task";
  document.getElementById("cancel-edit").hidden = true;
  document.getElementById("cron_expr").value = "0 9 * * *";
  document.getElementById("cron-preset").value = "0 9 * * *";

  selectedSkills.clear();
  document.querySelectorAll("#skills .chip").forEach((chip) => {
    chip.classList.remove("on");
    chip.setAttribute("aria-pressed", "false");
  });

  updateCronPreview();
}

async function saveSchedule(ev) {
  ev.preventDefault();

  const editId = document.getElementById("edit-id").value;
  const creatorId = document.getElementById("creator_id")?.value ||
                    document.getElementById("schedule-creator-select")?.value ||
                    currentUser?.discord_id ||
                    "";

  const body = {
    name: document.getElementById("name").value.trim(),
    description: document.getElementById("description").value.trim(),
    cron_expr: document.getElementById("cron_expr").value.trim(),
    necessary_workers: parseInt(document.getElementById("workers").value, 10) || 1,
    estimated_time_min: parseInt(document.getElementById("time").value, 10) || 10,
    assignment_timeout_min: parseInt(document.getElementById("timeout").value, 10) || 15,
    assignee_id: document.getElementById("assignee").value || "",
    necessary_capabilities: [...selectedSkills],
    template_key: document.getElementById("template_key").value || "",
    enabled: document.getElementById("enabled").checked,
    creator_id: creatorId,
  };

  try {
    if (editId) {
      await API.put("/api/schedules/" + editId, body);
      showToast(`Updated schedule "${body.name}"`);
    } else {
      await API.post("/api/schedules", body);
      showToast(`Created scheduled task "${body.name}"`);
    }
    resetForm();
    await loadSchedules();
  } catch (err) {
    console.error("Save schedule error", err);
    showError(err);
  }
}

async function triggerNow(s) {
  try {
    const res = await API.post(`/api/schedules/${s.id}/run`, {});
    showToast(`⚡ Posted chore "${s.name}" to board! (id: ${res.chore_id})`);
    await loadSchedules();
  } catch (err) {
    console.error("Run schedule error", err);
    showError(err);
  }
}

async function toggleActive(s) {
  try {
    const res = await API.post(`/api/schedules/${s.id}/toggle`, {});
    showToast(res.enabled ? `Activated "${s.name}"` : `Paused "${s.name}"`);
    await loadSchedules();
  } catch (err) {
    console.error("Toggle schedule error", err);
    showError(err);
  }
}

async function deleteSchedule(s) {
  if (!confirm(`Are you sure you want to delete scheduled task "${s.name}"?`)) return;
  try {
    await API.del(`/api/schedules/${s.id}`);
    showToast(`Deleted "${s.name}"`);
    await loadSchedules();
  } catch (err) {
    console.error("Delete schedule error", err);
    showError(err);
  }
}

document.addEventListener("DOMContentLoaded", init);
