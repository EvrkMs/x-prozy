package status

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "x-prozy/proto/nodecontrol/v1"
)

// --- CPU sampling with EMA --------------------------------------------------

type cpuSample struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

var (
	prevCPU    cpuSample
	prevCPUSet bool
	cpuEMA     float64
	cpuMu      sync.Mutex
)

const emaAlpha = 0.3

func readCPUSample() (cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				return cpuSample{}, fmt.Errorf("unexpected /proc/stat format")
			}
			var s cpuSample
			s.user, _ = strconv.ParseUint(fields[1], 10, 64)
			s.nice, _ = strconv.ParseUint(fields[2], 10, 64)
			s.system, _ = strconv.ParseUint(fields[3], 10, 64)
			s.idle, _ = strconv.ParseUint(fields[4], 10, 64)
			if len(fields) > 5 {
				s.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
			}
			if len(fields) > 6 {
				s.irq, _ = strconv.ParseUint(fields[6], 10, 64)
			}
			if len(fields) > 7 {
				s.softirq, _ = strconv.ParseUint(fields[7], 10, 64)
			}
			if len(fields) > 8 {
				s.steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
			return s, nil
		}
	}
	return cpuSample{}, fmt.Errorf("cpu line not found in /proc/stat")
}

func (s cpuSample) total() uint64 {
	return s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq + s.steal
}

func (s cpuSample) busy() uint64 {
	return s.user + s.nice + s.system + s.irq + s.softirq + s.steal
}

func sampleCPU() float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()

	cur, err := readCPUSample()
	if err != nil {
		return cpuEMA
	}

	if !prevCPUSet {
		prevCPU = cur
		prevCPUSet = true
		return 0
	}

	dTotal := float64(cur.total() - prevCPU.total())
	dBusy := float64(cur.busy() - prevCPU.busy())
	prevCPU = cur

	if dTotal == 0 {
		return cpuEMA
	}

	raw := (dBusy / dTotal) * 100
	cpuEMA = emaAlpha*raw + (1-emaAlpha)*cpuEMA

	return math.Round(cpuEMA*100) / 100
}

// --- Memory -----------------------------------------------------------------

func readMemInfo() (total, used uint64, pct float64, swapTotal, swapUsed uint64, swapPct float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var memTotal, memFree, buffers, cached, sReclaimable uint64
	var swTotal, swFree uint64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(parts[1], 10, 64)
		val *= 1024 // kB -> bytes

		switch parts[0] {
		case "MemTotal:":
			memTotal = val
		case "MemFree:":
			memFree = val
		case "Buffers:":
			buffers = val
		case "Cached:":
			cached = val
		case "SReclaimable:":
			sReclaimable = val
		case "SwapTotal:":
			swTotal = val
		case "SwapFree:":
			swFree = val
		}
	}

	avail := memFree + buffers + cached + sReclaimable
	if avail > memTotal {
		avail = memTotal
	}
	used = memTotal - avail
	total = memTotal
	if memTotal > 0 {
		pct = math.Round(float64(used)/float64(memTotal)*10000) / 100
	}

	swapTotal = swTotal
	swapUsed = swTotal - swFree
	if swTotal > 0 {
		swapPct = math.Round(float64(swapUsed)/float64(swTotal)*10000) / 100
	}
	return
}

// --- Disk -------------------------------------------------------------------

func readDisk() (total, used uint64, pct float64) {
	var stat syscallStatfs
	if err := statfs("/", &stat); err != nil {
		return
	}

	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free
	if total > 0 {
		pct = math.Round(float64(used)/float64(total)*10000) / 100
	}
	return
}

// --- Network I/O ------------------------------------------------------------

func readNetIO() (up, down uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue
		}
		line := scanner.Text()
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colonIdx])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(line[colonIdx+1:])
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		down += rx
		up += tx
	}
	return
}

// --- Load Average -----------------------------------------------------------

func readLoadAvg() (l1, l5, l15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
		l5, _ = strconv.ParseFloat(fields[1], 64)
		l15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return
}

// --- Uptime -----------------------------------------------------------------

func readUptime() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return uint64(secs)
}

// --- TCP/UDP counts ---------------------------------------------------------

func countConnections(paths []string) int32 {
	var count int32
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if lineNum == 1 {
				continue
			}
			count++
		}
		f.Close()
	}
	return count
}

// --- CPU model name ---------------------------------------------------------

func readCPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return runtime.GOARCH
}

// --- Collect → proto StatusReport -------------------------------------------

// Collect собирает системные метрики и возвращает proto StatusReport.
func Collect() *pb.StatusReport {
	cpuPct := sampleCPU()
	memTotal, memUsed, memPct, swapTotal, swapUsed, swapPct := readMemInfo()
	diskTotal, diskUsed, diskPct := readDisk()
	netUp, netDown := readNetIO()
	l1, l5, l15 := readLoadAvg()

	return &pb.StatusReport{
		CpuPercent:  cpuPct,
		CpuCores:    int32(runtime.NumCPU()),
		CpuModel:    readCPUModel(),
		MemTotal:    memTotal,
		MemUsed:     memUsed,
		MemPercent:  memPct,
		SwapTotal:   swapTotal,
		SwapUsed:    swapUsed,
		SwapPercent: swapPct,
		DiskTotal:   diskTotal,
		DiskUsed:    diskUsed,
		DiskPercent: diskPct,
		NetUp:       netUp,
		NetDown:     netDown,
		Load1:       l1,
		Load5:       l5,
		Load15:      l15,
		TcpCount:    countConnections([]string{"/proc/net/tcp", "/proc/net/tcp6"}),
		UdpCount:    countConnections([]string{"/proc/net/udp", "/proc/net/udp6"}),
		Uptime:      readUptime(),
		Timestamp:   time.Now().UnixMilli(),
	}
}

// StartCollector запускает фоновый сборщик и возвращает функцию для получения
// последнего report (для передачи в Agent).
func StartCollector(interval time.Duration) func() *pb.StatusReport {
	var (
		mu     sync.RWMutex
		latest *pb.StatusReport
	)

	// Инициализируем CPU baseline.
	sampleCPU()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		time.Sleep(500 * time.Millisecond)
		s := Collect()
		mu.Lock()
		latest = s
		mu.Unlock()

		for range ticker.C {
			s := Collect()
			mu.Lock()
			latest = s
			mu.Unlock()
		}
	}()

	return func() *pb.StatusReport {
		mu.RLock()
		defer mu.RUnlock()
		if latest == nil {
			return Collect()
		}
		return latest
	}
}
