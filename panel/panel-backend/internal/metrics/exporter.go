// Package metrics предоставляет Prometheus-экспортёр метрик нод.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "prozy"

// Exporter хранит Prometheus gauge-метрики для всех нод.
type Exporter struct {
	// ── Node metrics (с лейблами node_id, hostname) ──────
	CPUPercent *prometheus.GaugeVec
	CPUCores   *prometheus.GaugeVec

	MemTotalBytes *prometheus.GaugeVec
	MemUsedBytes  *prometheus.GaugeVec
	MemPercent    *prometheus.GaugeVec

	SwapTotalBytes *prometheus.GaugeVec
	SwapUsedBytes  *prometheus.GaugeVec

	DiskTotalBytes *prometheus.GaugeVec
	DiskUsedBytes  *prometheus.GaugeVec
	DiskPercent    *prometheus.GaugeVec

	NetUpBytes   *prometheus.GaugeVec
	NetDownBytes *prometheus.GaugeVec

	Load1  *prometheus.GaugeVec
	Load5  *prometheus.GaugeVec
	Load15 *prometheus.GaugeVec

	TCPConnections *prometheus.GaugeVec
	UDPConnections *prometheus.GaugeVec

	UptimeSeconds *prometheus.GaugeVec

	// ── Xray metrics ─────────────────────────────────────────
	XrayRunning     *prometheus.GaugeVec
	XrayUptime      *prometheus.GaugeVec
	XrayGoroutines  *prometheus.GaugeVec
	XrayMemAlloc    *prometheus.GaugeVec
	XrayTrafficUp   *prometheus.GaugeVec
	XrayTrafficDown *prometheus.GaugeVec

	// ── Panel-level metrics ──────────────────────────────
	NodesTotal *prometheus.GaugeVec // label: status (online/offline)
	WSClients  prometheus.Gauge
}

// nodeLabels — общие лейблы для per-node метрик.
var nodeLabels = []string{"node_id", "hostname"}

// NewExporter создаёт и регистрирует все Prometheus метрики.
func NewExporter() *Exporter {
	e := &Exporter{
		CPUPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "cpu_percent",
			Help: "Current CPU usage percent (0-100).",
		}, nodeLabels),
		CPUCores: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "cpu_cores",
			Help: "Number of CPU cores.",
		}, nodeLabels),

		MemTotalBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "mem_total_bytes",
			Help: "Total memory in bytes.",
		}, nodeLabels),
		MemUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "mem_used_bytes",
			Help: "Used memory in bytes.",
		}, nodeLabels),
		MemPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "mem_percent",
			Help: "Memory usage percent (0-100).",
		}, nodeLabels),

		SwapTotalBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "swap_total_bytes",
			Help: "Total swap in bytes.",
		}, nodeLabels),
		SwapUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "swap_used_bytes",
			Help: "Used swap in bytes.",
		}, nodeLabels),

		DiskTotalBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "disk_total_bytes",
			Help: "Total disk space in bytes.",
		}, nodeLabels),
		DiskUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "disk_used_bytes",
			Help: "Used disk space in bytes.",
		}, nodeLabels),
		DiskPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "disk_percent",
			Help: "Disk usage percent (0-100).",
		}, nodeLabels),

		NetUpBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "net_up_bytes",
			Help: "Network bytes sent per interval.",
		}, nodeLabels),
		NetDownBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "net_down_bytes",
			Help: "Network bytes received per interval.",
		}, nodeLabels),

		Load1: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "load1",
			Help: "1-minute load average.",
		}, nodeLabels),
		Load5: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "load5",
			Help: "5-minute load average.",
		}, nodeLabels),
		Load15: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "load15",
			Help: "15-minute load average.",
		}, nodeLabels),

		TCPConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "tcp_connections",
			Help: "Number of active TCP connections.",
		}, nodeLabels),
		UDPConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "udp_connections",
			Help: "Number of active UDP connections.",
		}, nodeLabels),

		UptimeSeconds: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "node", Name: "uptime_seconds",
			Help: "System uptime in seconds.",
		}, nodeLabels),

		// Xray
		XrayRunning: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "running",
			Help: "Whether Xray is running (1=yes, 0=no).",
		}, nodeLabels),
		XrayUptime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "uptime_seconds",
			Help: "Xray uptime in seconds.",
		}, nodeLabels),
		XrayGoroutines: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "goroutines",
			Help: "Number of Xray goroutines.",
		}, nodeLabels),
		XrayMemAlloc: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "mem_alloc_bytes",
			Help: "Xray heap alloc in bytes.",
		}, nodeLabels),
		XrayTrafficUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "traffic_up_bytes",
			Help: "Xray total uplink traffic in bytes.",
		}, nodeLabels),
		XrayTrafficDown: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "xray", Name: "traffic_down_bytes",
			Help: "Xray total downlink traffic in bytes.",
		}, nodeLabels),

		// Panel-level
		NodesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace, Name: "nodes_total",
			Help: "Total number of nodes by status.",
		}, []string{"status"}),
		WSClients: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "ws_clients",
			Help: "Number of active WebSocket clients.",
		}),
	}

	// Регистрируем все метрики в default registry.
	prometheus.MustRegister(
		e.CPUPercent, e.CPUCores,
		e.MemTotalBytes, e.MemUsedBytes, e.MemPercent,
		e.SwapTotalBytes, e.SwapUsedBytes,
		e.DiskTotalBytes, e.DiskUsedBytes, e.DiskPercent,
		e.NetUpBytes, e.NetDownBytes,
		e.Load1, e.Load5, e.Load15,
		e.TCPConnections, e.UDPConnections,
		e.UptimeSeconds,
		e.XrayRunning, e.XrayUptime, e.XrayGoroutines,
		e.XrayMemAlloc, e.XrayTrafficUp, e.XrayTrafficDown,
		e.NodesTotal, e.WSClients,
	)

	return e
}

