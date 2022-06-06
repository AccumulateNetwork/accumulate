package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAddCredits_BurnsAcme(t *testing.T) {
	var timestamp uint64
	const issued = 1000.00
	const balance = 100.00
	const spend = 10.00

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(balance * protocol.AcmePrecision)})
	updateAccount(sim, AcmeUrl(), func(acme *TokenIssuer) { acme.Issued.SetUint64(1e3 * protocol.AcmePrecision) }) // Make it easier to read

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: alice.JoinPath("book", "1"),
				Amount:    *big.NewInt(spend * protocol.AcmePrecision),
				Oracle:    protocol.InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// var ledgerState *protocol.SystemLedger
	// require.NoError(t, ledger.GetStateAs(&ledgerState))

	// Credits I should have received
	credits := big.NewInt(protocol.CreditUnitsPerFiatUnit)                // want to obtain credits
	credits.Mul(credits, big.NewInt(int64(ledgerState.ActiveOracle)))     // fiat units / acme
	credits.Mul(credits, big.NewInt(acmeToSpendOnCredits))                // acme the user wants to spend
	credits.Div(credits, big.NewInt(int64(protocol.AcmeOraclePrecision))) // adjust the precision of oracle to real units
	credits.Div(credits, big.NewInt(int64(protocol.AcmePrecision)))       // adjust the precision of acme to spend to real units

	expectedCreditsToReceive := credits.Uint64()
	//the balance of the account should be

	ks := n.GetKeyPage("foo/book0/1")
	acct := n.GetTokenAccount("foo/tokens")
	acmeIssuer = n.GetTokenIssuer(protocol.AcmeUrl().String())
	acmeAfterBurn := acmeIssuer.Issued
	require.Equal(t, expectedCreditsToReceive, ks.CreditBalance)
	require.Equal(t, int64(acmeAmount*protocol.AcmePrecision)-acmeToSpendOnCredits, acct.Balance.Int64())
	require.Equal(t,
		protocol.FormatBigAmount(acmeBeforeBurn.Sub(&acmeBeforeBurn, big.NewInt(acmeToSpendOnCredits)), protocol.AcmePrecisionPower),
		protocol.FormatBigAmount(&acmeAfterBurn, protocol.AcmePrecisionPower))
}
