// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	"golang.org/x/exp/rand"
)

func init() { acctesting.EnableDebugFeatures() }

var rng = rand.New(rand.NewSource(0))

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rng)
	return tmed25519.PrivKey(key)
}

func doHash(b []byte) []byte {
	if b == nil {
		return nil
	}
	h := sha256.Sum256(b)
	return h[:]
}

func TestUpdateKeyPage_Priority(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, batch.Commit())

	bookUrl := protocol.AccountUrl("foo", "book")

	for _, idx := range []uint64{0, 1, 2} {
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			op := new(protocol.UpdateKeyOperation)
			kh := sha256.Sum256(testKey.PubKey().Bytes())
			nkh := sha256.Sum256(newKey.PubKey().Bytes())
			op.OldEntry.KeyHash = kh[:]
			op.NewEntry.KeyHash = nkh[:]
			body := new(protocol.UpdateKeyPage)
			body.Operation = append(body.Operation, op)

			env := acctesting.NewTransaction().
				WithPrincipal(protocol.FormatKeyPageUrl(bookUrl, 1)).
				WithSigner(protocol.FormatKeyPageUrl(bookUrl, idx), 1).
				WithTimestamp(1).
				WithBody(body).
				Initiate(protocol.SignatureTypeED25519, testKey).
				Build()

			batch := db.Begin(true)
			defer batch.Discard()

			var signer protocol.Signer
			require.NoError(t, batch.Account(env.Signatures[0].GetSigner()).GetStateAs(&signer))

			_, err := UpdateKeyPage{}.SignerIsAuthorized(nil, batch, env.Transaction[0], signer, SignatureValidationMetadata{Location: bookUrl})
			if idx <= 1 {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, fmt.Sprintf(`signer acc://foo.acme/book/%d is lower priority than the principal acc://foo.acme/book/2`, idx+1))
			}

			// Do not store state changes
		})
	}
}

func TestUpdateKeyPage_Duplicate(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	bvn := sim.PartitionFor(alice)
	sim.CreateIdentity(alice, aliceKey[32:])

	key := acctesting.GenerateKey("key")
	bob := protocol.AccountUrl("bob")

	cases := []struct {
		OldPublicKey []byte
		OldDelegate  *url.URL
		NewPublicKey []byte
		NewDelegate  *url.URL
	}{
		{key, nil, key, nil},
		{nil, bob, nil, bob},
		{key, bob, key, nil},
		{key, bob, nil, bob},
		{key, nil, key, bob},
		{nil, bob, key, bob},
		{key, bob, key, bob},
	}

	pageUrl := alice.JoinPath("book", "1")
	for _, c := range cases {
		var oldStr, newStr string
		switch {
		case c.OldPublicKey == nil:
			oldStr = "an existing delegate"
		case c.OldDelegate == nil:
			oldStr = "an existing hash"
		default:
			oldStr = "an existing delegate and hash"
		}
		switch {
		case c.NewPublicKey == nil:
			newStr = "a delegate"
		case c.NewDelegate == nil:
			newStr = "a hash"
		default:
			newStr = "both"
		}

		t.Run(fmt.Sprintf("Add %s with %s fails", newStr, oldStr), func(t *testing.T) {
			batch := bvn.Database.Begin(true)
			defer batch.Discard()

			var page *protocol.KeyPage
			require.NoError(t, batch.Account(pageUrl).Main().GetAs(&page))
			page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: doHash(c.OldPublicKey), Delegate: c.OldDelegate})

			env := acctesting.NewTransaction().
				WithPrincipal(pageUrl).
				WithSigner(pageUrl, 1).
				WithTimestamp(1).
				WithBody(&protocol.UpdateKeyPage{
					Operation: []protocol.KeyPageOperation{
						&protocol.AddKeyOperation{
							Entry: protocol.KeySpecParams{
								KeyHash:  doHash(c.NewPublicKey),
								Delegate: c.NewDelegate,
							},
						},
					},
				}).
				Initiate(protocol.SignatureTypeED25519, aliceKey).
				Build()

			st, delivery := LoadStateManagerForTest(t, batch, env)
			defer st.Discard()

			_, err := UpdateKeyPage{}.Execute(st, delivery)
			require.Errorf(t, err, "Adding an entry with %v when another entry with %v exists should fail", newStr, oldStr)
		})

		t.Run(fmt.Sprintf("Update to %s with %s fails", newStr, oldStr), func(t *testing.T) {
			batch := bvn.Database.Begin(true)
			defer batch.Discard()

			var page *protocol.KeyPage
			require.NoError(t, batch.Account(pageUrl).Main().GetAs(&page))
			page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: doHash(c.OldPublicKey), Delegate: c.OldDelegate})

			env := acctesting.NewTransaction().
				WithPrincipal(pageUrl).
				WithSigner(pageUrl, 1).
				WithTimestamp(1).
				WithBody(&protocol.UpdateKeyPage{
					Operation: []protocol.KeyPageOperation{
						&protocol.UpdateKeyOperation{
							OldEntry: protocol.KeySpecParams{
								KeyHash: doHash(aliceKey[32:]),
							},
							NewEntry: protocol.KeySpecParams{
								KeyHash:  doHash(c.NewPublicKey),
								Delegate: c.NewDelegate,
							},
						},
					},
				}).
				Initiate(protocol.SignatureTypeED25519, aliceKey).
				Build()

			st, delivery := LoadStateManagerForTest(t, batch, env)
			defer st.Discard()

			_, err := UpdateKeyPage{}.Execute(st, delivery)
			require.Errorf(t, err, "Updating an entry to %v when another entry with %v exists should fail", newStr, oldStr)
		})
	}
}