// SetNodeMetrics обновляет все gauge для указанной ноды.
func (e *Exporter) SetNodeMetrics(nodeID, hostname string, snap Snapshot) {
	l := prometheus.Labels{"node_id": nodeID, "hostname": hostname}

	e.CPUPercent.With(l).Set(snap.CPUPercent)
	e.CPUCores.With(l).Set(float64(snap.CPUCores))

	e.MemTotalBytes.With(l).Set(float64(snap.MemTotal))
	e.MemUsedBytes.With(l).Set(float64(snap.MemUsed))
	e.MemPercent.With(l).Set(snap.MemPercent)

	e.SwapTotalBytes.With(l).Set(float64(snap.SwapTotal))
	e.SwapUsedBytes.With(l).Set(float64(snap.SwapUsed))

	e.DiskTotalBytes.With(l).Set(float64(snap.DiskTotal))
	e.DiskUsedBytes.With(l).Set(float64(snap.DiskUsed))
	e.DiskPercent.With(l).Set(snap.DiskPercent)

	e.NetUpBytes.With(l).Set(float64(snap.NetUp))
	e.NetDownBytes.With(l).Set(float64(snap.NetDown))

	e.Load1.With(l).Set(snap.Load1)
	e.Load5.With(l).Set(snap.Load5)
	e.Load15.With(l).Set(snap.Load15)

	e.TCPConnections.With(l).Set(float64(snap.TCPCount))
	e.UDPConnections.With(l).Set(float64(snap.UDPCount))

	e.UptimeSeconds.With(l).Set(float64(snap.Uptime))

	// Xray
	if snap.XrayRunning {
		e.XrayRunning.With(l).Set(1)
	} else {
		e.XrayRunning.With(l).Set(0)
	}
	e.XrayUptime.With(l).Set(float64(snap.XrayUptime))
	e.XrayGoroutines.With(l).Set(float64(snap.XrayGoroutines))
	e.XrayMemAlloc.With(l).Set(float64(snap.XrayMemAlloc))
	e.XrayTrafficUp.With(l).Set(float64(snap.XrayTrafficUp))
	e.XrayTrafficDown.With(l).Set(float64(snap.XrayTrafficDown))
}

// RemoveNode удаляет все метрики для ноды (при delete/offline).
func (e *Exporter) RemoveNode(nodeID, hostname string) {
	l := prometheus.Labels{"node_id": nodeID, "hostname": hostname}

	e.CPUPercent.Delete(l)
	e.CPUCores.Delete(l)
	e.MemTotalBytes.Delete(l)
	e.MemUsedBytes.Delete(l)
	e.MemPercent.Delete(l)
	e.SwapTotalBytes.Delete(l)
	e.SwapUsedBytes.Delete(l)
	e.DiskTotalBytes.Delete(l)
	e.DiskUsedBytes.Delete(l)
	e.DiskPercent.Delete(l)
	e.NetUpBytes.Delete(l)
	e.NetDownBytes.Delete(l)
	e.Load1.Delete(l)
	e.Load5.Delete(l)
	e.Load15.Delete(l)
	e.TCPConnections.Delete(l)
	e.UDPConnections.Delete(l)
	e.UptimeSeconds.Delete(l)
	e.XrayRunning.Delete(l)
	e.XrayUptime.Delete(l)
	e.XrayGoroutines.Delete(l)
	e.XrayMemAlloc.Delete(l)
	e.XrayTrafficUp.Delete(l)
	e.XrayTrafficDown.Delete(l)
}

// SetNodeCounts обновляет prozy_nodes_total{status=online|offline}.
func (e *Exporter) SetNodeCounts(online, offline int) {
	e.NodesTotal.With(prometheus.Labels{"status": "online"}).Set(float64(online))
	e.NodesTotal.With(prometheus.Labels{"status": "offline"}).Set(float64(offline))
}

// SetWSClients обновляет prozy_ws_clients.
func (e *Exporter) SetWSClients(count int) {
	e.WSClients.Set(float64(count))
}

// Snapshot — минимальный интерфейс метрик ноды (чтобы не импортировать node пакет).
type Snapshot struct {
	CPUPercent float64
	CPUCores   int32
	MemTotal   uint64
	MemUsed    uint64
	MemPercent float64
	SwapTotal  uint64
	SwapUsed   uint64
	DiskTotal  uint64
	DiskUsed   uint64
	DiskPercent float64
	NetUp      uint64
	NetDown    uint64
	Load1      float64
	Load5      float64
	Load15     float64
	TCPCount   int32
	UDPCount   int32
	Uptime     uint64

	// Xray runtime metrics
	XrayRunning     bool
	XrayUptime      uint32
	XrayGoroutines  uint32
	XrayMemAlloc    uint64
	XrayTrafficUp   uint64
	XrayTrafficDown uint64
}
