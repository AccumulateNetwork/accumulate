// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"runtime"
	"testing"
	"time"
)

func TestSystemMetrics_New(t *testing.T) {
	sm := NewSystemMetrics()

	if sm == nil {
		t.Fatal("NewSystemMetrics() returned nil")
	}

	// Should have initialized lastCollectTime
	if sm.lastCollectTime.IsZero() {
		t.Error("lastCollectTime should be initialized")
	}
}

func TestSystemMetrics_Update(t *testing.T) {
	sm := NewSystemMetrics()

	err := sm.Update()
	if err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}

	snap := sm.GetSnapshot()

	// Basic sanity checks
	if snap.CPUPercent < 0 || snap.CPUPercent > 100 {
		t.Errorf("invalid CPU percent: %.2f", snap.CPUPercent)
	}

	if snap.MemoryTotalMB <= 0 {
		t.Errorf("invalid total memory: %.2f MB", snap.MemoryTotalMB)
	}

	if snap.MemoryUsedMB < 0 {
		t.Errorf("invalid used memory: %.2f MB", snap.MemoryUsedMB)
	}

	if snap.MemoryUsedMB > snap.MemoryTotalMB {
		t.Errorf("used memory (%.2f) should not exceed total (%.2f)",
			snap.MemoryUsedMB, snap.MemoryTotalMB)
	}

	// Disk and network metrics should be non-negative
	if snap.DiskReadMB < 0 {
		t.Errorf("invalid disk read: %.2f MB/s", snap.DiskReadMB)
	}
	if snap.DiskWriteMB < 0 {
		t.Errorf("invalid disk write: %.2f MB/s", snap.DiskWriteMB)
	}
	if snap.NetworkRxMB < 0 {
		t.Errorf("invalid network rx: %.2f MB/s", snap.NetworkRxMB)
	}
	if snap.NetworkTxMB < 0 {
		t.Errorf("invalid network tx: %.2f MB/s", snap.NetworkTxMB)
	}
}

func TestSystemMetrics_GetSnapshot(t *testing.T) {
	sm := NewSystemMetrics()
	sm.Update()

	snap := sm.GetSnapshot()

	// Verify it's a copy (changing snap shouldn't affect sm)
	snap.CPUPercent = 999.0
	snap.MemoryUsedMB = 123456.0

	snap2 := sm.GetSnapshot()
	if snap2.CPUPercent == 999.0 || snap2.MemoryUsedMB == 123456.0 {
		t.Error("GetSnapshot should return a copy, not a reference")
	}
}

func TestSystemMetrics_ConcurrentAccess(t *testing.T) {
	sm := NewSystemMetrics()

	done := make(chan struct{})

	// Concurrent updates
	go func() {
		for i := 0; i < 20; i++ {
			sm.Update()
			time.Sleep(5 * time.Millisecond)
		}
		close(done)
	}()

	// Concurrent reads
	for i := 0; i < 50; i++ {
		_ = sm.GetSnapshot()
		time.Sleep(2 * time.Millisecond)
	}

	<-done
}

func TestSystemMetrics_MultipleUpdates(t *testing.T) {
	sm := NewSystemMetrics()

	// First update
	err := sm.Update()
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	_ = sm.GetSnapshot()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Second update
	err = sm.Update()
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	snap2 := sm.GetSnapshot()

	// Metrics should be valid after second update
	if snap2.MemoryTotalMB <= 0 {
		t.Error("memory total should remain positive")
	}
}

func TestSystemMetrics_MemoryCollection(t *testing.T) {
	sm := NewSystemMetrics()

	err := sm.collectMemory()
	if err != nil {
		// On Linux, this should work. On other systems, it might use fallback
		if runtime.GOOS == "linux" {
			t.Fatalf("collectMemory failed on Linux: %v", err)
		}
	}

	if sm.MemoryTotalMB <= 0 {
		t.Error("memory total should be positive after collection")
	}

	if sm.MemoryUsedMB < 0 {
		t.Error("memory used should be non-negative")
	}
}

func TestSystemMetrics_CPUCollection(t *testing.T) {
	sm := NewSystemMetrics()

	// First collection (baseline)
	err := sm.collectCPU(1.0)
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Fatalf("collectCPU failed on Linux: %v", err)
		}
		// Non-Linux systems may not have /proc/stat, which is fine
		return
	}

	// Wait and collect again
	time.Sleep(100 * time.Millisecond)

	err = sm.collectCPU(0.1)
	if err != nil {
		t.Fatalf("second collectCPU failed: %v", err)
	}

	// CPU should be in valid range
	if sm.CPUPercent < 0 || sm.CPUPercent > 100 {
		t.Errorf("invalid CPU percent: %.2f", sm.CPUPercent)
	}
}

