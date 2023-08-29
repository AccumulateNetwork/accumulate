// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/sha256"
	"testing"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
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
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.Genesis(GenesisTime),
		)

		MakeAccount(t, sim.DatabaseFor(lite), &LiteIdentity{Url: lite, CreditBalance: 1e9})
		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		MakeIdentity(t, sim.DatabaseFor(charlie), charlie, acctesting.GenerateKey(charlie)[32:])
		UpdateAccount(t, sim.DatabaseFor(alice), alicePage, func(p *KeyPage) { p.CreditBalance = 1e9 })

		bld := build.Transaction()
		if c.Direct {
			bld = bld.For(c.IdentityUrl)
		} else {
			bld = bld.For(c.SignerUrl)
		}

		st := sim.Submit(MustBuild(t,
			bld.Body(&CreateIdentity{
				Url:        c.IdentityUrl,
				KeyHash:    bobKeyHash[:],
				KeyBookUrl: c.IdentityUrl.JoinPath("book"),
			}).SignWith(c.SignerUrl).
				Version(1).
				Timestamp(&timestamp).
				PrivateKey(c.SignerKey)))

		var didFail error
		for _, st := range st {
			if st.Error != nil {
				didFail = st.AsError()
				break
			}
		}

		if c.Success {
			// Should succeed
			require.NoError(t, didFail)
			sim.StepUntil(
				Txn(st[0].TxID).Completes())

		} else if didFail == nil {
			// Should fail or not be delivered
			sim.StepUntil(
				Txn(st[0].TxID).Fails())
		}

		// Verify
		_ = sim.DatabaseFor(c.IdentityUrl).View(func(batch *database.Batch) error {
			err := batch.Account(c.IdentityUrl).Main().GetAs(new(*ADI))
			if c.Success {
				require.NoError(t, err, "Expected the ADI to have been created")
			} else {
				require.Error(t, err, "Expected the ADI to not have been created")
				require.ErrorIs(t, err, errors.NotFound, "Expected the ADI to not have been created")
			}
			return nil
		})
	})
}

func TestCreateIdentity_Eth(t *testing.T) {
	seed := storage.MakeKey(t.Name())
	sk, pk := btc.PrivKeyFromBytes(btc.S256(), seed[:])
	lite := LiteAuthorityForKey(pk.SerializeUncompressed(), SignatureTypeETH)
	alice := AccountUrl("alice")

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeAccount(t, sim.DatabaseFor(lite), &LiteIdentity{Url: lite, CreditBalance: 1e9})

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			CreateIdentity(alice).WithKeyBook(alice, "book").WithKey(pk.SerializeUncompressed(), SignatureTypeETH).
			SignWith(lite).Version(1).Timestamp(1).Type(SignatureTypeETH).PrivateKey(sk.Serialize()))
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Verify
	GetAccount[*ADI](t, sim.DatabaseFor(alice), alice)
}
