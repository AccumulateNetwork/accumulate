// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeliveryStalled verifies the gate that keeps anchor healing from
// re-driving a destination that is delivering on its own: healing fires only
// after Delivered has been pinned across StallScans consecutive scans while
// anchors remain undelivered. Delivered is monotonic in reality, so each
// scenario uses a fresh destination key and a non-decreasing sequence.
func TestDeliveryStalled(t *testing.T) {
	c := new(Conductor)

	// Caught up (delivered >= produced) is never stalled.
	require.False(t, c.deliveryStalled("caughtup", 10, 10), "caught up must not heal")

	// A destination that keeps advancing is catching up on its own — never heal.
	require.False(t, c.deliveryStalled("advancing", 5, 20))
	require.False(t, c.deliveryStalled("advancing", 8, 20))
	require.False(t, c.deliveryStalled("advancing", 12, 20))
	require.False(t, c.deliveryStalled("advancing", 20, 20), "reaching produced must not heal")

	// A momentary pause shorter than the window is not a stall (bursty delivery).
	require.False(t, c.deliveryStalled("bursty", 10, 20), "advance")
	for i := 1; i < StallScans; i++ {
		require.False(t, c.deliveryStalled("bursty", 10, 20), "pause within the window must not heal")
	}
	require.False(t, c.deliveryStalled("bursty", 14, 20), "delivery resumed before the window elapsed")
	require.False(t, c.deliveryStalled("bursty", 14, 20), "the stall count reset on the advance")

	// Pinned across the full window: genuinely stuck, heal.
	require.False(t, c.deliveryStalled("stuck", 15, 20), "first observation")
	for i := 1; i < StallScans; i++ {
		require.False(t, c.deliveryStalled("stuck", 15, 20), "not yet past the window")
	}
	require.True(t, c.deliveryStalled("stuck", 15, 20), "stalled across the window must heal")
	require.True(t, c.deliveryStalled("stuck", 15, 20), "still stuck keeps healing")
	require.False(t, c.deliveryStalled("stuck", 18, 20), "resumed delivery stops healing")

	// Destinations are tracked independently: a new destination starts its own
	// window and does not inherit another's stall count.
	require.False(t, c.deliveryStalled("other", 3, 20), "a new destination starts its own window")
	require.False(t, c.deliveryStalled("other", 3, 20), "still within its own window")
}
