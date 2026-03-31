// Package metrics — встроенное хранилище метрик нод (замена внешнего Prometheus).
package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ── Sample ─────────────────────────────────────────────────────────────────

// Sample — одна точка метрик ноды, сохраняемая в SQLite.
// Хранятся только ключевые значения для графиков; полный snapshot
// доступен из live-данных через WebSocket.
type Sample struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"-"`
	NodeID    string `gorm:"index:idx_sample_node_ts;not null;size:36" json:"node_id"`
	Timestamp int64  `gorm:"index:idx_sample_node_ts;not null" json:"ts"` // unix seconds

	CPUPercent  float64 `json:"cpu"`
	MemPercent  float64 `json:"mem"`
	DiskPercent float64 `json:"disk"`
	NetUp       uint64  `json:"net_up"`
	NetDown     uint64  `json:"net_down"`
	Load1       float64 `json:"load1"`
	TCPCount    int32   `json:"tcp"`
	UDPCount    int32   `json:"udp"`
}

// ── Store ──────────────────────────────────────────────────────────────────

const (
	defaultRetention  = 24 * time.Hour
	minWriteInterval  = 25 * time.Second // не записываем чаще (status_interval ≈ 30 s)
	cleanupInterval   = 1 * time.Hour
	defaultQueryLimit = 1000
)

// Store — встроенное хранилище time-series метрик нод поверх SQLite.
type Store struct {
	db        *gorm.DB
	retention time.Duration
	log       *slog.Logger

	mu       sync.Mutex
	lastWrite map[string]int64 // node_id → last unix ts записи
}

// NewStore создаёт Store, мигрирует таблицу, запускает cleanup-loop.
func NewStore(db *gorm.DB, retentionHours int, log *slog.Logger) (*Store, error) {
	ret := defaultRetention
	if retentionHours > 0 {
		ret = time.Duration(retentionHours) * time.Hour
	}

	if err := db.AutoMigrate(&Sample{}); err != nil {
		return nil, err
	}

	s := &Store{
		db:        db,
		retention: ret,
		log:       log,
		lastWrite: make(map[string]int64),
	}

	return s, nil
}

// StartCleanupLoop запускает фоновую горутину удаления старых семплов.
func (s *Store) StartCleanupLoop(ctx context.Context) {
	// Сразу чистим при старте.
	s.cleanup()

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanup()
			}
		}
	}()
}

// Record сохраняет точку метрик для ноды.
// Автоматически пропускает запись, если прошло менее minWriteInterval.
func (s *Store) Record(nodeID string, snap Snapshot) {
	now := time.Now().Unix()

	s.mu.Lock()
	if last, ok := s.lastWrite[nodeID]; ok && now-last < int64(minWriteInterval.Seconds()) {
		s.mu.Unlock()
		return
	}
	s.lastWrite[nodeID] = now
	s.mu.Unlock()

	sample := Sample{
		NodeID:      nodeID,
		Timestamp:   now,
		CPUPercent:  snap.CPUPercent,
		MemPercent:  snap.MemPercent,
		DiskPercent: snap.DiskPercent,
		NetUp:       snap.NetUp,
		NetDown:     snap.NetDown,
		Load1:       snap.Load1,
		TCPCount:    snap.TCPCount,
		UDPCount:    snap.UDPCount,
	}

	if err := s.db.Create(&sample).Error; err != nil {
		s.log.Warn("metrics store: record failed", "node_id", nodeID, "error", err)
	}
}

// Query возвращает семплы для конкретной ноды за [from, to].
// Для диапазонов > 1h применяется даунсемплинг через временные бакеты с AVG.
func (s *Store) Query(nodeID string, from, to time.Time, limit int) ([]Sample, error) {
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	span := to.Sub(from)

	// Для коротких диапазонов (≤ 1h) возвращаем сырые семплы.
	if span <= time.Hour {
		var samples []Sample
		err := s.db.
			Where("node_id = ? AND timestamp >= ? AND timestamp <= ?", nodeID, from.Unix(), to.Unix()).
			Order("timestamp ASC").
			Limit(limit).
			Find(&samples).Error
		return samples, err
	}

	// Для длинных диапазонов — даунсемплинг через временные бакеты.
	// Целевое кол-во точек: ~300.
	bucketSec := int64(span.Seconds()) / 300
	if bucketSec < 30 {
		bucketSec = 30
	}

	var samples []Sample
	err := s.db.Raw(`
		SELECT
			? AS node_id,
			(timestamp / ? * ?) AS timestamp,
			ROUND(AVG(cpu_percent), 2)  AS cpu_percent,
			ROUND(AVG(mem_percent), 2)  AS mem_percent,
			ROUND(AVG(disk_percent), 2) AS disk_percent,
			CAST(AVG(net_up) AS INTEGER)   AS net_up,
			CAST(AVG(net_down) AS INTEGER) AS net_down,
			ROUND(AVG(load1), 2)   AS load1,
			CAST(AVG(tcp_count) AS INTEGER) AS tcp_count,
			CAST(AVG(udp_count) AS INTEGER) AS udp_count
		FROM samples
		WHERE node_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY (timestamp / ?)
		ORDER BY timestamp ASC
		LIMIT ?
	`, nodeID, bucketSec, bucketSec, nodeID, from.Unix(), to.Unix(), bucketSec, limit).Scan(&samples).Error

	return samples, err
}

// DeleteNode удаляет все семплы ноды (вызывается при удалении ноды).
func (s *Store) DeleteNode(nodeID string) {
	s.mu.Lock()
	delete(s.lastWrite, nodeID)
	s.mu.Unlock()

	s.db.Where("node_id = ?", nodeID).Delete(&Sample{})
}

// SetRetention обновляет время хранения (вызывается из API настроек).
func (s *Store) SetRetention(hours int) {
	if hours < 1 {
		hours = 24
	}
	s.mu.Lock()
	s.retention = time.Duration(hours) * time.Hour
	s.mu.Unlock()
	s.log.Info("metrics store: retention updated", "hours", hours)
}

// Retention возвращает текущее время хранения.
func (s *Store) Retention() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retention
}

// cleanup удаляет семплы старше retention.
func (s *Store) cleanup() {
	ret := s.Retention()
	cutoff := time.Now().Add(-ret).Unix()
	result := s.db.Where("timestamp < ?", cutoff).Delete(&Sample{})
	if result.Error != nil {
		s.log.Warn("metrics store: cleanup failed", "error", result.Error)
	} else if result.RowsAffected > 0 {
		s.log.Info("metrics store: cleaned up old samples", "deleted", result.RowsAffected)
	}
}
