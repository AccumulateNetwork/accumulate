// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"math/big"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// changingLite builds a network with the given retention depth and a lite token
// account that changes at every one of `changes` blocks.
func changingLite(t *testing.T, depth uint64, changes int) (*Sim, *url.URL) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(depth),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(5)

	for i := 0; i < changes; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}
	return sim, lite
}

var bvn0 = config.NetworkUrl{URL: PartitionUrl("BVN0")}
