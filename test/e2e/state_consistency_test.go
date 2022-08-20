package e2e

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func sendLotsOfTokens(sim *simulator.Simulator, N, M int, timestamp *uint64, sender *url.URL, senderKey ed25519.PrivateKey) {
	recipients := make([]*url.URL, N)
	for i := range recipients {
		recipients[i] = acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("recipient", i))
	}

	for i := 0; i < M; i++ {
		var envs []*Envelope

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
	stores[Directory] = memory.New(nil)
	for i := 0; i < bvnCount; i++ {
		stores[fmt.Sprintf("BVN%d", i)] = memory.New(nil)
	}
	openDb := func(partition string, _ int, logger log.Logger) *database.Database {
		return database.New(stores[partition], logger)
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
