// Package xray manages the Xray process lifecycle on the node.
package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Manager controls the Xray binary: start, stop, restart, config write.
type Manager struct {
	xrayBin    string // path to xray binary
	configDir  string // directory for config files
	apiAddr    string // xray stats API address (host:port)
	log        *slog.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	running bool
}

// NewManager creates a new Xray process manager.
// If the binary does not exist, methods are no-ops.
func NewManager(xrayBin, configDir, apiAddr string, log *slog.Logger) *Manager {
	return &Manager{
		xrayBin:   xrayBin,
		configDir: configDir,
		apiAddr:   apiAddr,
		log:       log.With("component", "xray"),
	}
}

// Available returns true if the xray binary exists and is executable.
func (m *Manager) Available() bool {
	info, err := os.Stat(m.xrayBin)
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// Start launches the Xray process. Writes default config if needed.
func (m *Manager) Start(parentCtx context.Context) error {
	if !m.Available() {
		m.log.Warn("xray binary not found, skipping start", "path", m.xrayBin)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return fmt.Errorf("xray: create config dir: %w", err)
	}

	cfgPath := m.configPath()

	// Write default config if not exists.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := m.writeDefaultConfig(cfgPath); err != nil {
			return fmt.Errorf("xray: write default config: %w", err)
		}
		m.log.Info("wrote default xray config", "path", cfgPath)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	m.cancel = cancel

	cmd := exec.CommandContext(ctx, m.xrayBin, "run", "-config", cfgPath)
	cmd.Stdout = newLogWriter(m.log, slog.LevelDebug, "xray")
	cmd.Stderr = newLogWriter(m.log, slog.LevelWarn, "xray")

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("xray: start: %w", err)
	}

	m.cmd = cmd
	m.running = true
	m.log.Info("xray started", "pid", cmd.Process.Pid)

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		m.running = false
		m.cmd = nil
		m.mu.Unlock()

		if err != nil && ctx.Err() == nil {
			m.log.Warn("xray process exited unexpectedly", "error", err)
			// Auto-restart after a short delay.
			select {
			case <-parentCtx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			if err := m.Start(parentCtx); err != nil {
				m.log.Error("xray auto-restart failed", "error", err)
			}
		}
	}()

	return nil
}

// Stop kills the Xray process.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.running = false
	m.cmd = nil
	m.log.Info("xray stopped")
}

// Restart stops and starts xray. Used after config changes.
func (m *Manager) Restart(ctx context.Context) error {
	m.Stop()
	time.Sleep(500 * time.Millisecond)
	return m.Start(ctx)
}

// IsRunning returns whether the xray process is alive.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// ApplyConfig writes config JSON and restarts xray.
// The stats API inbound is always injected to ensure metrics collection works.
func (m *Manager) ApplyConfig(ctx context.Context, configJSON string) error {
	if !m.Available() {
		return nil
	}

	// Parse provided config.
	var cfg map[string]any
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("xray: invalid config json: %w", err)
	}

	// Ensure stats + API are always enabled.
	m.injectStatsAPI(cfg)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	cfgPath := m.configPath()
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("xray: write config: %w", err)
	}

	m.log.Info("xray config updated, restarting")
	return m.Restart(ctx)
}

// XrayBin returns the path to the xray binary (used by stats collector).
func (m *Manager) XrayBin() string {
	return m.xrayBin
}

// APIAddr returns the xray stats API address.
func (m *Manager) APIAddr() string {
	return m.apiAddr
}

// configPath returns the full path to the xray config file.
func (m *Manager) configPath() string {
	return filepath.Join(m.configDir, "config.json")
}

// writeDefaultConfig creates a minimal xray config with stats API enabled.
func (m *Manager) writeDefaultConfig(path string) error {
	cfg := m.defaultConfig()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// defaultConfig returns a minimal xray config with stats API.
func (m *Manager) defaultConfig() map[string]any {
	// Parse host:port for the stats API inbound.
	host, port := m.parseAPIAddr()

	return map[string]any{
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"StatsService"},
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"inbounds": []any{
			map[string]any{
				"tag":      "api",
				"listen":   host,
				"port":     port,
				"protocol": "dokodemo-door",
				"settings": map[string]any{
					"address": host,
				},
			},
		},
		"routing": map[string]any{
			"rules": []any{
				map[string]any{
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
					"type":        "field",
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]any{},
			},
		},
	}
}

// injectStatsAPI ensures the provided config has stats API enabled.
func (m *Manager) injectStatsAPI(cfg map[string]any) {
	host, port := m.parseAPIAddr()

	// Ensure "api" section.
	cfg["api"] = map[string]any{
		"tag":      "api",
		"services": []string{"StatsService"},
	}

	// Ensure "stats" section.
	cfg["stats"] = map[string]any{}

	// Ensure system stats in policy.
	policy, _ := cfg["policy"].(map[string]any)
	if policy == nil {
		policy = map[string]any{}
	}
	policy["system"] = map[string]any{
		"statsInboundUplink":    true,
		"statsInboundDownlink":  true,
		"statsOutboundUplink":   true,
		"statsOutboundDownlink": true,
	}
	cfg["policy"] = policy

	// Ensure API inbound exists.
	apiInbound := map[string]any{
		"tag":      "api",
		"listen":   host,
		"port":     port,
		"protocol": "dokodemo-door",
		"settings": map[string]any{
			"address": host,
		},
	}

	inbounds, _ := cfg["inbounds"].([]any)
	// Remove existing api inbound if present.
	filtered := make([]any, 0, len(inbounds)+1)
	for _, ib := range inbounds {
		if m, ok := ib.(map[string]any); ok {
			if m["tag"] == "api" {
				continue
			}
		}
		filtered = append(filtered, ib)
	}
	filtered = append([]any{apiInbound}, filtered...)
	cfg["inbounds"] = filtered

	// Ensure routing rule for api inbound.
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
	}
	rules, _ := routing["rules"].([]any)
	apiRule := map[string]any{
		"inboundTag":  []string{"api"},
		"outboundTag": "api",
		"type":        "field",
	}
	// Remove existing api rule.
	filteredRules := make([]any, 0, len(rules)+1)
	for _, r := range rules {
		if rm, ok := r.(map[string]any); ok {
			if tag, _ := rm["outboundTag"].(string); tag == "api" {
				continue
			}
		}
		filteredRules = append(filteredRules, r)
	}
	filteredRules = append([]any{apiRule}, filteredRules...)
	routing["rules"] = filteredRules
	cfg["routing"] = routing
}

// parseAPIAddr splits apiAddr into host and port number.
func (m *Manager) parseAPIAddr() (string, int) {
	host := "127.0.0.1"
	port := 10085

	if m.apiAddr != "" {
		for i := len(m.apiAddr) - 1; i >= 0; i-- {
			if m.apiAddr[i] == ':' {
				host = m.apiAddr[:i]
				fmt.Sscanf(m.apiAddr[i+1:], "%d", &port)
				break
			}
		}
	}
	return host, port
}

// logWriter adapts slog to io.Writer for xray process stdout/stderr.
type logWriter struct {
	log   *slog.Logger
	level slog.Level
	tag   string
}

func newLogWriter(log *slog.Logger, level slog.Level, tag string) *logWriter {
	return &logWriter{log: log, level: level, tag: tag}
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.log.Log(context.Background(), w.level, string(p), "source", w.tag)
	return len(p), nil
}