func TestUpdateKeyPage_Update(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	bvn := sim.PartitionFor(alice)
	sim.CreateIdentity(alice, aliceKey[32:])

	key := acctesting.GenerateKey("key")
	bob := protocol.AccountUrl("bob")
	charlie := protocol.AccountUrl("charlie")

	cases := []struct {
		OldPublicKey []byte
		OldDelegate  *url.URL
		NewPublicKey []byte
		NewDelegate  *url.URL
	}{
		{key, nil, key, nil},
		{nil, bob, nil, bob},
		{key, bob, key, nil},
		{key, bob, nil, bob},
		{key, nil, key, bob},
		{nil, bob, key, bob},
		{key, bob, key, bob},

		{key, nil, key, nil},
		{nil, bob, nil, charlie},
		{key, bob, key, nil},
		{key, bob, nil, charlie},
		{key, nil, key, charlie},
		{nil, bob, key, charlie},
		{key, bob, key, charlie},
	}

	pageUrl := alice.JoinPath("book", "1")
	for _, c := range cases {
		var oldStr, newStr string
		switch {
		case c.OldPublicKey == nil:
			oldStr = "an existing delegate (" + c.OldDelegate.ShortString() + ")"
		case c.OldDelegate == nil:
			oldStr = "an existing hash"
		default:
			oldStr = "an existing delegate (" + c.OldDelegate.ShortString() + ") and hash"
		}
		switch {
		case c.NewPublicKey == nil:
			newStr = "a delegate (" + c.NewDelegate.ShortString() + ")"
		case c.NewDelegate == nil:
			newStr = "a hash"
		default:
			newStr = "both (" + c.NewDelegate.ShortString() + ")"
		}

		t.Run(fmt.Sprintf("Update %s to %s succeeds", oldStr, newStr), func(t *testing.T) {
			batch := bvn.Database.Begin(true)
			defer batch.Discard()

			var page *protocol.KeyPage
			require.NoError(t, batch.Account(pageUrl).Main().GetAs(&page))
			page.AddKeySpec(&protocol.KeySpec{PublicKeyHash: doHash(c.OldPublicKey), Delegate: c.OldDelegate})

			env := acctesting.NewTransaction().
				WithPrincipal(pageUrl).
				WithSigner(pageUrl, 1).
				WithTimestamp(1).
				WithBody(&protocol.UpdateKeyPage{
					Operation: []protocol.KeyPageOperation{
						&protocol.UpdateKeyOperation{
							OldEntry: protocol.KeySpecParams{
								KeyHash:  doHash(c.OldPublicKey),
								Delegate: c.OldDelegate,
							},
							NewEntry: protocol.KeySpecParams{
								KeyHash:  doHash(c.NewPublicKey),
								Delegate: c.NewDelegate,
							},
						},
					},
				}).
				Initiate(protocol.SignatureTypeED25519, aliceKey).
				Build()

			st, delivery := LoadStateManagerForTest(t, batch, env)
			defer st.Discard()

			_, err := UpdateKeyPage{}.Execute(st, delivery)
			require.NoErrorf(t, err, "Updating an entry with %v to %v should succeed", oldStr, newStr)
		})
	}
}

func TestUpdateKeyPage_SelfDelegation_Add(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice)

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateKeyPage{Operation: []protocol.KeyPageOperation{
			&protocol.AddKeyOperation{
				Entry: protocol.KeySpecParams{
					Delegate: alice.JoinPath("book"),
				},
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	x := sim.PartitionFor(alice)
	st, txn := LoadStateManagerForTest(t, x.Database, env)
	defer st.Discard()

	_, err := UpdateKeyPage{}.Execute(st, txn)
	require.EqualError(t, err, "self-delegation is not allowed")
}

func TestUpdateKeyPage_SelfDelegation_Update(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice)

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateKeyPage{Operation: []protocol.KeyPageOperation{
			&protocol.UpdateKeyOperation{
				OldEntry: protocol.KeySpecParams{
					KeyHash: doHash(aliceKey),
				},
				NewEntry: protocol.KeySpecParams{
					KeyHash:  doHash(aliceKey),
					Delegate: alice.JoinPath("book"),
				},
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	x := sim.PartitionFor(alice)
	st, txn := LoadStateManagerForTest(t, x.Database, env)
	defer st.Discard()

	_, err := UpdateKeyPage{}.Execute(st, txn)
	require.EqualError(t, err, "self-delegation is not allowed")
}

func TestUpdateKeyPage_PageDelegate_Add(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateKeyPage{Operation: []protocol.KeyPageOperation{
			&protocol.AddKeyOperation{
				Entry: protocol.KeySpecParams{
					Delegate: alice.JoinPath("book", "1"),
				},
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	_, err := sim.SubmitAndExecuteBlock(env)
	require.EqualError(t, err, "invalid delegate acc://alice.acme/book/1: a key page is not a valid authority")
}

func TestUpdateKeyPage_PageDelegate_Update(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateKeyPage{Operation: []protocol.KeyPageOperation{
			&protocol.UpdateKeyOperation{
				OldEntry: protocol.KeySpecParams{
					KeyHash: doHash(aliceKey),
				},
				NewEntry: protocol.KeySpecParams{
					KeyHash:  doHash(aliceKey),
					Delegate: alice.JoinPath("book", "1"),
				},
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	_, err := sim.SubmitAndExecuteBlock(env)
	require.EqualError(t, err, "invalid delegate acc://alice.acme/book/1: a key page is not a valid authority")
}
