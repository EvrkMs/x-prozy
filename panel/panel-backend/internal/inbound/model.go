package inbound

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Protocol — поддерживаемые протоколы Xray.
type Protocol string

const (
	ProtoVLESS       Protocol = "vless"
	ProtoVMess       Protocol = "vmess"
	ProtoTrojan      Protocol = "trojan"
	ProtoShadowsocks Protocol = "shadowsocks"
)

// Network — транспорт Xray.
type Network string

const (
	NetTCP        Network = "tcp"
	NetWebSocket  Network = "ws"
	NetGRPC       Network = "grpc"
	NetHTTPUpgrade Network = "httpupgrade"
)

// Security — TLS-режим.
type Security string

const (
	SecNone    Security = "none"
	SecTLS     Security = "tls"
	SecReality Security = "reality"
)

// Inbound — подключение (inbound) Xray, хранимое в БД.
type Inbound struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Remark    string         `gorm:"size:255;not null" json:"remark"`
	Enable    bool           `gorm:"default:true" json:"enable"`
	Protocol  Protocol       `gorm:"size:32;not null" json:"protocol"`
	Listen    string         `gorm:"size:64;default:''" json:"listen"`
	Port      int            `gorm:"not null" json:"port"`
	Settings  string         `gorm:"type:text;not null" json:"settings"`       // JSON: clients, decryption, etc.
	Stream    string         `gorm:"type:text;default:'{}'" json:"stream"`     // JSON: streamSettings
	Sniffing  string         `gorm:"type:text;default:'{}'" json:"sniffing"`   // JSON: sniffing
	Allocate  string         `gorm:"type:text;default:'{}'" json:"allocate"`   // JSON: allocate
	Tag       string         `gorm:"size:128;uniqueIndex" json:"tag"`
	NodeIDs   string         `gorm:"type:text;default:'[]'" json:"node_ids"`   // JSON: массив node UUID куда деплоить
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// GetNodeIDs парсит JSON-массив node ID.
func (i *Inbound) GetNodeIDs() []string {
	var ids []string
	_ = json.Unmarshal([]byte(i.NodeIDs), &ids)
	return ids
}

// SetNodeIDs сериализует массив node ID в JSON.
func (i *Inbound) SetNodeIDs(ids []string) {
	b, _ := json.Marshal(ids)
	i.NodeIDs = string(b)
}

// ToXrayInbound генерирует объект inbound для Xray JSON-конфига.
func (i *Inbound) ToXrayInbound() map[string]any {
	listen := i.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}

	obj := map[string]any{
		"listen":   listen,
		"port":     i.Port,
		"protocol": string(i.Protocol),
		"tag":      i.Tag,
	}

	// Settings — JSON
	var settings any
	if err := json.Unmarshal([]byte(i.Settings), &settings); err == nil {
		obj["settings"] = settings
	}

	// StreamSettings
	var stream any
	if i.Stream != "" && i.Stream != "{}" {
		if err := json.Unmarshal([]byte(i.Stream), &stream); err == nil {
			obj["streamSettings"] = stream
		}
	}

	// Sniffing
	var sniffing any
	if i.Sniffing != "" && i.Sniffing != "{}" {
		if err := json.Unmarshal([]byte(i.Sniffing), &sniffing); err == nil {
			obj["sniffing"] = sniffing
		}
	}

	return obj
}

// Validate проверяет базовые поля inbound.
func (i *Inbound) Validate() error {
	if i.Remark == "" {
		return fmt.Errorf("remark is required")
	}
	if i.Port < 1 || i.Port > 65535 {
		return fmt.Errorf("port must be 1-65535")
	}
	switch i.Protocol {
	case ProtoVLESS, ProtoVMess, ProtoTrojan, ProtoShadowsocks:
	default:
		return fmt.Errorf("unsupported protocol: %s", i.Protocol)
	}
	if i.Tag == "" {
		i.Tag = fmt.Sprintf("inbound-%s-%d", i.Protocol, i.Port)
	}
	if i.Settings == "" {
		return fmt.Errorf("settings is required")
	}
	return nil
}
