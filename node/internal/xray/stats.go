package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SysStats — system-level stats from Xray runtime.
type SysStats struct {
	NumGoroutine uint32
	Alloc        uint64 // heap alloc bytes
	Uptime       uint32 // seconds
}

// TrafficStats — aggregated traffic from all inbounds/outbounds.
type TrafficStats struct {
	TotalUp   uint64 // uplink bytes
	TotalDown uint64 // downlink bytes
}

// Stats — combined Xray metrics snapshot.
type Stats struct {
	Running bool
	Sys     *SysStats
	Traffic *TrafficStats
}

// CollectStats gathers metrics from the running Xray instance.
// Returns a zero Stats with Running=false if Xray is not running.
func (m *Manager) CollectStats() *Stats {
	if !m.IsRunning() {
		return &Stats{Running: false}
	}

	s := &Stats{Running: true}

	sys, err := m.querySysStats()
	if err != nil {
		m.log.Debug("xray sys stats query failed", "error", err)
	} else {
		s.Sys = sys
	}

	traffic, err := m.queryTrafficStats()
	if err != nil {
		m.log.Debug("xray traffic stats query failed", "error", err)
	} else {
		s.Traffic = traffic
	}

	return s
}

// querySysStats calls `xray api sys` and parses JSON output.
func (m *Manager) querySysStats() (*SysStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.xrayBin, "api", "sys", "-s", m.apiAddr)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}

	var raw struct {
		NumGoroutine flexInt `json:"numGoroutine"`
		Alloc        flexInt `json:"alloc"`
		Uptime       flexInt `json:"uptime"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w (output: %s)", err, truncate(out, 200))
	}

	return &SysStats{
		NumGoroutine: uint32(raw.NumGoroutine),
		Alloc:        uint64(raw.Alloc),
		Uptime:       uint32(raw.Uptime),
	}, nil
}

// queryTrafficStats calls `xray api statsquery` and aggregates traffic.
func (m *Manager) queryTrafficStats() (*TrafficStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.xrayBin, "api", "statsquery", "-s", m.apiAddr)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}

	var raw struct {
		Stat []struct {
			Name  string  `json:"name"`
			Value flexInt `json:"value"`
		} `json:"stat"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	t := &TrafficStats{}
	for _, s := range raw.Stat {
		name := s.Name
		val := uint64(s.Value)

		// Skip API inbound traffic — it's our own stats queries.
		if strings.Contains(name, ">>>api>>>") {
			continue
		}

		if strings.Contains(name, ">>>traffic>>>uplink") {
			t.TotalUp += val
		}
		if strings.Contains(name, ">>>traffic>>>downlink") {
			t.TotalDown += val
		}
	}

	return t, nil
}

// flexInt handles both JSON number (42) and JSON string ("42")
// because protojson marshals uint64/int64 as strings.
type flexInt int64

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := string(data)
	// Strip quotes if present.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(v)
	return nil
}

func truncate(b []byte, maxLen int) string {
	s := string(b)
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}
