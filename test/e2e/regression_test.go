package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
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

func TestAC1704(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	const initialBalance = 100
	aliceKey := acctesting.GenerateKey("alice")
	alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
	sim.CreateAccount(&LiteIdentity{Url: alice.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: alice, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9 * AcmePrecision)})

	// Create a few deposits
	for i := 0; i < 10; i++ {
		_, txns := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
			acctesting.NewTransaction().
				WithPrincipal(alice).
				WithSigner(alice, 1).
				WithTimestampVar(&timestamp).
				WithBody(&SendTokens{To: []*TokenRecipient{{
					Url:    AccountUrl("bob", "tokens"),
					Amount: *big.NewInt(1),
				}}}).
				Initiate(SignatureTypeED25519, aliceKey).
				Build(),
		)...)
		if i != 7 {
			continue
		}

		synthTxn := txns[1]
		require.Equal(t, TransactionTypeSyntheticDepositTokens, synthTxn.Body.Type())

		req := new(api.TxnQuery)
		req.Txid = synthTxn.GetHash()
		res, err := sim.SubnetFor(synthTxn.Header.Principal).API.QueryTx(context.Background(), req)
		require.NoError(t, err)

		var sig KeySignature
		for _, s := range res.Signatures {
			if s, ok := s.(KeySignature); ok {
				sig = s
				break
			}
		}
		require.NotNil(t, sig)

		sim.MustSubmitAndExecuteBlock(&Envelope{
			Transaction: []*Transaction{synthTxn},
			Signatures:  []Signature{sig},
		})
	}
}
