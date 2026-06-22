// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestNewGlobalsDefaultsMempoolBackpressure verifies the mempool backpressure
// threshold is defaulted in the globals (the single source of truth the node
// reads), and that an explicitly configured value is preserved.
func TestNewGlobalsDefaultsMempoolBackpressure(t *testing.T) {
	if got := NewGlobals(nil).Globals.Limits.MempoolBackpressurePercent; got != 20 {
		t.Fatalf("default MempoolBackpressurePercent = %d, want 20", got)
	}

	explicit := &GlobalValues{Globals: &protocol.NetworkGlobals{
		Limits: &protocol.NetworkLimits{MempoolBackpressurePercent: 35},
	}}
	if got := NewGlobals(explicit).Globals.Limits.MempoolBackpressurePercent; got != 35 {
		t.Fatalf("explicit MempoolBackpressurePercent = %d, want 35 (not overwritten)", got)
	}
}
