document.addEventListener("DOMContentLoaded", () => {

  // ── Theme ─────────────────────────────────────────────────
  const THEME_KEY = "prozy-theme";
  if (localStorage.getItem(THEME_KEY) === "light") {
    document.documentElement.classList.add("light");
  }

  const toggleBtn = document.getElementById("theme-toggle");
  if (toggleBtn) {
    toggleBtn.addEventListener("click", () => {
      document.documentElement.classList.toggle("light");
      const isLight = document.documentElement.classList.contains("light");
      localStorage.setItem(THEME_KEY, isLight ? "light" : "dark");
      // Redraw charts on theme change
      if (typeof ProzyCharts !== 'undefined') ProzyCharts.redraw();
    });
  }

  // ── Sidebar nav groups ────────────────────────────────────
  document.querySelectorAll("[data-nav-toggle]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const group = btn.closest("[data-nav-group]");
      if (!group) return;
      group.classList.toggle("is-open");
      btn.classList.toggle("is-open");
    });
  });

  // ── Sidebar page switching ────────────────────────────────
  const pageLinks  = document.querySelectorAll("[data-page]");
  const pagePanels = document.querySelectorAll("[data-page-panel]");
  const pageTitle  = document.getElementById("page-title");
  const pageCrumb  = document.getElementById("page-crumb");

  const pageMeta = {
    overview:     { title: "Обзор системы",   crumb: "дешборд / обзор системы" },
    nodes:        { title: "Список нод",      crumb: "ноды / список нод" },
    account:      { title: "Учётная запись",  crumb: "настройки / учётная запись" },
    panel:        { title: "Конфигурация",    crumb: "настройки / конфигурация" },
    integrations: { title: "Интеграции",      crumb: "настройки / интеграции" },
  };

  let activePage = "overview";

  pageLinks.forEach((link) => {
    link.addEventListener("click", () => {
      if (link.disabled) return;
      const target = link.dataset.page;

      pageLinks.forEach((l)  => l.classList.remove("is-active"));
      pagePanels.forEach((p) => p.classList.remove("is-active"));

      link.classList.add("is-active");
      const panel = document.querySelector(`[data-page-panel="${target}"]`);
      if (panel) panel.classList.add("is-active");

      const meta = pageMeta[target];
      if (meta && pageTitle) pageTitle.textContent = meta.title;
      if (meta && pageCrumb) pageCrumb.textContent = meta.crumb;

      activePage = target;
    });
  });

  // ── API form handling ─────────────────────────────────────
  document.querySelectorAll("[data-api]").forEach((form) => {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();

      const url  = (window.BASE_PATH || "") + form.dataset.api;
      const data = new FormData(form);
      const body = {};
      data.forEach((v, k) => {
        const inp = form.querySelector(`[name="${k}"]`);
        body[k] = inp && inp.type === "number" ? Number(v) : v;
      });

      // Retention form: convert value + unit → hours
      if (form.hasAttribute("data-retention-form")) {
        const val  = Number(body.value) || 1;
        const mult = Number(body.unit)  || 1;
        body.hours = val * mult;
        delete body.value;
        delete body.unit;
      }

      // Client-side password validation
      if (body.new_password || body.confirm_password) {
        if (body.new_password !== body.confirm_password) {
          toast("Пароли не совпадают", "err");
          return;
        }
        if (body.new_password && body.new_password.length < 4) {
          toast("Пароль слишком короткий (мин. 4 символа)", "err");
          return;
        }
      }

      const btn = form.querySelector("button[type=submit]");
      if (btn) {
        btn.disabled = true;
        btn.style.opacity = "0.6";
      }

      try {
        const res  = await fetch(url, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        const json = await res.json();

        if (res.ok) {
          toast(json.message || "Сохранено", "ok");

          if (json.redirect_login) {
            setTimeout(() => {
              window.location.href = (window.BASE_PATH || "") + "/login";
            }, 1200);
            return;
          }

          if (json.redirect !== undefined) {
            setTimeout(() => {
              window.location.href = json.redirect || "/";
            }, 1000);
            return;
          }

          form.querySelectorAll('input[type="password"]').forEach((i) => (i.value = ""));
        } else {
          toast(json.error || "Ошибка", "err");
        }
      } catch {
        toast("Ошибка сети", "err");
      } finally {
        if (btn) {
          btn.disabled = false;
          btn.style.opacity = "";
        }
      }
    });
  });

  // ── Toast ─────────────────────────────────────────────────
  const toastEl = document.getElementById("toast");
  let toastTimer;

  function toast(msg, type = "ok") {
    if (!toastEl) return;
    toastEl.textContent = msg;
    toastEl.className = `toast toast--${type} is-visible`;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => toastEl.classList.remove("is-visible"), 3000);
  }

  // ── Nodes: WS real-time + polling fallback ─────────────
  const POLL_FALLBACK_MS = 10000; // fallback polling (only when WS is down)

  function fmtBytes(b) {
    if (b < 1024) return b + " B";
    if (b < 1048576) return (b / 1024).toFixed(1) + " KB";
    if (b < 1073741824) return (b / 1048576).toFixed(1) + " MB";
    return (b / 1073741824).toFixed(2) + " GB";
  }

  function fmtUptime(sec) {
    if (!sec) return "—";
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (d > 0) return `${d}д ${h}ч`;
    if (h > 0) return `${h}ч ${m}м`;
    return `${m}м`;
  }

  function fmtMem(used, total) {
    if (!total) return "—";
    const u = total >= 1073741824 ? (used / 1073741824).toFixed(1) + " GB" : (used / 1048576).toFixed(0) + " MB";
    const t = total >= 1073741824 ? (total / 1073741824).toFixed(1) + " GB" : (total / 1048576).toFixed(0) + " MB";
    return `${u} / ${t}`;
  }

  function nodeStatusClass(status) {
    if (status === "online") return "node-card__status--online";
    if (status === "offline") return "node-card__status--offline";
    return "node-card__status--unknown";
  }

  function nodeStatusLabel(status) {
    if (status === "online") return "Онлайн";
    if (status === "offline") return "Офлайн";
    return status || "—";
  }

  function renderGaugeMini(pct) {
    const r = 18, c = 2 * Math.PI * r;
    const dash = (pct / 100) * c;
    const color = pct > 85 ? "var(--danger)" : pct > 60 ? "var(--warning, #f59e0b)" : "var(--accent)";
    return `<svg class="node-gauge" viewBox="0 0 44 44">
      <circle cx="22" cy="22" r="${r}" fill="none" stroke="var(--line)" stroke-width="3.5"/>
      <circle cx="22" cy="22" r="${r}" fill="none" stroke="${color}" stroke-width="3.5"
        stroke-dasharray="${dash} ${c - dash}" stroke-linecap="round"
        transform="rotate(-90 22 22)" style="transition:stroke-dasharray .6s ease"/>
      <text x="22" y="22" text-anchor="middle" dominant-baseline="central"
        fill="var(--text)" font-size="11" font-weight="600">${Math.round(pct)}%</text>
    </svg>`;
  }

  function buildNodeCard(info) {
    const n = info.node;
    const s = info.snapshot;
    const card = document.createElement("div");
    card.className = "node-card";
    card.dataset.nodeId = n.id;

    const isOnline = n.status === "online";

    let metricsHTML = "";
    if (s && isOnline) {
      metricsHTML = `
        <div class="node-card__metrics">
          <div class="node-card__metric">
            <div class="node-card__metric-label">CPU</div>
            ${renderGaugeMini(s.cpu_percent || 0)}
          </div>
          <div class="node-card__metric">
            <div class="node-card__metric-label">RAM</div>
            ${renderGaugeMini(s.mem_percent || 0)}
          </div>
          <div class="node-card__metric">
            <div class="node-card__metric-label">Диск</div>
            ${renderGaugeMini(s.disk_percent || 0)}
          </div>
        </div>
        <div class="node-card__details">
          <div class="node-card__detail"><span>CPU</span><span>${(s.cpu_model || "—").split(/\s+/).slice(0, 4).join(" ")} (${s.cpu_cores || "?"}c)</span></div>
          <div class="node-card__detail"><span>RAM</span><span>${fmtMem(s.mem_used, s.mem_total)}</span></div>
          <div class="node-card__detail"><span>Диск</span><span>${fmtMem(s.disk_used, s.disk_total)}</span></div>
          <div class="node-card__detail"><span>Сеть ↑/↓</span><span>${fmtBytes(s.net_up || 0)} / ${fmtBytes(s.net_down || 0)}</span></div>
          <div class="node-card__detail"><span>TCP / UDP</span><span>${s.tcp_count || 0} / ${s.udp_count || 0}</span></div>
          <div class="node-card__detail"><span>Load</span><span>${(s.load1 || 0).toFixed(2)} / ${(s.load5 || 0).toFixed(2)} / ${(s.load15 || 0).toFixed(2)}</span></div>
        </div>`;
    } else if (!isOnline) {
      metricsHTML = `<div class="node-card__offline-hint">Нет данных — нода офлайн</div>`;
    }

    card.innerHTML = `
      <div class="node-card__header">
        <div class="node-card__identity">
          <div class="node-card__status ${nodeStatusClass(n.status)}"></div>
          <div class="node-card__name">${n.hostname || "node"}</div>
          <div class="node-card__badge">${nodeStatusLabel(n.status)}</div>
        </div>
        <div class="node-card__actions">
          <button class="node-card__delete" title="Удалить ноду" data-delete-node="${n.id}">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M5 2h6M2 4h12M6 7v5M10 7v5M3.5 4l.5 9a1.5 1.5 0 001.5 1.5h5A1.5 1.5 0 0012 13l.5-9"
                stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        </div>
      </div>
      <div class="node-card__info">
        <span>${n.os || ""} ${n.arch || ""}</span>
        <span>v${n.version || "?"}</span>
        <span>uptime ${s ? fmtUptime(s.uptime) : "—"}</span>
      </div>
      ${metricsHTML}
      <div class="node-card__footer">
        <span class="node-card__id" title="${n.id}">${n.id.substring(0, 8)}…</span>
        <button class="node-card__metrics-btn" data-metrics-node="${n.id}" title="Метрики">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
            <polyline points="2 12 5 6 8 9 11 3 14 7"/>
            <line x1="2" y1="14" x2="14" y2="14"/>
          </svg>
          <span>Метрики</span>
        </button>
        <span class="node-card__addr">${n.public_ip || n.remote_addr || "—"}</span>
      </div>
    `;

    // Delete handler
    card.querySelector("[data-delete-node]")?.addEventListener("click", async (e) => {
      e.stopPropagation();
      if (!confirm("Удалить ноду " + (n.hostname || n.id.substring(0, 8)) + "?")) return;

      try {
        const url = (window.BASE_PATH || "") + "/api/nodes/" + n.id;
        const res = await fetch(url, { method: "DELETE" });
        const json = await res.json();
        if (res.ok) {
          toast(json.message || "Нода удалена", "ok");
        } else {
          toast(json.error || "Ошибка удаления", "err");
        }
      } catch {
        toast("Ошибка сети", "err");
      }
    });

    // Metrics chart handler — open in modal
    card.querySelector("[data-metrics-node]")?.addEventListener("click", (e) => {
      e.stopPropagation();
      if (typeof ProzyCharts !== 'undefined') {
        ProzyCharts.openForNode(n.id, n.hostname || n.id.substring(0, 8));
      }
    });

    return card;
  }

  // ── Render helpers ──────────────────────────────────────
  function updateNodeUI(nodes) {
    if (!nodes) return;

    const totalEl  = document.getElementById("nodes-total");
    const onlineEl = document.getElementById("nodes-online");
    const offlineEl = document.getElementById("nodes-offline");
    const navCount = document.getElementById("nav-nodes-count");

    const online  = nodes.filter(n => n.node.status === "online").length;
    const offline = nodes.length - online;

    if (totalEl) totalEl.textContent = nodes.length;
    if (onlineEl) onlineEl.textContent = online;
    if (offlineEl) offlineEl.textContent = offline;

    if (navCount) {
      navCount.textContent = online;
      navCount.style.display = nodes.length > 0 ? "" : "none";
    }

    renderNodeList("node-list", "node-list-empty", nodes);
    renderNodeList("node-list-detail", "node-list-detail-empty", nodes);
  }

  function renderNodeList(containerId, emptyId, nodes) {
    const container = document.getElementById(containerId);
    const emptyEl   = document.getElementById(emptyId);
    if (!container) return;

    if (!nodes.length) {
      container.querySelectorAll(".node-card").forEach(c => c.remove());
      if (emptyEl) emptyEl.style.display = "";
      return;
    }

    if (emptyEl) emptyEl.style.display = "none";

    const existing = new Map();
    container.querySelectorAll(".node-card").forEach(c => {
      existing.set(c.dataset.nodeId, c);
    });

    const incomingIds = new Set(nodes.map(n => n.node.id));

    for (const [id, el] of existing) {
      if (!incomingIds.has(id)) el.remove();
    }

    for (const info of nodes) {
      const id = info.node.id;
      const card = buildNodeCard(info);
      if (existing.has(id)) {
        existing.get(id).replaceWith(card);
      } else {
        container.appendChild(card);
      }
    }
  }

  // ── HTTP fetch (fallback + initial load) ────────────────
  async function fetchNodes() {
    try {
      const url = (window.BASE_PATH || "") + "/api/nodes";
      const res = await fetch(url);
      if (!res.ok) return null;
      const json = await res.json();
      return json.data || json;
    } catch { return null; }
  }

  // ── WebSocket with auto-reconnect ───────────────────────
  let ws = null;
  let wsRetryDelay = 1000;
  let fallbackTimer = null;

  function wsUrl() {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const base = (window.BASE_PATH || "");
    return `${proto}//${location.host}${base}/ws`;
  }

  function connectWS() {
    if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) return;

    try {
      ws = new WebSocket(wsUrl());
    } catch {
      scheduleWSReconnect();
      return;
    }

    ws.onopen = () => {
      wsRetryDelay = 1000; // reset backoff
      stopFallbackPolling();
      // Сразу запрашиваем актуальные данные (WS только пушит изменения).
      fetchNodes().then(n => n && updateNodeUI(n));
    };

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data);
        if (msg.type === "nodes") {
          updateNodeUI(msg.data);
        }
      } catch { /* ignore malformed */ }
    };

    ws.onclose = () => {
      ws = null;
      startFallbackPolling();
      scheduleWSReconnect();
    };

    ws.onerror = () => {
      // onclose will fire after this
    };
  }

  function scheduleWSReconnect() {
    setTimeout(connectWS, wsRetryDelay);
    wsRetryDelay = Math.min(wsRetryDelay * 1.5, 30000); // max 30s
  }

  // Fallback polling — active only when WS is disconnected.
  function startFallbackPolling() {
    if (fallbackTimer) return;
    fallbackTimer = setInterval(async () => {
      const nodes = await fetchNodes();
      if (nodes) updateNodeUI(nodes);
    }, POLL_FALLBACK_MS);
  }

  function stopFallbackPolling() {
    if (fallbackTimer) {
      clearInterval(fallbackTimer);
      fallbackTimer = null;
    }
  }

  // ── Boot ────────────────────────────────────────────────
  // Initial HTTP fetch (instant), then connect WS for real-time.
  fetchNodes().then(n => n && updateNodeUI(n));
  connectWS();

});