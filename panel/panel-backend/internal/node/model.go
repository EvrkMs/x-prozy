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
	NodeID string

	CPUPercent float64
	CPUCores   int32
	CPUModel   string

	MemTotal   uint64
	MemUsed    uint64
	MemPercent float64

	SwapTotal   uint64
	SwapUsed    uint64
	SwapPercent float64

	DiskTotal   uint64
	DiskUsed    uint64
	DiskPercent float64

	NetUp   uint64
	NetDown uint64

	Load1  float64
	Load5  float64
	Load15 float64

	TCPCount int32
	UDPCount int32

	Uptime    uint64
	Timestamp int64 // unix millis
}

// NodeInfo — полная информация для API (node + snapshot).
type NodeInfo struct {
	Node     Node          `json:"node"`
	Snapshot *NodeSnapshot `json:"snapshot,omitempty"`
}
