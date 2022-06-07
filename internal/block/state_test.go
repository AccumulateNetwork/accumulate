package block_test

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestStateSaveAndRestore(t *testing.T) {
	var timestamp uint64

	// Initialize
	t.Log("Setup")
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Prepare the ADI
	name := protocol.AccountUrl("foo")
	key := acctesting.GenerateKey(t.Name(), name)
	SetupIdentity(sim, name, key, &timestamp)

	// Create snapshots
	t.Log("Save")
	dir := t.TempDir()
	filename := func(subnet string) string {
		return filepath.Join(dir, fmt.Sprintf("%s.bpt", subnet))
	}
	for _, subnet := range sim.Subnets {
		x := sim.Subnet(subnet.Name)
		batch := x.Database.Begin(false)
		defer batch.Discard()
		f, err := os.Create(filename(subnet.Name))
		require.NoError(t, err)
		require.NoError(t, x.Executor.SaveSnapshot(batch, f))
		require.NoError(t, f.Close())
	}

	// Create a new network
	t.Log("Restore")
	sim = simulator.New(t, 3)
	sim.InitFromSnapshot(filename)

	// Send tokens to a lite account
	liteUrl := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey(sim.Name(), "Recipient"))
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(name.JoinPath("tokens")).
			WithSigner(name.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{
					{Url: liteUrl, Amount: *big.NewInt(68)},
				},
			}).
			Initiate(SignatureTypeED25519, key).
			Build(),
	)...)
}

func SetupIdentity(sim *simulator.Simulator, name *url.URL, key []byte, timestamp *uint64) {
	// Fund a lite account
	liteKey := acctesting.GenerateKey(sim.Name(), "SetupIdentity", name)
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(FaucetUrl).
			WithBody(&AcmeFaucet{Url: liteUrl}).
			Faucet(),
	)...)

	// Add credits to the lite account
	const liteCreditAmount = 1e3
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: liteUrl,
				Amount:    *big.NewInt(AcmePrecision * liteCreditAmount),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Create the ADI
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateIdentity{
				Url:        name,
				KeyBookUrl: name.JoinPath("book"),
				KeyHash:    doSha256(key[32:]),
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Add credits to the key page
	const tokenAccountAmount = 1e5
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: name.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * (AcmeFaucetAmount - liteCreditAmount - tokenAccountAmount)),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// Create a token account
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(name).
			WithSigner(name.JoinPath("book", "1"), 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateTokenAccount{
				Url:      name.JoinPath("tokens"),
				TokenUrl: protocol.AcmeUrl(),
			}).
			Initiate(SignatureTypeED25519, key).
			Build(),
	)...)

	// Send tokens to the ADI token account
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&SendTokens{
				To: []*TokenRecipient{
					{Url: name.JoinPath("tokens"), Amount: *big.NewInt(tokenAccountAmount * protocol.AcmePrecision)},
				},
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)
}
