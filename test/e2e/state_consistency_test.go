// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func sendLotsOfTokens(sim *Sim, N, M int, timestamp *uint64, sender *url.URL, senderKey ed25519.PrivateKey) {
	sim.TB.Helper()

	recipients := make([]*url.URL, N)
	for i := range recipients {
		recipients[i] = acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("recipient", i))
	}

	for i := 0; i < M; i++ {
		var cond []Condition
		for i := 0; i < N; i++ {
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().
					For(sender).
					Body(&SendTokens{To: []*TokenRecipient{{
						Url:    recipients[rand.Intn(len(recipients))],
						Amount: *big.NewInt(1000),
					}}}).
					SignWith(sender).Version(1).Timestamp(timestamp).PrivateKey(senderKey))
			cond = append(cond,
				Txn(st.TxID).Completes())
		}

		sim.StepUntil(cond...)
	}
}

func TestStateRelaunch(t *testing.T) {
	const bvnCount = 3
	var timestamp uint64

	// Create sender
	senderKey := acctesting.GenerateKey("sender")
	sender := acctesting.AcmeLiteAddressStdPriv(senderKey)

	// Create databases
	stores := map[string]keyvalue.Beginner{}
	for i := 0; i < bvnCount; i++ {
		stores[fmt.Sprintf("%s-%d", Directory, i)] = memory.New(nil)
		stores[fmt.Sprintf("BVN%d-0", i)] = memory.New(nil)
	}
	openDb := func(partition *PartitionInfo, node int, logger log.Logger) keyvalue.Beginner {
		return stores[fmt.Sprintf("%s-%d", partition.ID, node)]
	}

	// [1] Setup
	s1 := NewSim(t,
		simulator.SimpleNetwork(t.Name(), bvnCount, 1),
		simulator.WithDatabase(openDb),
		simulator.Genesis(GenesisTime),
	)
	MakeAccount(t, s1.DatabaseFor(sender), &LiteIdentity{Url: sender.RootIdentity(), CreditBalance: 1e9})
	MakeAccount(t, s1.DatabaseFor(sender), &LiteTokenAccount{Url: sender, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e6 * AcmePrecision)})

	// [1] Send a bunch of tokens
	sendLotsOfTokens(s1, 10, 1, &timestamp, sender, senderKey)

	// [1] Wait a bit for everything to settle
	s1.StepN(10)

	// [1] Get the DN root hash
	var root1 []byte
	var err error
	_ = s1.Database(Directory).View(func(batch *database.Batch) error {
		root1, err = batch.Account(DnUrl().JoinPath(Ledger)).RootChain().Anchor()
		require.NoError(t, err)
		return nil
	})

	// [2] Reload (do not init)
	s2 := NewSim(t,
		simulator.SimpleNetwork(t.Name(), bvnCount, 1),
		simulator.WithDatabase(openDb),
		// Do not provide a snapshot
	)

	// [2] Check the DN root hash
	var root2 []byte
	_ = s1.Database(Directory).View(func(batch *database.Batch) error {
		root2, err = batch.Account(DnUrl().JoinPath(Ledger)).RootChain().Anchor()
		require.NoError(t, err)
		return nil
	})
	require.Equal(t, fmt.Sprintf("%X", root1), fmt.Sprintf("%X", root2), "Hash does not match after load from disk")

	// [2] Send a bunch of tokens
	sendLotsOfTokens(s2, 10, 1, &timestamp, sender, senderKey)

	// [2] Wait a bit for everything to settle
	s2.StepN(10)
}
