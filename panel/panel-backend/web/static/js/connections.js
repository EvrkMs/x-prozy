/**
 * connections.js — Управление подключениями (inbounds) Xray.
 */
document.addEventListener("DOMContentLoaded", () => {

  const BASE = window.BASE_PATH || "";
  const API  = BASE + "/api/inbounds";

  // ── DOM refs ──────────────────────────────────────────────
  const listEl     = document.getElementById("ib-list");
  const emptyEl    = document.getElementById("ib-list-empty");
  const modalEl    = document.getElementById("ib-modal");
  const modalTitle = document.getElementById("ib-modal-title");
  const form       = document.getElementById("ib-form");
  const addBtn     = document.getElementById("ib-add-btn");
  const pushBtn    = document.getElementById("ib-push-btn");
  const closeBtn   = document.getElementById("ib-modal-close");
  const cancelBtn  = document.getElementById("ib-cancel");
  const badge      = document.getElementById("nav-inbounds-count");

  // form fields
  const fId        = document.getElementById("ib-id");
  const fRemark    = document.getElementById("ib-remark");
  const fEnable    = document.getElementById("ib-enable");
  const fProtocol  = document.getElementById("ib-protocol");
  const fPort      = document.getElementById("ib-port");
  const fListen    = document.getElementById("ib-listen");
  const fNetwork   = document.getElementById("ib-network");
  const fSecurity  = document.getElementById("ib-security");
  const fWsPath    = document.getElementById("ib-ws-path");
  const fGrpcSvc   = document.getElementById("ib-grpc-service");
  const fSni       = document.getElementById("ib-sni");
  const fRealDest  = document.getElementById("ib-reality-dest");
  const fRealSni   = document.getElementById("ib-reality-sni");
  const fRealPriv  = document.getElementById("ib-reality-private");
  const fRealPub   = document.getElementById("ib-reality-public");
  const fRealShort = document.getElementById("ib-reality-shortids");
  const fRealFP    = document.getElementById("ib-reality-fingerprint");
  const fRealSpidX = document.getElementById("ib-reality-spiderx");
  const fSniffing  = document.getElementById("ib-sniffing");
  const nodesCtn   = document.getElementById("ib-node-targets");
  const genKeysBtn = document.getElementById("ib-gen-keys");
  const genIdsBtn  = document.getElementById("ib-gen-shortids");

  const wsRow      = document.getElementById("ib-ws-row");
  const grpcRow    = document.getElementById("ib-grpc-row");
  const tlsFields  = document.getElementById("ib-tls-fields");
  const realFields = document.getElementById("ib-reality-fields");

  let inbounds = [];
  let nodes    = [];    // для node target selector
  let editMode = false;

  // ── Load data ─────────────────────────────────────────────
  async function loadInbounds() {
    try {
      const res = await fetch(API);
      const json = await res.json();
      inbounds = json.data || [];
      renderList();
      updateBadge();
    } catch (e) {
      console.error("load inbounds:", e);
    }
  }

  async function loadNodes() {
    try {
      const res = await fetch(BASE + "/api/nodes");
      const json = await res.json();
      nodes = json.data || [];
    } catch (e) {
      console.error("load nodes:", e);
    }
  }

  function updateBadge() {
    if (!badge) return;
    const n = inbounds.length;
    badge.textContent = n;
    badge.style.display = n > 0 ? "" : "none";
  }

  // ── Render list ───────────────────────────────────────────
  function renderList() {
    // remove old cards
    listEl.querySelectorAll(".ib-card").forEach(c => c.remove());

    if (inbounds.length === 0) {
      emptyEl.style.display = "";
      return;
    }
    emptyEl.style.display = "none";

    for (const ib of inbounds) {
      const card = document.createElement("div");
      card.className = "ib-card" + (ib.enable ? "" : " ib-card--disabled");
      card.innerHTML = `
        <div class="ib-card__head">
          <div class="ib-card__status ${ib.enable ? 'ib-card__status--on' : 'ib-card__status--off'}"></div>
          <div class="ib-card__name">${esc(ib.remark)}</div>
          <span class="ib-card__proto">${ib.protocol.toUpperCase()}</span>
          <span class="ib-card__port">:${ib.port}</span>
        </div>
        <div class="ib-card__meta">
          <span>${getTransport(ib)} / ${getSecurity(ib)}</span>
          <span>${nodeLabel(ib)}</span>
        </div>
        <div class="ib-card__actions">
          <button class="btn btn--outline btn--xs" data-toggle="${ib.id}">${ib.enable ? 'Откл' : 'Вкл'}</button>
          <button class="btn btn--outline btn--xs" data-edit="${ib.id}">Изменить</button>
          <button class="btn btn--danger btn--xs" data-delete="${ib.id}">Удалить</button>
        </div>
      `;
      listEl.appendChild(card);
    }

    // bind actions
    listEl.querySelectorAll("[data-toggle]").forEach(btn => {
      btn.addEventListener("click", () => toggleInbound(Number(btn.dataset.toggle)));
    });
    listEl.querySelectorAll("[data-edit]").forEach(btn => {
      btn.addEventListener("click", () => openEdit(Number(btn.dataset.edit)));
    });
    listEl.querySelectorAll("[data-delete]").forEach(btn => {
      btn.addEventListener("click", () => deleteInbound(Number(btn.dataset.delete)));
    });
  }

  function esc(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function getTransport(ib) {
    try {
      const s = JSON.parse(ib.stream || "{}");
      return (s.network || "tcp").toUpperCase();
    } catch { return "TCP"; }
  }

  function getSecurity(ib) {
    try {
      const s = JSON.parse(ib.stream || "{}");
      return (s.security || "none").toUpperCase();
    } catch { return "NONE"; }
  }


  function nodeLabel(ib) {
    try {
      const ids = JSON.parse(ib.node_ids || "[]");
      return ids.length === 0 ? "Все ноды" : `${ids.length} нод`;
    } catch { return "Все ноды"; }
  }

  // ── CRUD ──────────────────────────────────────────────────
  async function toggleInbound(id) {
    try {
      await fetch(`${API}/${id}/toggle`, { method: "POST" });
      await loadInbounds();
      toast("Подключение обновлено", "ok");
    } catch (e) {
      toast("Ошибка: " + e.message, "err");
    }
  }

  async function deleteInbound(id) {
    if (!confirm("Удалить подключение?")) return;
    try {
      await fetch(`${API}/${id}`, { method: "DELETE" });
      await loadInbounds();
      toast("Подключение удалено", "ok");
    } catch (e) {
      toast("Ошибка: " + e.message, "err");
    }
  }

  // ── Modal: open / close ───────────────────────────────────
  function openModal() { modalEl.style.display = "flex"; }
  function closeModal() { modalEl.style.display = "none"; editMode = false; }

  if (addBtn) addBtn.addEventListener("click", () => openNew());
  if (closeBtn) closeBtn.addEventListener("click", closeModal);
  if (cancelBtn) cancelBtn.addEventListener("click", closeModal);
  if (modalEl) modalEl.addEventListener("click", e => {
    if (e.target === modalEl) closeModal();
  });

  function resetForm() {
    fId.value = "";
    fRemark.value = "";
    fEnable.checked = true;
    fProtocol.value = "vless";
    fPort.value = "";
    fListen.value = "";
    fNetwork.value = "tcp";
    fSecurity.value = "none";
    fWsPath.value = "/";
    fGrpcSvc.value = "";
    fSni.value = "";
    fRealDest.value = "";
    fRealSni.value = "";
    fRealPriv.value = "";
    fRealPub.value = "";
    fRealShort.value = "";
    fRealFP.value = "chrome";
    fRealSpidX.value = "/";
    fSniffing.checked = true;
    updateTransportFields();
    updateSecurityFields();
  }

  function openNew() {
    editMode = false;
    resetForm();
    modalTitle.textContent = "Новое подключение";
    renderNodeTargets([]);
    openModal();
  }

  function openEdit(id) {
    const ib = inbounds.find(i => i.id === id);
    if (!ib) return;

    editMode = true;
    resetForm();
    modalTitle.textContent = "Редактирование";

    fId.value = ib.id;
    fRemark.value = ib.remark;
    fEnable.checked = ib.enable;
    fProtocol.value = ib.protocol;
    fPort.value = ib.port;
    fListen.value = ib.listen || "";

    // Parse stream
    try {
      const st = JSON.parse(ib.stream || "{}");
      fNetwork.value = st.network || "tcp";
      fSecurity.value = st.security || "none";
      if (st.wsSettings) fWsPath.value = st.wsSettings.path || "/";
      if (st.grpcSettings) fGrpcSvc.value = st.grpcSettings.serviceName || "";
      if (st.tlsSettings) fSni.value = st.tlsSettings.serverName || "";
      if (st.realitySettings) {
        fRealDest.value  = st.realitySettings.dest || "";
        fRealSni.value   = (st.realitySettings.serverNames || []).join(",");
        fRealPriv.value  = st.realitySettings.privateKey || "";
        fRealPub.value   = st.realitySettings.publicKey || "";
        fRealShort.value = (st.realitySettings.shortIds || []).join(",");
        if (st.realitySettings.settings) {
          fRealFP.value    = st.realitySettings.settings.fingerprint || "chrome";
          fRealSpidX.value = st.realitySettings.settings.spiderX || "/";
        } else {
          fRealFP.value    = st.realitySettings.fingerprint || "chrome";
          fRealSpidX.value = st.realitySettings.spiderX || "/";
        }
      }
    } catch {}

    // Sniffing
    try {
      const sn = JSON.parse(ib.sniffing || "{}");
      fSniffing.checked = sn.enabled !== false;
    } catch {}

    // Node targets
    try {
      const ids = JSON.parse(ib.node_ids || "[]");
      renderNodeTargets(ids);
    } catch {
      renderNodeTargets([]);
    }

    updateTransportFields();
    updateSecurityFields();
    openModal();
  }

  // ── Transport / Security conditional fields ───────────────
  function updateTransportFields() {
    const net = fNetwork.value;
    wsRow.style.display   = net === "ws" ? "" : "none";
    grpcRow.style.display = net === "grpc" ? "" : "none";
  }

  function updateSecurityFields() {
    const sec = fSecurity.value;
    tlsFields.style.display  = sec === "tls" ? "" : "none";
    realFields.style.display = sec === "reality" ? "" : "none";
  }

  fNetwork.addEventListener("change", updateTransportFields);
  fSecurity.addEventListener("change", updateSecurityFields);

  // ── Protocol change: set sensible defaults ────────────────
  fProtocol.addEventListener("change", () => {
    const proto = fProtocol.value;
    if (proto === "vless") {
      fSecurity.value = "reality";
      fNetwork.value = "tcp";
    } else if (proto === "vmess") {
      fSecurity.value = "none";
      fNetwork.value = "ws";
    } else if (proto === "trojan") {
      fSecurity.value = "tls";
      fNetwork.value = "tcp";
    } else if (proto === "shadowsocks") {
      fSecurity.value = "none";
      fNetwork.value = "tcp";
    }
    updateTransportFields();
    updateSecurityFields();
  });

  // ── Key & ID generators ───────────────────────────────────
  if (genKeysBtn) {
    genKeysBtn.addEventListener("click", async () => {
      genKeysBtn.disabled = true;
      try {
        const res = await fetch(BASE + "/api/utils/x25519");
        if (!res.ok) { toast("Ошибка генерации ключей: " + res.status, "err"); genKeysBtn.disabled = false; return; }
        const json = await res.json();
        const d = json.data || json;
        fRealPriv.value = d.private_key || "";
        fRealPub.value  = d.public_key || "";
        if (fRealPriv.value && fRealPub.value) {
          toast("Ключи сгенерированы", "ok");
        } else {
          toast("Пустой ответ сервера", "err");
          console.error("x25519 response:", json);
        }
      } catch (e) {
        toast("Ошибка генерации: " + e.message, "err");
      }
      genKeysBtn.disabled = false;
    });
  }

  if (genIdsBtn) {
    genIdsBtn.addEventListener("click", () => {
      fRealShort.value = generateShortIds();
      toast("Short IDs сгенерированы", "ok");
    });
  }

  function generateShortIds() {
    const lengths = [2, 4, 6, 8, 10, 12, 14, 16];
    // shuffle
    for (let i = lengths.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [lengths[i], lengths[j]] = [lengths[j], lengths[i]];
    }
    return lengths.map(len => {
      const arr = new Uint8Array(Math.ceil(len / 2));
      crypto.getRandomValues(arr);
      return Array.from(arr, b => b.toString(16).padStart(2, "0")).join("").slice(0, len);
    }).join(",");
  }

  // ── Node targets ──────────────────────────────────────────
  function renderNodeTargets(selectedIds) {
    nodesCtn.innerHTML = "";
    if (nodes.length === 0) {
      nodesCtn.innerHTML = '<span class="ib-section__hint">Нет доступных нод</span>';
      return;
    }
    for (const n of nodes) {
      const nd = n.node || n;
      const label = document.createElement("label");
      label.className = "ib-node-check";
      const checked = selectedIds.includes(nd.id) ? "checked" : "";
      label.innerHTML = `
        <input type="checkbox" value="${nd.id}" ${checked}>
        <span>${esc(nd.hostname || nd.id)}</span>
        <span class="ib-node-check__status ${nd.status === 'online' ? 'ib-node-check__status--on' : ''}">${nd.status}</span>
      `;
      nodesCtn.appendChild(label);
    }
  }

  // ── Build payload ─────────────────────────────────────────
  function buildPayload() {
    const proto = fProtocol.value;

    // Settings
    const settings = {};
    if (proto === "vless") {
      settings.decryption = "none";
    }
    if (proto === "shadowsocks") {
      settings.method = "2022-blake3-aes-128-gcm";
    }

    // Stream
    const stream = {
      network:  fNetwork.value,
      security: fSecurity.value,
    };

    if (fNetwork.value === "ws") {
      stream.wsSettings = { path: fWsPath.value || "/" };
    }
    if (fNetwork.value === "grpc") {
      stream.grpcSettings = { serviceName: fGrpcSvc.value || "" };
    }
    if (fNetwork.value === "httpupgrade") {
      stream.httpupgradeSettings = { path: fWsPath.value || "/" };
    }

    if (fSecurity.value === "tls") {
      stream.tlsSettings = { serverName: fSni.value || "" };
    }
    if (fSecurity.value === "reality") {
      stream.realitySettings = {
        dest:        fRealDest.value || "",
        serverNames: fRealSni.value ? fRealSni.value.split(",").map(s => s.trim()) : [],
        privateKey:  fRealPriv.value || "",
        publicKey:   fRealPub.value || "",
        shortIds:    fRealShort.value ? fRealShort.value.split(",").map(s => s.trim()) : [],
        fingerprint: fRealFP.value || "chrome",
        spiderX:     fRealSpidX.value || "/",
      };
    }

    // Sniffing
    const sniffing = {
      enabled:      fSniffing.checked,
      destOverride: ["http", "tls", "quic", "fakedns"],
      routeOnly:    false,
    };

    // Node IDs
    const nodeIds = [];
    nodesCtn.querySelectorAll("input[type=checkbox]:checked").forEach(cb => {
      nodeIds.push(cb.value);
    });

    const tag = `inbound-${proto}-${fPort.value || '0'}`;

    return {
      remark:   fRemark.value,
      enable:   fEnable.checked,
      protocol: proto,
      listen:   fListen.value || "",
      port:     Number(fPort.value) || 0,
      settings: JSON.stringify(settings),
      stream:   JSON.stringify(stream),
      sniffing: JSON.stringify(sniffing),
      tag:      tag,
      node_ids: JSON.stringify(nodeIds),
    };
  }

  // ── Submit form ───────────────────────────────────────────
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const payload = buildPayload();

    try {
      const url = editMode ? `${API}/${fId.value}` : API;
      const method = editMode ? "PUT" : "POST";
      const res = await fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const json = await res.json();
      if (!res.ok) {
        toast(json.error || "Ошибка сохранения", "err");
        return;
      }
      toast(editMode ? "Подключение обновлено" : "Подключение создано", "ok");
      closeModal();
      await loadInbounds();
    } catch (err) {
      toast("Ошибка: " + err.message, "err");
    }
  });

  // ── Push button ───────────────────────────────────────────
  if (pushBtn) {
    pushBtn.addEventListener("click", async () => {
      pushBtn.disabled = true;
      try {
        const res = await fetch(`${API}/push`, { method: "POST" });
        const json = await res.json();
        toast(json.data?.message || "Конфиг отправлен", "ok");
      } catch (e) {
        toast("Ошибка: " + e.message, "err");
      }
      pushBtn.disabled = false;
    });
  }

  // ── Helpers ───────────────────────────────────────────────
  function genUUID() {
    return "10000000-1000-4000-8000-100000000000".replace(/[018]/g, c =>
      (+c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> +c / 4).toString(16)
    );
  }

  function randomPassword() {
    const a = new Uint8Array(16);
    crypto.getRandomValues(a);
    return btoa(String.fromCharCode(...a)).replace(/=+$/, "");
  }

  function toast(msg, type) {
    // use global toast if available
    const el = document.getElementById("toast");
    if (!el) { console.log("toast:", msg); return; }
    el.textContent = msg;
    el.className = "toast toast--visible" + (type === "err" ? " toast--err" : " toast--ok");
    clearTimeout(el._t);
    el._t = setTimeout(() => { el.className = "toast"; }, 3000);
  }

  // ── Init: load when connections tab becomes active ────────
  // Observe page activation
  const observer = new MutationObserver(() => {
    const panel = document.querySelector('[data-page-panel="connections"]');
    if (panel && panel.classList.contains("is-active") && inbounds.length === 0) {
      loadNodes().then(() => loadInbounds());
    }
  });
  const content = document.querySelector('.dash-content');
  if (content) observer.observe(content, { subtree: true, attributes: true, attributeFilter: ["class"] });

  // Also load immediately if tab is active
  const panel = document.querySelector('[data-page-panel="connections"]');
  if (panel && panel.classList.contains("is-active")) {
    loadNodes().then(() => loadInbounds());
  }
});
