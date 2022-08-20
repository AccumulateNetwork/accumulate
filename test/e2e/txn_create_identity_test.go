package e2e

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func TestCreateIdentity(t *testing.T) {
	type Case struct {
		SignerUrl   *url.URL // URL of the page/lite id
		SignerKey   []byte   // Key of the page/lite id
		IdentityUrl *url.URL // URL of the identity to create
		Direct      bool     // Principal = Identity (true) or Signer (false)
		Success     bool     // Should succeed?
	}

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity()
	alice := AccountUrl("alice")
	alicePage := alice.JoinPath("book", "1")
	aliceKey := acctesting.GenerateKey(alice)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)
	bobKeyHash := sha256.Sum256(bobKey[32:])
	charlie := AccountUrl("charlie")

	cases := map[string]Case{
		"Root, Lite, Direct":       {lite, liteKey, AccountUrl("bob"), true, true},
		"Root, Lite, Indirect":     {lite, liteKey, AccountUrl("bob"), false, true},
		"Root, Page, Direct":       {alicePage, aliceKey, AccountUrl("bob"), true, true},
		"Root, Page, Indirect":     {alicePage, aliceKey, AccountUrl("bob"), false, true},
		"Non-root, Lite, Direct":   {lite, liteKey, charlie.JoinPath("sub"), true, false},
		"Non-root, Lite, Indirect": {lite, liteKey, charlie.JoinPath("sub"), false, false},
		"Non-root, Page, Direct":   {alicePage, aliceKey, charlie.JoinPath("sub"), true, false},
		"Non-root, Page, Indirect": {alicePage, aliceKey, charlie.JoinPath("sub"), false, false},
	}

	Run(t, cases, func(t *testing.T, c Case) {
		var timestamp uint64

		// Initialize
		sim := simulator.New(t, 3)
		sim.InitFromGenesis()

		sim.CreateAccount(&LiteIdentity{Url: lite, CreditBalance: 1e9})
		sim.CreateIdentity(alice, aliceKey[32:])
		sim.CreateIdentity(charlie, acctesting.GenerateKey(charlie)[32:])
		updateAccount(sim, alicePage, func(p *KeyPage) { p.CreditBalance = 1e9 })

		bld := acctesting.NewTransaction()
		if c.Direct {
			bld = bld.WithPrincipal(c.IdentityUrl)
		} else {
			bld = bld.WithPrincipal(c.SignerUrl)
		}

		st, err := sim.SubmitAndExecuteBlock(
			bld.WithSigner(c.SignerUrl, 1).
				WithTimestampVar(&timestamp).
				WithBody(&CreateIdentity{
					Url:        c.IdentityUrl,
					KeyHash:    bobKeyHash[:],
					KeyBookUrl: c.IdentityUrl.JoinPath("book"),
				}).
				Initiate(SignatureTypeED25519, c.SignerKey).
				Build(),
		)
		if c.Success {
			require.NoError(t, err, "Expected the transaction to succeed")
		} else if err != nil {
			return // Failed to validate
		}

		h := st[0].TxID.Hash()
		if c.Success {
			// Should succeed
			st, _ = sim.WaitForTransactionFlow(delivered, h[:])
			require.Equal(t, errors.StatusDelivered, st[0].Code, "Expected the transaction to succeed")
		} else {
			// Should fail or not be delivered
			_, st, _ := sim.WaitForTransaction(delivered, h[:], 50)
			if st != nil {
				require.True(t, st.Failed(), "Expected the transaction to fail")
			}
		}

		// Verify
		_ = sim.PartitionFor(c.IdentityUrl).Database.View(func(batch *database.Batch) error {
			err := batch.Account(c.IdentityUrl).Main().GetAs(new(*ADI))
			if c.Success {
				require.NoError(t, err, "Expected the ADI to have been created")
			} else {
				require.Error(t, err, "Expected the ADI to not have been created")
				require.ErrorIs(t, err, errors.StatusNotFound, "Expected the ADI to not have been created")
			}
			return nil
		})
	})
}
