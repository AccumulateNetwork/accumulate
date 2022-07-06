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

func TestIssueTokens_Good(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})
	liteKey := acctesting.GenerateKey("lite")
	lite := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519).JoinPath(alice.ShortString(), "tokens")

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&IssueTokens{
				Recipient: lite,
				Amount:    *big.NewInt(123),
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	liteAcct := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.Equal(t, 123, int(liteAcct.Balance.Int64()))
}

func TestIssueTokens_Bad(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})
	liteKey := acctesting.GenerateKey("lite")
	lite := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519).JoinPath(protocol.ACME)

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&IssueTokens{
				Recipient: lite,
				Amount:    *big.NewInt(123),
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// All issued tokens should be returned
	issuer := simulator.GetAccount[*TokenIssuer](sim, alice.JoinPath("tokens"))
	require.Zero(t, issuer.Issued.Int64())
}
