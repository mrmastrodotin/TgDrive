package system

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// ContainerMemoryMetrics encapsulates Linux container and process-level memory consumption,
// distinguishing true Resident Set Size (RSS) and cgroup usage from Go runtime virtual allocations (Sys).
type ContainerMemoryMetrics struct {
	ProcessRSSBytes           uint64 `json:"process_rss_bytes"`
	PeakProcessRSSBytes       uint64 `json:"peak_process_rss_bytes"`
	ContainerMemoryBytes      uint64 `json:"container_memory_bytes"`
	PeakContainerMemoryBytes  uint64 `json:"peak_container_memory_bytes"`
	ContainerMemoryLimitBytes uint64 `json:"container_memory_limit_bytes"`
	CgroupVersion             string `json:"cgroup_version"` // "v1", "v2", or "none"
	CgroupAvailable           bool   `json:"cgroup_available"`
	RSSAvailable              bool   `json:"rss_available"`

	// Go Runtime metrics for correlation
	GoAllocBytes     uint64 `json:"go_alloc_bytes"`
	GoHeapAllocBytes uint64 `json:"go_heap_alloc_bytes"`
	GoHeapInUseBytes uint64 `json:"go_heap_inuse_bytes"`
	GoSysBytes       uint64 `json:"go_sys_bytes"`
	NumGC            uint32 `json:"num_gc"`
}

var (
	observedPeakRSS       atomic.Uint64
	observedPeakContainer atomic.Uint64
)

// ReadContainerMemoryMetrics samples current Linux container metrics (cgroup v1/v2),
// process resident memory (VmRSS / VmHWM), and Go runtime memory statistics.
func ReadContainerMemoryMetrics() ContainerMemoryMetrics {
	var metrics ContainerMemoryMetrics

	// 1. Sample Go runtime memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.GoAllocBytes = m.Alloc
	metrics.GoHeapAllocBytes = m.HeapAlloc
	metrics.GoHeapInUseBytes = m.HeapInuse
	metrics.GoSysBytes = m.Sys
	metrics.NumGC = m.NumGC

	// 2. Read Linux /proc/self/status for Process RSS and High Water Mark (Peak RSS)
	rss, peakRSS, rssFound := readProcStatusRSS()
	if rssFound {
		metrics.ProcessRSSBytes = rss
		metrics.PeakProcessRSSBytes = peakRSS
		metrics.RSSAvailable = true

		// Update tracked atomic peak
		for {
			cur := observedPeakRSS.Load()
			if metrics.ProcessRSSBytes <= cur {
				metrics.PeakProcessRSSBytes = max(metrics.PeakProcessRSSBytes, cur)
				break
			}
			if observedPeakRSS.CompareAndSwap(cur, metrics.ProcessRSSBytes) {
				metrics.PeakProcessRSSBytes = max(metrics.PeakProcessRSSBytes, metrics.ProcessRSSBytes)
				break
			}
		}
	}

	// 3. Read Cgroup memory (v2 first, then v1 fallback)
	cgroupVer, current, peak, limit, cgroupFound := readCgroupMemory()
	if cgroupFound {
		metrics.CgroupVersion = cgroupVer
		metrics.ContainerMemoryBytes = current
		metrics.PeakContainerMemoryBytes = peak
		metrics.ContainerMemoryLimitBytes = limit
		metrics.CgroupAvailable = true

		for {
			cur := observedPeakContainer.Load()
			if metrics.ContainerMemoryBytes <= cur {
				metrics.PeakContainerMemoryBytes = max(metrics.PeakContainerMemoryBytes, cur)
				break
			}
			if observedPeakContainer.CompareAndSwap(cur, metrics.ContainerMemoryBytes) {
				metrics.PeakContainerMemoryBytes = max(metrics.PeakContainerMemoryBytes, metrics.ContainerMemoryBytes)
				break
			}
		}
	} else {
		metrics.CgroupVersion = "none"
	}

	return metrics
}

// readProcStatusRSS parses /proc/self/status for VmRSS and VmHWM in bytes.
func readProcStatusRSS() (rss uint64, peakRSS uint64, found bool) {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					rss = kb * 1024
					found = true
				}
			}
		} else if strings.HasPrefix(line, "VmHWM:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					peakRSS = kb * 1024
				}
			}
		}
	}

	if peakRSS == 0 && rss > 0 {
		peakRSS = rss
	}
	return rss, peakRSS, found
}

// readCgroupMemory checks cgroup v2 then cgroup v1 filesystem endpoints.
func readCgroupMemory() (version string, current uint64, peak uint64, limit uint64, found bool) {
	// Try cgroup v2: /sys/fs/cgroup/memory.current
	if curBytes, err := readUintFromFile("/sys/fs/cgroup/memory.current"); err == nil {
		version = "v2"
		current = curBytes
		found = true

		if pBytes, err := readUintFromFile("/sys/fs/cgroup/memory.peak"); err == nil {
			peak = pBytes
		}
		if maxBytes, err := readUintFromFile("/sys/fs/cgroup/memory.max"); err == nil {
			limit = maxBytes
		}
		return
	}

	// Try cgroup v1: /sys/fs/cgroup/memory/memory.usage_in_bytes
	if curBytes, err := readUintFromFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); err == nil {
		version = "v1"
		current = curBytes
		found = true

		if pBytes, err := readUintFromFile("/sys/fs/cgroup/memory/memory.max_usage_in_bytes"); err == nil {
			peak = pBytes
		}
		if limBytes, err := readUintFromFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
			limit = limBytes
		}
		return
	}

	return "none", 0, 0, 0, false
}

func readUintFromFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	str := strings.TrimSpace(string(data))
	if str == "max" {
		return 0, nil
	}
	return strconv.ParseUint(str, 10, 64)
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
