package system

import (
	"runtime"
	"testing"
)

func TestReadContainerMemoryMetrics(t *testing.T) {
	metrics := ReadContainerMemoryMetrics()

	// Verify Go runtime statistics are populated regardless of platform
	if metrics.GoAllocBytes == 0 {
		t.Fatalf("expected GoAllocBytes > 0, got %d", metrics.GoAllocBytes)
	}
	if metrics.GoSysBytes == 0 {
		t.Fatalf("expected GoSysBytes > 0, got %d", metrics.GoSysBytes)
	}
	if metrics.GoHeapAllocBytes == 0 {
		t.Fatalf("expected GoHeapAllocBytes > 0, got %d", metrics.GoHeapAllocBytes)
	}

	t.Logf("Go Alloc: %d bytes, Go Sys: %d bytes, Go Heap: %d bytes", metrics.GoAllocBytes, metrics.GoSysBytes, metrics.GoHeapAllocBytes)
	t.Logf("Platform: %s, RSS Available: %v (RSS: %d bytes, Peak: %d bytes), Cgroup: %s (Usage: %d bytes)",
		runtime.GOOS, metrics.RSSAvailable, metrics.ProcessRSSBytes, metrics.PeakProcessRSSBytes, metrics.CgroupVersion, metrics.ContainerMemoryBytes)

	if runtime.GOOS == "linux" {
		// On Linux environments (Docker / production), RSS or cgroups should normally be accessible
		t.Logf("Linux detected: RSS available = %v, Cgroup = %s", metrics.RSSAvailable, metrics.CgroupVersion)
	}
}
