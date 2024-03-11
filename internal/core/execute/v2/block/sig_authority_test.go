// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestDelegationPath(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	bob := protocol.AccountUrl("bob")
	bobKey := generateKey(bob)
	charlie := protocol.AccountUrl("charlie")
	charlieKey := generateKey(charlie)
	david := protocol.AccountUrl("david")
	davidKey := generateKey(david)

	type Path struct {
		Delegates []*url.URL
		Key       ed25519.PrivateKey
	}
	cases := []struct {
		Paths []Path
	}{
		// One delegated, one direct
		{Paths: []Path{
			{Delegates: nil, Key: bobKey},
			{Delegates: []*url.URL{charlie}, Key: charlieKey},
		}},

		// Both delegated, same path
		{Paths: []Path{
			{Delegates: []*url.URL{bob}, Key: bobKey},
			{Delegates: []*url.URL{charlie}, Key: charlieKey},
		}},

		// Both delegated, different paths
		{Paths: []Path{
			{Delegates: []*url.URL{bob, david}, Key: davidKey},
			{Delegates: []*url.URL{charlie, david}, Key: davidKey},
		}},
	}

	// Run the tests
	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i+1), func(t *testing.T) {
			var timestamp uint64

			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.GenesisWithVersion(GenesisTime, protocol.ExecutorVersionV2Baikonur),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice)
			MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
			MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
			MakeIdentity(t, sim.DatabaseFor(david), david, davidKey[32:])
			MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e15)})

			UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *protocol.KeyPage) {
				p.CreditBalance = 1e9
				p.AddKeySpec(&protocol.KeySpec{Delegate: bob.JoinPath("book"), PublicKeyHash: doSha256(bobKey[32:])})
				p.AddKeySpec(&protocol.KeySpec{Delegate: charlie.JoinPath("book"), PublicKeyHash: doSha256(charlieKey[32:])})
				p.AcceptThreshold = 2
			})
			UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *protocol.KeyPage) {
				p.CreditBalance = 1e9
				p.AddKeySpec(&protocol.KeySpec{Delegate: david.JoinPath("book"), PublicKeyHash: doSha256(davidKey[32:])})
			})
			UpdateAccount(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), func(p *protocol.KeyPage) {
				p.CreditBalance = 1e9
				p.AddKeySpec(&protocol.KeySpec{Delegate: david.JoinPath("book"), PublicKeyHash: doSha256(davidKey[32:])})
			})
			UpdateAccount(t, sim.DatabaseFor(david), david.JoinPath("book", "1"), func(p *protocol.KeyPage) {
				p.CreditBalance = 1e9
			})

			txn, err := build.Transaction().For(alice, "tokens").BurnTokens(1, 0).
				Done()
			require.NoError(t, err)

			var st []*protocol.TransactionStatus
			for _, p := range c.Paths {
				var path []*url.URL
				for i := len(p.Delegates) - 1; i >= 0; i-- {
					path = append(path, p.Delegates[i].JoinPath("book", "1"))
				}
				path = append(path, alice.JoinPath("book", "1"))
				st = sim.BuildAndSubmitSuccessfully(
					build.SignatureForTransaction(txn).
						Url(path[0]).
						Delegators(path[1:]...).
						Version(1).
						Timestamp(&timestamp).
						PrivateKey(p.Key))
			}

			sim.StepUntil(
				Txn(st[0].TxID).Completes(),
				Sig(st[1].TxID).AuthoritySignature().Completes())
		})
	}
}

func generateKey(seed ...interface{}) ed25519.PrivateKey {
	h := storage.MakeKey(seed...)
	return ed25519.NewKeyFromSeed(h[:])
}

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}
