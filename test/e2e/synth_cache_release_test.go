// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The producer cache holds the entries in play, not an hour of history: every
// synthetic a partition dispatches carries its Delivered for the reverse
// stream, and the destination drops what it holds for that partition at or
// below it (healing spec, "The cache"). With traffic flowing both ways, each
// side's cache for the other stays at the undelivered tail.
func TestSyntheticCacheReleasesOnDelivered(t *testing.T) {
	var ts0, ts1 uint64
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	// One account on each partition, sending to the other
	var a, b *url.URL
	var ak, bk []byte
	for i := 0; a == nil || b == nil; i++ {
		key := acctesting.GenerateKey("release", i)
		u := acctesting.AcmeLiteAddressStdPriv(key)
		part, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		switch {
		case part == "BVN0" && a == nil:
			a, ak = u, key
		case part == "BVN1" && b == nil:
			b, bk = u, key
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(a), ak[32:], AcmeUrl())
	MakeLiteTokenAccount(t, sim.DatabaseFor(b), bk[32:], AcmeUrl())

	const transfers = 40
	var st []*protocol.TransactionStatus
	for i := 0; i < transfers; i++ {
		st = append(st, sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(a).SendTokens(1, 0).To(b).
				SignWith(a).Version(1).Timestamp(&ts0).PrivateKey(ak))))
		st = append(st, sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(b).SendTokens(1, 0).To(a).
				SignWith(b).Version(1).Timestamp(&ts1).PrivateKey(bk))))
		sim.StepN(1)
	}
	for _, st := range st {
		sim.StepUntilN(200, Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	// Let the last deliveries be reported back on the reverse traffic
	sim.StepN(20)

	// The word has been heard, but a validator of the destination that
	// restarted as it executed pulls these back before its first block, so
	// they stay a grace of blocks (healing spec, "Rejoining"; #4290)
	for _, part := range []string{"BVN0", "BVN1"} {
		entries, _ := sim.S.Partition(part).SynthCache().Len()
		t.Logf("%s cache holds %d entries after %d transfers each way, within the grace", part, entries, transfers)
		require.GreaterOrEqual(t, entries, transfers/2, "%s: the cache keeps what it released for a rejoining node", part)
	}
	sim.StepN(synthcache.RejoinGrace)
	for _, part := range []string{"BVN0", "BVN1"} {
		entries, _ := sim.S.Partition(part).SynthCache().Len()
		t.Logf("%s cache holds %d entries once the grace has run", part, entries)
		require.Less(t, entries, transfers/2, "%s: the cache should hold only the undelivered tail, not the history", part)
	}
}