func TestSystemMetrics_DiskCollection(t *testing.T) {
	sm := NewSystemMetrics()

	// First collection (baseline)
	err := sm.collectDisk(1.0)
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Fatalf("collectDisk failed on Linux: %v", err)
		}
		// Non-Linux is okay
		return
	}

	// Metrics should be non-negative
	if sm.DiskReadMB < 0 || sm.DiskWriteMB < 0 {
		t.Errorf("disk metrics should be non-negative: read=%.2f, write=%.2f",
			sm.DiskReadMB, sm.DiskWriteMB)
	}

	// Second collection
	time.Sleep(50 * time.Millisecond)
	err = sm.collectDisk(0.05)
	if err != nil {
		t.Fatalf("second collectDisk failed: %v", err)
	}

	// Still should be non-negative
	if sm.DiskReadMB < 0 || sm.DiskWriteMB < 0 {
		t.Errorf("disk metrics should remain non-negative: read=%.2f, write=%.2f",
			sm.DiskReadMB, sm.DiskWriteMB)
	}
}

func TestSystemMetrics_NetworkCollection(t *testing.T) {
	sm := NewSystemMetrics()

	// First collection (baseline)
	err := sm.collectNetwork(1.0)
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Fatalf("collectNetwork failed on Linux: %v", err)
		}
		// Non-Linux is okay
		return
	}

	// Metrics should be non-negative
	if sm.NetworkRxMB < 0 || sm.NetworkTxMB < 0 {
		t.Errorf("network metrics should be non-negative: rx=%.2f, tx=%.2f",
			sm.NetworkRxMB, sm.NetworkTxMB)
	}

	// Second collection
	time.Sleep(50 * time.Millisecond)
	err = sm.collectNetwork(0.05)
	if err != nil {
		t.Fatalf("second collectNetwork failed: %v", err)
	}

	// Still should be non-negative
	if sm.NetworkRxMB < 0 || sm.NetworkTxMB < 0 {
		t.Errorf("network metrics should remain non-negative: rx=%.2f, tx=%.2f",
			sm.NetworkRxMB, sm.NetworkTxMB)
	}
}

func TestSystemMetrics_ZeroElapsedTime(t *testing.T) {
	sm := NewSystemMetrics()

	// Set last collect time to now
	sm.lastCollectTime = time.Now()

	// Immediate collect should handle zero elapsed gracefully
	err := sm.collect()
	if err != nil {
		t.Fatalf("collect with zero elapsed failed: %v", err)
	}
}

func TestParseUint64(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"0", 0},
		{"123", 123},
		{"999999999", 999999999},
		{"invalid", 0}, // Should return 0 on error
		{"", 0},
		{"-1", 0}, // Negative should return 0
	}

	for _, tt := range tests {
		result := parseUint64(tt.input)
		if result != tt.expected {
			t.Errorf("parseUint64(%q) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}

func TestSystemMetrics_NonLinuxFallback(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test is for non-Linux systems")
	}

	sm := NewSystemMetrics()
	err := sm.Update()
	if err != nil {
		t.Fatalf("Update should work on non-Linux: %v", err)
	}

	snap := sm.GetSnapshot()

	// Memory should still be populated via runtime.MemStats
	if snap.MemoryTotalMB <= 0 {
		t.Error("memory total should be positive on non-Linux")
	}

	// Disk and network should be 0 (no /proc on non-Linux)
	if snap.DiskReadMB != 0 || snap.DiskWriteMB != 0 {
		t.Error("disk metrics should be 0 on non-Linux")
	}
	if snap.NetworkRxMB != 0 || snap.NetworkTxMB != 0 {
		t.Error("network metrics should be 0 on non-Linux")
	}

	// CPU should be 0 (no /proc/stat on non-Linux)
	if snap.CPUPercent != 0 {
		t.Error("CPU should be 0 on non-Linux")
	}
}

func TestSystemMetrics_RapidUpdates(t *testing.T) {
	sm := NewSystemMetrics()

	// Rapid updates should not cause issues
	for i := 0; i < 50; i++ {
		err := sm.Update()
		if err != nil && runtime.GOOS == "linux" {
			t.Fatalf("update %d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	snap := sm.GetSnapshot()
	if snap.MemoryTotalMB <= 0 {
		t.Error("memory should still be valid after rapid updates")
	}
}

func TestSystemMetrics_Baseline(t *testing.T) {
	sm := NewSystemMetrics()

	// After creation, baseline should be established
	if sm.lastCollectTime.IsZero() {
		t.Error("baseline should be established on creation")
	}

	// First snapshot should have valid data
	snap := sm.GetSnapshot()
	if snap.MemoryTotalMB <= 0 {
		t.Error("initial snapshot should have valid memory data")
	}
}
