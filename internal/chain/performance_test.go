package chain_test

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func BenchmarkPerformance(b *testing.B) {
	exec, db := setupWithGenesis(b)

	// Create the keys and URLs
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	// Add the lite account to the database
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(b, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, 1e9, 1e9))
	require.NoError(b, batch.Commit())

	// Create the transaction
	envelope := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, alice)

	for i := 0; i < b.N; i++ {
		batch := db.Begin(true)
		defer batch.Discard()
		_, err := exec.ProcessSignature(batch, envelope.Transaction, envelope.Signatures[0])
		require.NoError(b, err)
	}
}

func setupWithGenesis(t testing.TB) (*chain.Executor, *database.Database) {
	// Create logger
	logger := logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)

	// Setup database
	db := database.OpenInMemory(logger)

	// Setup network config
	key := acctesting.GenerateKey(t.Name())
	network := config.Network{
		Type:          config.BlockValidator,
		LocalSubnetID: strings.ReplaceAll(t.Name(), "_", "-"),
	}

	// Create executor
	exec, err := chain.NewNodeExecutor(chain.ExecutorOptions{
		Logger:  logger,
		Key:     key,
		Network: network,
		Router:  acctesting.NullRouter{},
	}, db)
	require.NoError(t, err)

	// Start executor
	require.NoError(t, exec.Start())

	// Build genesis block
	temp := memory.New(logger)
	_, err = genesis.Init(temp, genesis.InitOpts{
		Network:     network,
		GenesisTime: time.Now(),
		Logger:      logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: ed25519.PubKey(key[32:])},
		},
	})
	require.NoError(t, err)

	state, err := temp.MarshalJSON()
	require.NoError(t, err)

	// Initialize database
	block := new(chain.Block)
	block.Index = protocol.GenesisBlock
	block.Time = time.Now()
	block.IsLeader = true
	block.Batch = db.Begin(true)
	defer block.Batch.Discard()
	_, err = exec.InitChain(block, state)
	require.NoError(t, err)

	// Commit the batch
	err = block.Batch.Commit()
	if err != nil {
		panic(fmt.Errorf("failed to commit block: %v", err))
	}

	// Notify the executor that we comitted
	batch := db.Begin(false)
	defer batch.Discard()
	err = exec.DidCommit(block, batch)
	if err != nil {
		panic(fmt.Errorf("failed to notify governor: %v", err))
	}

	return exec, db
}
