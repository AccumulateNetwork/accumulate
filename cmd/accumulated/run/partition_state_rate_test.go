// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"
	"time"
)

// TestReservePartitionState_PerPartition asserts the rate limit is
// scoped per-partition: admitting one partition does not block another.
func TestReservePartitionState_PerPartition(t *testing.T) {
	inst := &Instance{}

	if ok, _ := inst.ReservePartitionState("Directory"); !ok {
		t.Fatalf("first DN reservation should be admitted")
	}
	if ok, _ := inst.ReservePartitionState("Cyclops"); !ok {
		t.Fatalf("first Cyclops reservation should be admitted (per-partition limit)")
	}
}

// TestReservePartitionState_RejectsRepeat asserts a repeat request inside
// the window is rejected with a positive retryAfter.
func TestReservePartitionState_RejectsRepeat(t *testing.T) {
	inst := &Instance{}

	if ok, _ := inst.ReservePartitionState("Directory"); !ok {
		t.Fatal("first reservation should be admitted")
	}
	ok, retryAfter := inst.ReservePartitionState("Directory")
	if ok {
		t.Fatal("immediate repeat should be rejected")
	}
	if retryAfter <= 0 || retryAfter > PartitionStateMinInterval {
		t.Fatalf("retryAfter %v outside (0, %v]", retryAfter, PartitionStateMinInterval)
	}
}

// TestReservePartitionState_AdmitsAfterWindow uses an injected clock by
// pre-setting the last-request stamp to a time long enough in the past
// that the next call should be admitted.
func TestReservePartitionState_AdmitsAfterWindow(t *testing.T) {
	inst := &Instance{
		partitionStateLastReq: map[string]time.Time{
			"directory": time.Now().Add(-2 * PartitionStateMinInterval),
		},
	}
	if ok, _ := inst.ReservePartitionState("Directory"); !ok {
		t.Fatal("reservation older than the window should be admitted")
	}
}

// TestReservePartitionState_CaseInsensitive asserts case folding so the
// HTTP path's lowercase partition matches Querier registration.
func TestReservePartitionState_CaseInsensitive(t *testing.T) {
	inst := &Instance{}
	if ok, _ := inst.ReservePartitionState("Directory"); !ok {
		t.Fatal("first")
	}
	if ok, _ := inst.ReservePartitionState("DIRECTORY"); ok {
		t.Fatal("uppercase repeat should be rejected (same partition)")
	}
}
