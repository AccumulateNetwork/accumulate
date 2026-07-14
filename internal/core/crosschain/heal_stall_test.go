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
// when Delivered has not advanced since the previous scan while anchors remain
// undelivered.
func TestDeliveryStalled(t *testing.T) {
	c := new(Conductor)

	// Delivered is monotonic in reality, so each scenario uses a fresh
	// destination key and a non-decreasing sequence.

	// Caught up (delivered >= produced) is never stalled.
	require.False(t, c.deliveryStalled("caughtup", 10, 10), "caught up must not heal")

	// A destination that keeps advancing is catching up on its own — never heal,
	// including on the first (deferring) observation.
	require.False(t, c.deliveryStalled("advancing", 5, 20), "first look defers")
	require.False(t, c.deliveryStalled("advancing", 8, 20), "advancing delivery must not heal")
	require.False(t, c.deliveryStalled("advancing", 12, 20), "advancing delivery must not heal")
	require.False(t, c.deliveryStalled("advancing", 20, 20), "reaching produced must not heal")

	// A destination stuck at the same Delivered across scans is genuinely stuck.
	require.False(t, c.deliveryStalled("stuck", 15, 20), "first look defers")
	require.True(t, c.deliveryStalled("stuck", 15, 20), "delivery stuck at 15 must heal")
	require.True(t, c.deliveryStalled("stuck", 15, 20), "still stuck must keep healing")
	require.False(t, c.deliveryStalled("stuck", 18, 20), "resumed delivery must stop healing")

	// Destinations are tracked independently.
	require.False(t, c.deliveryStalled("other", 3, 20), "first look at a new destination defers")
	require.True(t, c.deliveryStalled("other", 3, 20), "stuck new destination heals")
	require.True(t, c.deliveryStalled("stuck", 18, 20), "unrelated destination stuck again heals independently")
}
