package node

import (
	"time"

	"gorm.io/gorm"
)

// NodeStatus — текущий статус ноды.
type NodeStatus string

const (
	StatusOnline       NodeStatus = "online"
	StatusOffline      NodeStatus = "offline"
	StatusConnecting   NodeStatus = "connecting"
	StatusUnregistered NodeStatus = "unregistered" // handshake не пройден
)

// Node — запись ноды в БД.
type Node struct {
	ID              string         `gorm:"primaryKey;size:36" json:"id"` // UUID
	Hostname        string         `gorm:"size:255" json:"hostname"`
	Status          NodeStatus     `gorm:"size:32;default:offline" json:"status"`
	ReconnectSecret string         `gorm:"size:128;not null" json:"-"`
	Version         string         `gorm:"size:64" json:"version"` // node-agent version
	OS              string         `gorm:"size:64" json:"os"`
	Arch            string         `gorm:"size:32" json:"arch"`
	PublicIP        string         `gorm:"size:64" json:"public_ip"`    // публичный IP ноды (сообщается нодой)
	RemoteAddr      string         `gorm:"size:128" json:"remote_addr"` // IP:port транспорта (grpc peer)
	ConnectedAt     *time.Time     `json:"connected_at"`
	LastSeenAt      *time.Time     `json:"last_seen_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// NodeSnapshot — текущие метрики ноды (не хранится в БД, только в памяти).
type NodeSnapshot struct {
	NodeID string `json:"node_id"`

	CPUPercent float64 `json:"cpu_percent"`
	CPUCores   int32   `json:"cpu_cores"`
	CPUModel   string  `json:"cpu_model"`

	MemTotal   uint64  `json:"mem_total"`
	MemUsed    uint64  `json:"mem_used"`
	MemPercent float64 `json:"mem_percent"`

	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapPercent float64 `json:"swap_percent"`

	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskPercent float64 `json:"disk_percent"`

	NetUp   uint64 `json:"net_up"`
	NetDown uint64 `json:"net_down"`

	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	TCPCount int32 `json:"tcp_count"`
	UDPCount int32 `json:"udp_count"`

	Uptime    uint64 `json:"uptime"`
	Timestamp int64  `json:"timestamp"`

	// Xray runtime metrics
	XrayRunning     bool   `json:"xray_running"`
	XrayUptime      uint32 `json:"xray_uptime"`
	XrayGoroutines  uint32 `json:"xray_goroutines"`
	XrayMemAlloc    uint64 `json:"xray_mem_alloc"`
	XrayTrafficUp   uint64 `json:"xray_traffic_up"`
	XrayTrafficDown uint64 `json:"xray_traffic_down"`
}

// NodeInfo — полная информация для API (node + snapshot).
type NodeInfo struct {
	Node     Node          `json:"node"`
	Snapshot *NodeSnapshot `json:"snapshot,omitempty"`
}
