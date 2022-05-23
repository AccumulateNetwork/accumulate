package e2e

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestSynthTxnToDirectory(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name(), "alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey(t.Name(), "bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)

	// Put Alice on BVN0 and Bob on the DN
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl.RootIdentity(), "Directory")

	// Create Alice
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e9)})

	// Send tokens from BVN to DN
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&SendTokens{
			To: []*TokenRecipient{{
				Url:    bobUrl,
				Amount: *big.NewInt(1e6),
			}},
		}).
		Initiate(SignatureTypeED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())
}

func TestOracleDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	dn := sim.Subnet(protocol.Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].ID)

	// TODO move back to OperatorPage in or after AC-1402
	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Network.ValidatorPage(0))
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Set price of acme to $445.00 / token
	price := 445.00
	data, err := json.Marshal(&AcmeOracle{Price: uint64(price * protocol.AcmeOraclePrecision)})
	require.NoError(t, err)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(protocol.PriceOracle()).
			WithTimestampVar(&timestamp).
			WithSigner(signer.Url, signer.Version).
			WithBody(&WriteData{Entry: &AccumulateDataEntry{Data: [][]byte{data}}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Wait for anchors to get distributed and processed
	sim.ExecuteBlocks(10)

	_ = sim.SubnetFor(dn.Executor.Network.NodeUrl()).Database.View(func(batch *database.Batch) error {
		var ledger *protocol.SystemLedger
		require.NoError(t, batch.Account(dn.Executor.Network.Ledger()).GetStateAs(&ledger))
		expected := uint64(price * protocol.AcmeOraclePrecision)
		require.Equal(t, int(expected), int(ledger.ActiveOracle))
		return nil
	})

	_ = sim.SubnetFor(bvn0.Executor.Network.NodeUrl()).Database.View(func(batch *database.Batch) error {
		var ledger *protocol.SystemLedger
		require.NoError(t, batch.Account(bvn0.Executor.Network.Ledger()).GetStateAs(&ledger))
		expected := uint64(price * protocol.AcmeOraclePrecision)
		require.Equal(t, int(expected), int(ledger.ActiveOracle))
		return nil
	})
}
