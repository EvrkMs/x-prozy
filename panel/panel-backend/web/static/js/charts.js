/**
 * Prozy Charts — lightweight canvas line-chart renderer.
 * No external dependencies. Designed for the node metrics panel.
 */

const ProzyCharts = (() => {
  "use strict";

  // ── Color palette ──────────────────────────────────────────
  const COLORS = {
    cpu:       { line: "#22c55e", fill: "rgba(34,197,94,0.10)" },
    mem:       { line: "#3b82f6", fill: "rgba(59,130,246,0.10)" },
    disk:      { line: "#f59e0b", fill: "rgba(245,158,11,0.10)" },
    load:      { line: "#a855f7", fill: "rgba(168,85,247,0.10)" },
    tcp:       { line: "#06b6d4", fill: "rgba(6,182,212,0.10)" },
    udp:       { line: "#f43f5e", fill: "rgba(244,63,94,0.10)" },
    net_up:    { line: "#10b981", fill: "rgba(16,185,129,0.10)" },
    net_down:  { line: "#6366f1", fill: "rgba(99,102,241,0.10)" },
    xray_mem:  { line: "#ec4899", fill: "rgba(236,72,153,0.10)" },
    xray_up:   { line: "#14b8a6", fill: "rgba(20,184,166,0.10)" },
    xray_down: { line: "#8b5cf6", fill: "rgba(139,92,246,0.10)" },
  };

  // ── Helpers ────────────────────────────────────────────────

  function getThemeColors() {
    const isLight = document.documentElement.classList.contains("light");
    return {
      grid:   isLight ? "rgba(0,0,0,0.06)" : "rgba(255,255,255,0.06)",
      label:  isLight ? "rgba(0,0,0,0.40)" : "rgba(255,255,255,0.35)",
      zero:   isLight ? "rgba(0,0,0,0.10)" : "rgba(255,255,255,0.10)",
    };
  }

  function fmtTime(ts) {
    const d = new Date(ts * 1000);
    const h = String(d.getHours()).padStart(2, "0");
    const m = String(d.getMinutes()).padStart(2, "0");
    return `${h}:${m}`;
  }

  function fmtBytes(b) {
    if (b < 1024) return b + " B";
    if (b < 1048576) return (b / 1024).toFixed(1) + " K";
    if (b < 1073741824) return (b / 1048576).toFixed(1) + " M";
    return (b / 1073741824).toFixed(2) + " G";
  }

  function fmtPercent(v) { return v.toFixed(1) + "%"; }
  function fmtNumber(v)  { return String(Math.round(v)); }
  function fmtLoad(v)    { return v.toFixed(2); }

  // Choose a nice formatter based on metric kind
  function pickFormatter(key) {
    if (key === "cpu" || key === "mem" || key === "disk") return fmtPercent;
    if (key === "net_up" || key === "net_down" || key === "xray_mem" || key === "xray_up" || key === "xray_down") return fmtBytes;
    if (key === "load") return fmtLoad;
    return fmtNumber;
  }

  // ── LTTB downsampling (Largest Triangle Three Buckets) ────
  // Preserves visual shape while reducing point count.
  function lttb(data, threshold, key) {
    if (data.length <= threshold) return data;

    const out = [data[0]];
    const bucketSize = (data.length - 2) / (threshold - 2);

    let a = 0;  // index of previous selected point

    for (let i = 1; i < threshold - 1; i++) {
      const rangeStart = Math.floor((i - 1) * bucketSize) + 1;
      const rangeEnd   = Math.min(Math.floor(i * bucketSize) + 1, data.length);
      const nextStart  = rangeEnd;
      const nextEnd    = Math.min(Math.floor((i + 1) * bucketSize) + 1, data.length);

      // Average of next bucket
      let avgTS = 0, avgVal = 0;
      for (let j = nextStart; j < nextEnd; j++) {
        avgTS  += data[j].ts;
        avgVal += (data[j][key] || 0);
      }
      const cnt = nextEnd - nextStart;
      avgTS  /= cnt;
      avgVal /= cnt;

      // Pick point in current bucket with largest triangle area
      let maxArea = -1, maxIdx = rangeStart;
      const aTS = data[a].ts, aVal = data[a][key] || 0;

      for (let j = rangeStart; j < rangeEnd; j++) {
        const area = Math.abs(
          (aTS - avgTS) * ((data[j][key] || 0) - aVal) -
          (aTS - data[j].ts) * (avgVal - aVal)
        );
        if (area > maxArea) {
          maxArea = area;
          maxIdx = j;
        }
      }
      out.push(data[maxIdx]);
      a = maxIdx;
    }
    out.push(data[data.length - 1]);
    return out;
  }

  // ── Drawing ────────────────────────────────────────────────

  const PAD = { top: 18, right: 12, bottom: 24, left: 42 };

  function drawChart(canvas, samples, key, opts = {}) {
    const ctx = canvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;

    // Responsive sizing
    const rect = canvas.parentElement.getBoundingClientRect();
    const w = rect.width || 320;
    const h = opts.height || 120;
    canvas.style.width  = w + "px";
    canvas.style.height = h + "px";
    canvas.width  = w * dpr;
    canvas.height = h * dpr;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, h);

    if (!samples || samples.length < 2) {
      // Generate a flat zero line spanning the visible range
      const now = Math.floor(Date.now() / 1000);
      samples = [
        { ts: now - 3600, [key]: 0 },
        { ts: now,        [key]: 0 },
      ];
    }

    const theme = getThemeColors();
    const color = COLORS[key] || COLORS.cpu;
    const fmt   = pickFormatter(key);

    const values = samples.map(s => s[key] || 0);
    const times  = samples.map(s => s.ts);

    // Client-side LTTB downsampling: cap at canvas pixel width
    const maxPts = Math.min(Math.max(Math.floor((rect.width || 320) * 0.8), 150), 500);
    if (samples.length > maxPts) {
      samples = lttb(samples, maxPts, key);
      values.length = 0;
      times.length = 0;
      for (const s of samples) {
        values.push(s[key] || 0);
        times.push(s.ts);
      }
    }

    let yMin = opts.yMin !== undefined ? opts.yMin : Math.min(...values);
    let yMax = opts.yMax !== undefined ? opts.yMax : Math.max(...values);
    if (yMin === yMax) { yMin = 0; yMax = yMax || 1; }
    const yRange = yMax - yMin;

    const plotW = w - PAD.left - PAD.right;
    const plotH = h - PAD.top  - PAD.bottom;

    // ── Grid lines (4 horizontal) ──
    ctx.strokeStyle = theme.grid;
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 3]);
    for (let i = 0; i <= 4; i++) {
      const y = PAD.top + (plotH / 4) * i;
      ctx.beginPath();
      ctx.moveTo(PAD.left, y);
      ctx.lineTo(w - PAD.right, y);
      ctx.stroke();
    }
    ctx.setLineDash([]);

    // ── Y-axis labels ──
    ctx.fillStyle = theme.label;
    ctx.font = "10px 'JetBrains Mono', monospace";
    ctx.textAlign = "right";
    ctx.textBaseline = "middle";
    for (let i = 0; i <= 4; i++) {
      const y = PAD.top + (plotH / 4) * i;
      const val = yMax - (yRange / 4) * i;
      ctx.fillText(fmt(val), PAD.left - 6, y);
    }

    // ── X-axis time labels ──
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const labelCount = Math.min(6, samples.length);
    const step = Math.floor(samples.length / labelCount);
    for (let i = 0; i < samples.length; i += step) {
      const x = PAD.left + (i / (samples.length - 1)) * plotW;
      ctx.fillText(fmtTime(times[i]), x, h - PAD.bottom + 6);
    }

    // ── Plot data ──
    function toX(i) { return PAD.left + (i / (samples.length - 1)) * plotW; }
    function toY(v) { return PAD.top + plotH - ((v - yMin) / yRange) * plotH; }

    // Fill area
    ctx.beginPath();
    ctx.moveTo(toX(0), toY(values[0]));
    for (let i = 1; i < values.length; i++) {
      ctx.lineTo(toX(i), toY(values[i]));
    }
    ctx.lineTo(toX(values.length - 1), PAD.top + plotH);
    ctx.lineTo(toX(0), PAD.top + plotH);
    ctx.closePath();
    ctx.fillStyle = color.fill;
    ctx.fill();

    // Line
    ctx.beginPath();
    ctx.moveTo(toX(0), toY(values[0]));
    for (let i = 1; i < values.length; i++) {
      ctx.lineTo(toX(i), toY(values[i]));
    }
    ctx.strokeStyle = color.line;
    ctx.lineWidth = 1.5;
    ctx.lineJoin = "round";
    ctx.stroke();

    // Last value dot
    const lastI = values.length - 1;
    const lx = toX(lastI), ly = toY(values[lastI]);
    ctx.beginPath();
    ctx.arc(lx, ly, 3, 0, 2 * Math.PI);
    ctx.fillStyle = color.line;
    ctx.fill();

    // Last value label (top-right)
    ctx.fillStyle = color.line;
    ctx.font = "bold 11px 'JetBrains Mono', monospace";
    ctx.textAlign = "right";
    ctx.textBaseline = "top";
    ctx.fillText(fmt(values[lastI]), w - PAD.right, 4);
  }

  // ── Chart config per metric ─────────────────────────────────

  const CHART_DEFS = [
    { key: "cpu",       label: "CPU %",        yMin: 0, yMax: 100 },
    { key: "mem",       label: "RAM %",        yMin: 0, yMax: 100 },
    { key: "disk",      label: "Диск %",       yMin: 0, yMax: 100 },
    { key: "load",      label: "Load 1m",      yMin: 0 },
    { key: "tcp",       label: "TCP соед.",     yMin: 0 },
    { key: "udp",       label: "UDP соед.",     yMin: 0 },
    { key: "net_up",    label: "Сеть ↑ (TX)",  yMin: 0 },
    { key: "net_down",  label: "Сеть ↓ (RX)",  yMin: 0 },
    { key: "xray_mem",  label: "Xray RAM",     yMin: 0 },
    { key: "xray_up",   label: "Xray ↑",       yMin: 0 },
    { key: "xray_down", label: "Xray ↓",       yMin: 0 },
  ];

  // ── Public: render metrics panel for a node ──────────────────

  let activeNodeId = null;
  let currentRange = "1h";
  let chartsData   = null;

  const RANGES = [
    { value: "30m", label: "30м" },
    { value: "1h",  label: "1ч" },
    { value: "6h",  label: "6ч" },
    { value: "12h", label: "12ч" },
    { value: "24h", label: "24ч" },
  ];

  function buildMetricsModal() {
    // Overlay backdrop
    const overlay = document.createElement("div");
    overlay.className = "metrics-modal-overlay";

    const modal = document.createElement("div");
    modal.className = "metrics-modal";
    modal.innerHTML = `
      <div class="metrics-modal__header">
        <div class="metrics-modal__title">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
            <polyline points="2 12 5 6 8 9 11 3 14 7"/>
            <line x1="2" y1="14" x2="14" y2="14"/>
          </svg>
          <span id="metrics-modal-name">Метрики</span>
        </div>
        <div class="metrics-modal__ranges" id="metrics-ranges"></div>
        <button class="metrics-modal__close" id="metrics-close" title="Закрыть">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
            <line x1="4" y1="4" x2="12" y2="12"/><line x1="12" y1="4" x2="4" y2="12"/>
          </svg>
        </button>
      </div>
      <div class="metrics-modal__loading" id="metrics-loading">Загрузка…</div>
      <div class="metrics-modal__grid" id="metrics-grid" style="display:none"></div>
    `;
    overlay.appendChild(modal);

    // Range buttons
    const rangesEl = modal.querySelector("#metrics-ranges");
    for (const r of RANGES) {
      const btn = document.createElement("button");
      btn.className = "metrics-modal__range" + (r.value === currentRange ? " is-active" : "");
      btn.textContent = r.label;
      btn.dataset.range = r.value;
      btn.addEventListener("click", () => {
        currentRange = r.value;
        rangesEl.querySelectorAll(".metrics-modal__range").forEach(b => b.classList.remove("is-active"));
        btn.classList.add("is-active");
        if (activeNodeId) loadAndRender(overlay);
      });
      rangesEl.appendChild(btn);
    }

    function closeModal() {
      activeNodeId = null;
      overlay.classList.remove("is-open");
      setTimeout(() => overlay.remove(), 200);
    }

    // Close on button click
    modal.querySelector("#metrics-close").addEventListener("click", closeModal);

    // Close on backdrop click
    overlay.addEventListener("click", (e) => {
      if (e.target === overlay) closeModal();
    });

    // Close on Escape
    function onKey(e) {
      if (e.key === "Escape") { closeModal(); document.removeEventListener("keydown", onKey); }
    }
    document.addEventListener("keydown", onKey);

    return overlay;
  }

  async function loadAndRender(overlay) {
    const loading = overlay.querySelector("#metrics-loading");
    const grid    = overlay.querySelector("#metrics-grid");

    loading.style.display = "";
    grid.style.display = "none";

    try {
      const base = window.BASE_PATH || "";
      const url  = `${base}/api/metrics/${activeNodeId}/history?range=${currentRange}&limit=500`;
      const res  = await fetch(url);
      if (!res.ok) throw new Error("HTTP " + res.status);
      const json = await res.json();
      chartsData = (json.data || json).samples || [];
    } catch (e) {
      loading.textContent = "Ошибка загрузки метрик";
      return;
    }

    loading.style.display = "none";
    grid.style.display = "";
    grid.innerHTML = "";

    // Build all cells first (DOM layout), then draw charts progressively
    const cells = [];
    for (const def of CHART_DEFS) {
      const cell = document.createElement("div");
      cell.className = "metrics-modal__cell";

      const label = document.createElement("div");
      label.className = "metrics-modal__label";
      label.textContent = def.label;
      cell.appendChild(label);

      const canvas = document.createElement("canvas");
      canvas.className = "metrics-modal__canvas";
      cell.appendChild(canvas);
      grid.appendChild(cell);

      cells.push({ canvas, def });
    }

    // Draw charts progressively with rAF to avoid blocking the main thread
    let idx = 0;
    function drawNext() {
      if (idx >= cells.length) return;
      const { canvas, def } = cells[idx++];
      const sampleKey = def.key === "load" ? "load1" : def.key;
      drawChart(canvas, chartsData, sampleKey, {
        height: 100,
        yMin: def.yMin,
        yMax: def.yMax,
      });
      if (idx < cells.length) requestAnimationFrame(drawNext);
    }
    requestAnimationFrame(drawNext);
  }

  /** Open metrics modal for a node. */
  function openForNode(nodeId, nodeName) {
    // Close existing modal
    const existing = document.querySelector(".metrics-modal-overlay");
    if (existing) existing.remove();

    activeNodeId = nodeId;
    currentRange = "1h";

    const overlay = buildMetricsModal();
    const nameEl = overlay.querySelector("#metrics-modal-name");
    if (nameEl) nameEl.textContent = "Метрики — " + (nodeName || nodeId.substring(0, 8));

    document.body.appendChild(overlay);

    // Animate in
    requestAnimationFrame(() => overlay.classList.add("is-open"));

    loadAndRender(overlay);
  }

  /** Redraw all visible charts (e.g. on theme change or resize). */
  function redraw() {
    const grid = document.querySelector("#metrics-grid");
    if (!grid || !chartsData) return;

    const cells = grid.querySelectorAll(".metrics-modal__cell");
    cells.forEach((cell, i) => {
      if (i >= CHART_DEFS.length) return;
      const def = CHART_DEFS[i];
      const canvas = cell.querySelector("canvas");
      if (!canvas) return;
      const sampleKey = def.key === "load" ? "load1" : def.key;
      drawChart(canvas, chartsData, sampleKey, {
        height: 100,
        yMin: def.yMin,
        yMax: def.yMax,
      });
    });
  }

  // Redraw on resize
  let resizeTimer;
  window.addEventListener("resize", () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(redraw, 200);
  });

  return { openForNode, redraw };
})();
