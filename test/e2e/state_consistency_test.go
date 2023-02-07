// Copyright 2023 The Accumulate Authors
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

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func sendLotsOfTokens(sim *simulator.Simulator, N, M int, timestamp *uint64, sender *url.URL, senderKey ed25519.PrivateKey) {
	recipients := make([]*url.URL, N)
	for i := range recipients {
		recipients[i] = acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("recipient", i))
	}

	for i := 0; i < M; i++ {
		var envs []*messaging.Envelope

		for i := 0; i < N; i++ {
			envs = append(envs, acctesting.NewTransaction().
				WithPrincipal(sender).
				WithSigner(sender, 1).
				WithTimestampVar(timestamp).
				WithBody(&SendTokens{To: []*TokenRecipient{{
					Url:    recipients[rand.Intn(len(recipients))],
					Amount: *big.NewInt(1000),
				}}}).
				Initiate(SignatureTypeED25519, senderKey).
				Build())
		}

		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(envs...)...)
	}
}

func TestStateRelaunch(t *testing.T) {
	const bvnCount = 3
	var timestamp uint64

	// Create sender
	senderKey := acctesting.GenerateKey("sender")
	sender := acctesting.AcmeLiteAddressStdPriv(senderKey)

	// Create databases
	stores := map[string]storage.KeyValueStore{}
	for i := 0; i < bvnCount; i++ {
		stores[fmt.Sprintf("%s-%d", Directory, i)] = memory.New(nil)
		stores[fmt.Sprintf("BVN%d-0", i)] = memory.New(nil)
	}
	openDb := func(partition string, node int, logger log.Logger) database.Beginner {
		return database.New(stores[fmt.Sprintf("%s-%d", partition, node)], logger)
	}

	// [1] Setup
	s1 := simulator.NewWith(t, simulator.SimulatorOptions{BvnCount: bvnCount, OpenDB: openDb})
	s1.InitFromGenesis()
	s1.CreateAccount(&LiteIdentity{Url: sender.RootIdentity(), CreditBalance: 1e9})
	s1.CreateAccount(&LiteTokenAccount{Url: sender, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e6 * AcmePrecision)})

	// [1] Send a bunch of tokens
	sendLotsOfTokens(s1, 10, 1, &timestamp, sender, senderKey)

	// [1] Wait a bit for everything to settle
	s1.ExecuteBlocks(10)

	// [1] Get the DN root hash
	var root1 []byte
	var err error
	x := s1.Partition(Directory)
	_ = x.Database.View(func(batch *database.Batch) error {
		root1, err = batch.GetMinorRootChainAnchor(&x.Executor.Describe)
		require.NoError(t, err)
		return nil
	})

	// [2] Reload (do not init)
	s2 := simulator.NewWith(t, simulator.SimulatorOptions{BvnCount: bvnCount, OpenDB: openDb})
	s2.Init(func(string, *accumulated.NetworkInit, log.Logger) (ioutil2.SectionReader, error) {
		return new(ioutil2.Buffer), nil // Empty, must init from db
	})

	// [2] Check the DN root hash
	var root2 []byte
	x = s2.Partition(Directory)
	_ = x.Database.View(func(batch *database.Batch) error {
		root2, err = batch.GetMinorRootChainAnchor(&x.Executor.Describe)
		require.NoError(t, err)
		return nil
	})
	require.Equal(t, fmt.Sprintf("%X", root1), fmt.Sprintf("%X", root2), "Hash does not match after load from disk")

	// [2] Send a bunch of tokens
	sendLotsOfTokens(s2, 10, 1, &timestamp, sender, senderKey)

	// [2] Wait a bit for everything to settle
	s2.ExecuteBlocks(10)
}
