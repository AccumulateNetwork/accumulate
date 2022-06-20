package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func TestIssueAC1555(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	const initialBalance = 100
	alice := AccountUrl("alice")
	aliceKey, liteKey := acctesting.GenerateKey(alice), acctesting.GenerateKey("lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)
	sim.CreateAccount(&LiteIdentity{Url: liteUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: liteUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9 * AcmePrecision)})
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = initialBalance * CreditPrecision })

	// Add credits
	const additionalBalance = 99
	const oracle = InitialAcmeOracle * AcmeOraclePrecision //nolint
	acme := big.NewInt(AcmePrecision)
	acme.Mul(acme, big.NewInt(additionalBalance))
	acme.Div(acme, big.NewInt(CreditsPerDollar))
	acme.Mul(acme, big.NewInt(AcmeOraclePrecision))
	acme.Div(acme, big.NewInt(oracle)) //nolint
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl).
			WithSigner(liteUrl, 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: alice.JoinPath("book", "1"),
				Amount:    *acme,
				Oracle:    oracle,
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)...)

	// The balance should be added
	page := simulator.GetAccount[*KeyPage](sim, alice.JoinPath("book", "1"))
	require.Equal(t, int((initialBalance+additionalBalance)*CreditPrecision), int(page.CreditBalance))
}

func TestQueryKeyIndexWithRemoteAuthority(t *testing.T) {
	// var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	alice, bob := AccountUrl("alice"), AccountUrl("bob")
	aliceKey, bobKey := acctesting.GenerateKey(alice), acctesting.GenerateKey(bob)
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])

	sim.CreateAccount(&TokenAccount{
		Url:         alice.JoinPath("managed-tokens"),
		AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book")}}},
		TokenUrl:    AcmeUrl(),
	})

	// Query key
	req := new(query.RequestKeyPageIndex)
	req.Url = alice.JoinPath("managed-tokens")
	req.Key = aliceKey[32:]
	x := sim.SubnetFor(req.Url)
	_ = x.Database.View(func(batch *database.Batch) error {
		// The query MUST fail with "no authority of ... holds ..." NOT with
		// "account ... not found"
		_, _, err := x.Executor.Query(batch, req, 0, false)
		require.EqualError(t, err, fmt.Sprintf("no authority of %s holds %X", req.Url, req.Key))
		return nil
	})
}

func TestAddCreditsToLiteIdentityOnOtherBVN(t *testing.T) {
	// Tests AC-1859
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	sendKey, recvKey := acctesting.GenerateKey("sender"), acctesting.GenerateKey("receiver")
	sender, receiver := acctesting.AcmeLiteAddressStdPriv(sendKey), acctesting.AcmeLiteAddressStdPriv(recvKey)
	sim.SetRouteFor(sender.RootIdentity(), "BVN0")
	sim.SetRouteFor(receiver.RootIdentity(), "BVN1")
	sim.CreateAccount(&LiteIdentity{Url: sender.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: sender, Balance: *big.NewInt(1e12), TokenUrl: AcmeUrl()})

	// Add credits
	const creditAmount = 99
	const oracle = InitialAcmeOracle * AcmeOraclePrecision //nolint
	acme := big.NewInt(AcmePrecision)
	acme.Mul(acme, big.NewInt(creditAmount))
	acme.Div(acme, big.NewInt(CreditsPerDollar))
	acme.Mul(acme, big.NewInt(AcmeOraclePrecision))
	acme.Div(acme, big.NewInt(oracle)) //nolint
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(sender).
			WithSigner(sender, 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: receiver,
				Amount:    *acme,
				Oracle:    oracle,
			}).
			Initiate(SignatureTypeED25519, sendKey).
			Build(),
	)...)

	// Verify
	recvId := simulator.GetAccount[*LiteIdentity](sim, receiver.RootIdentity())
	require.Equal(t, int(creditAmount*CreditPrecision), int(recvId.CreditBalance))
}

func TestFaucetMultiNetwork(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	liteKey := acctesting.GenerateKey("Lite")
	lite := sim.CreateLiteTokenAccount(liteKey, AcmeUrl(), 1e9, 1e12)

	// Set the lite account routing to a different BVN from the faucet
	faucetBvn := sim.SubnetFor(FaucetUrl)
	for _, subnet := range sim.Subnets[1:] {
		if faucetBvn.Subnet.Id != subnet.Id {
			sim.SetRouteFor(lite.RootIdentity(), subnet.Id)
			break
		}
	}

	// Execute
	resp, err := sim.Executors[Directory].API.Faucet(context.Background(), &AcmeFaucet{Url: lite})
	require.NoError(t, err)
	sim.WaitForTransactionFlow(delivered, resp.TransactionHash)

	// Verify
	lta := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.NotZero(t, lta.Balance.Int64())
}
