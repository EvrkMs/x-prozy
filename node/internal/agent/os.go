package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// readOSName читает PRETTY_NAME из /etc/os-release.
func readOSName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			name := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(name, `"`)
		}
	}
	return runtime.GOOS
}

// detectPublicIP пытается определить публичный IP ноды через внешние сервисы.
// Тайм-аут 3 секунды. Если не удалось — возвращает "".
func detectPublicIP() string {
	endpoints := []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://icanhazip.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, url := range endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "x-prozy-node/0.1")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()

		ip := strings.TrimSpace(string(body))
		if ip != "" && len(ip) <= 45 { // max IPv6 len
			return ip
		}
	}
	return ""
}
