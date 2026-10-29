// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// From v2-canaveral the system ledger records each block's time below the
// second, beside Timestamp, which records it to the second; a node that
// joins or restarts reads the clamp's previous time from it (consensus spec,
// "Restart"). Before v2-canaveral the field is never written.
func TestSystemLedgerRecordsTheBlockTime(t *testing.T) {
	for _, version := range []protocol.ExecutorVersion{protocol.ExecutorVersionV2Kourou, protocol.ExecutorVersionV2Canaveral} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 2, 1),
				simulator.GenesisWithVersion(GenesisTime, version),
			)
			sim.StepN(5)

			for _, part := range []string{protocol.Directory, "BVN0", "BVN1"} {
				ledger := GetAccount[*protocol.SystemLedger](t, sim.Database(part), protocol.PartitionUrl(part).JoinPath(protocol.Ledger))
				if version.V2CanaveralEnabled() {
					require.Equal(t, uint64(ledger.Timestamp.Nanosecond()), ledger.TimestampNanosecond, "%s", part)
				} else {
					require.Zero(t, ledger.TimestampNanosecond, "%s", part)
				}
			}
		})
	}
}
