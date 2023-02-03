// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	newsim "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func updateAccount[T Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		if err != nil {
			sim.Log(err)
			sim.FailNow()
		}

		fn(typed)
	})
}

func updateAccountNew[T Account](sim *newsim.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		require.NoError(sim.TB, err)
		fn(typed)
	})
}

func updatePartitionFor(sim *simulator.Simulator, account *url.URL, fn func(batch *database.Batch)) {
	_ = sim.PartitionFor(account).Database.Update(func(batch *database.Batch) error {
		fn(batch)
		return nil
	})
}

func viewPartitionFor(sim *simulator.Simulator, account *url.URL, fn func(batch *database.Batch)) {
	_ = sim.PartitionFor(account).Database.View(func(batch *database.Batch) error {
		fn(batch)
		return nil
	})
}

func TestDelegatedSignatureDepth(tt *testing.T) {
	// Initialize
	sim := simulator.New(tt, 1)
	sim.InitFromGenesis()
	x := sim.PartitionFor(AccountUrl("foo"))
	exec := x.Executor
	t := acctesting.NewBatchTest(tt, x.Database)
	defer t.Discard()

	// Foo's first authority and signer
	authority := new(acctesting.FakeAuthority)
	authority.Url = protocol.AccountUrl("foo", "authority")
	authority.AddAuthority(authority.Url)
	t.PutAccount(authority)

	key := acctesting.GenerateKey(tt.Name())
	signer := new(acctesting.FakeSigner)
	signer.Url = authority.Url.JoinPath("signer")
	signer.Version = 1
	signer.CreditBalance = 1e9
	signer.Keys = []*KeySpec{{PublicKeyHash: doSha256(key[32:])}}
	t.PutAccount(signer)

	// Foo's account
	account := new(acctesting.FakeAccount)
	account.Url = protocol.AccountUrl("foo", "account")
	account.AddAuthority(authority.Url)
	t.PutAccount(account)

	// The transaction
	body := new(acctesting.FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = account.Url
	txn.Body = body
	delivery := new(chain.Delivery)
	delivery.Transaction = txn

	// Use a fake executor that overrides TransactionIsReady
	exec.SetExecutor_TESTONLY(executor{typ: body.TheType})

	// The key signature
	builder := new(signing.Builder).
		SetUrl(signer.Url).
		SetPrivateKey(key).
		SetVersion(signer.Version)

	// It should succeed when under the limit
	for i := 0; i < protocol.DelegationDepthLimit; i++ {
		builder.AddDelegator(AccountUrl("baz", fmt.Sprint(i)))
	}

	sig, err := builder.Sign(txn.GetHash())
	require.NoError(t, err)
	require.NoError(t, exec.ValidateSignature(t.Batch, delivery, new(TransactionStatus), sig))

	// It should fail when over the limit
	builder.AddDelegator(AccountUrl("baz", fmt.Sprint(protocol.DelegationDepthLimit)))

	sig, err = builder.Sign(txn.GetHash())
	require.NoError(t, err)
	err = exec.ValidateSignature(t.Batch, delivery, new(TransactionStatus), sig)
	require.EqualError(t, err, fmt.Sprintf("delegated signature exceeded the depth limit (%d)", protocol.DelegationDepthLimit))
}

func TestDelegatedSignature_Local(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	alice := AccountUrl("alice")
	key1, key2 := acctesting.GenerateKey(alice), acctesting.GenerateKey(alice, 2)
	sim.CreateIdentity(alice, key1[32:])
	sim.CreateKeyBook(alice.JoinPath("other-book"), key2[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("other-book")})
	})
	updateAccount(sim, alice.JoinPath("other-book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(alice.JoinPath("other-book", "1"), 1).
			WithDelegator(alice.JoinPath("book", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}
func TestDelegatedSignature_LocalMultisig(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	alice := AccountUrl("alice")
	key1, otherKey1, otherKey2 := acctesting.GenerateKey(alice), acctesting.GenerateKey(alice, 1), acctesting.GenerateKey(alice, 2)
	sim.CreateIdentity(alice, key1[32:])
	sim.CreateKeyBook(alice.JoinPath("other-book"), otherKey1[32:], otherKey2[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("other-book")})
	})
	updateAccount(sim, alice.JoinPath("other-book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AcceptThreshold = 2
	})
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("data")).
		WithBody(&WriteData{
			Entry: &AccumulateDataEntry{Data: [][]byte{
				[]byte("foo"),
			}},
		}).
		WithSigner(alice.JoinPath("other-book", "1"), 1).
		WithDelegator(alice.JoinPath("book", "1")).
		WithTimestampVar(&timestamp).
		Initiate(SignatureTypeED25519, otherKey1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, otherKey2).
		Build())
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_Double(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	alice := AccountUrl("alice")
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updatePartitionFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), alice.String(), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, alice.JoinPath("book1").String(), tmed25519.PubKey(key2[32:])))
		require.NoError(t, acctesting.AddCredits(batch, alice.JoinPath("book1", "1"), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, alice.JoinPath("book2").String(), tmed25519.PubKey(key3[32:])))
		require.NoError(t, acctesting.AddCredits(batch, alice.JoinPath("book2", "1"), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book1")})
		}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book1", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book2")})
		}))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(alice.JoinPath("book2", "1"), 1).
			WithDelegator(alice.JoinPath("book1", "1")).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_RemoteDelegate(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice, bob := AccountUrl("alice"), AccountUrl("bob")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")

	// Setup
	key1, key2 := acctesting.GenerateKey(), acctesting.GenerateKey()
	updatePartitionFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), alice.String(), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateKeyPage(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book0")})
		}))
	})
	updatePartitionFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), bob.String(), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(bob.JoinPath("book0", "1"), 1).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_RemoteDelegator(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice, bob := AccountUrl("alice"), AccountUrl("bob")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updatePartitionFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), alice.String(), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data"), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book0")}}}}))
	})
	updatePartitionFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), bob.String(), 1e9))
		require.NoError(t, acctesting.CreateKeyBook(batch, bob.JoinPath("book1").String(), tmed25519.PubKey(key3[32:])))
		require.NoError(t, acctesting.AddCredits(batch, bob.JoinPath("book1", "1"), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book1")})
		}))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(bob.JoinPath("book1", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_RemoteDelegateAndAuthority(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice, bob, charlie := AccountUrl("alice"), AccountUrl("bob"), AccountUrl("charlie")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.SetRouteFor(charlie, "BVN2")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updatePartitionFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), alice.String(), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data"), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book0")}}}}))
	})
	updatePartitionFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), bob.String(), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book0")})
		}))
	})
	updatePartitionFor(sim, charlie, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key3), charlie.String(), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(charlie.JoinPath("book0", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_DobuleRemote(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice, bob, charlie := AccountUrl("alice"), AccountUrl("bob"), AccountUrl("charlie")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.SetRouteFor(charlie, "BVN2")

	// Setup
	key1, key2, key3 := acctesting.GenerateKey(), acctesting.GenerateKey(), acctesting.GenerateKey()
	updatePartitionFor(sim, alice, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key1), alice.String(), 1e9))
		require.NoError(t, acctesting.CreateAccount(batch, &DataAccount{Url: alice.JoinPath("data")}))
		require.NoError(t, acctesting.UpdateAccount(batch, alice.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book0")})
		}))
	})
	updatePartitionFor(sim, bob, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key2), bob.String(), 1e9))
		require.NoError(t, acctesting.UpdateAccount(batch, bob.JoinPath("book0", "1"), func(page *KeyPage) {
			page.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book0")})
		}))
	})
	updatePartitionFor(sim, charlie, func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, tmed25519.PrivKey(key3), charlie.String(), 1e9))
	})

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(charlie.JoinPath("book0", "1"), 1).
			WithDelegator(bob.JoinPath("book0", "1")).
			WithDelegator(alice.JoinPath("book0", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, key2).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))
	})
}

func TestDelegatedSignature_Multisig(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice, bob, charlie := AccountUrl("alice"), AccountUrl("bob"), AccountUrl("charlie")
	sim.SetRouteFor(alice, "BVN0")
	sim.SetRouteFor(bob, "BVN1")
	sim.SetRouteFor(charlie, "BVN2")

	// Setup
	bobK1, bobK2, charlieK1, charlieK2 := acctesting.GenerateKey(bob, 1), acctesting.GenerateKey(bob, 2), acctesting.GenerateKey(charlie, 1), acctesting.GenerateKey(charlie, 2)
	sim.CreateIdentity(alice, acctesting.GenerateKey(alice)[32:])
	sim.CreateIdentity(bob, bobK1[32:])
	sim.CreateKeyPage(bob.JoinPath("book"), bobK2[32:])
	sim.CreateIdentity(charlie, charlieK1[32:], charlieK2[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AcceptThreshold = 2
		page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		page.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
	})
	updateAccount(sim, bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})
	updateAccount(sim, bob.JoinPath("book", "2"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})
	updateAccount(sim, charlie.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AcceptThreshold = 2
	})
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Initiate with bob/book/1
	_, txn := sim.WaitForTransactions(pending, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("data")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithSigner(bob.JoinPath("book", "1"), 1).
			WithDelegator(alice.JoinPath("book", "1")).
			WithTimestampVar(&timestamp).
			Initiate(SignatureTypeED25519, bobK1).
			Build(),
	)...)
	txnHash := txn[0].GetHash()

	// Sign with bob/book/2
	sim.WaitForTransactions(pending, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(txn[0]).
			WithSigner(bob.JoinPath("book", "2"), 1).
			WithDelegator(alice.JoinPath("book", "1")).
			WithTimestampVar(&timestamp).
			Sign(SignatureTypeED25519, bobK2).
			Build(),
	)...)

	// Sign with charlie/book/1
	sim.WaitForTransactions(pending, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(txn[0]).
			WithSigner(charlie.JoinPath("book", "1"), 1).
			WithDelegator(alice.JoinPath("book", "1")).
			WithTimestampVar(&timestamp).
			Sign(SignatureTypeED25519, charlieK1).
			Build(),
	)...)

	// Sign with charlie/book/1
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(txn[0]).
			WithSigner(charlie.JoinPath("book", "1"), 1).
			WithDelegator(alice.JoinPath("book", "1")).
			WithTimestampVar(&timestamp).
			Sign(SignatureTypeED25519, charlieK2).
			Build(),
	)...)

	// Validate
	viewPartitionFor(sim, alice, func(batch *database.Batch) {
		de, _, _, err := indexing.Data(batch, alice.JoinPath("data")).GetLatestEntry()
		require.NoError(t, err)
		require.Equal(t, "foo", string(de.GetData()[0]))

		// alice/book/1 should have one delegated signature each from bob/book/1
		// and charlie/book/1
		record := batch.Transaction(txnHash)
		status, err := record.GetStatus()
		require.NoError(t, err)
		sigs, err := GetAllSignatures(batch, record, status, nil)
		require.NoError(t, err)
		require.Len(t, sigs, 2)
		require.IsType(t, (*DelegatedSignature)(nil), sigs[0])
		sig := sigs[0].(*DelegatedSignature)
		require.Equal(t, "bob.acme/book/1", sig.GetSigner().ShortString())
		require.IsType(t, (*DelegatedSignature)(nil), sigs[1])
		sig = sigs[1].(*DelegatedSignature)
		require.Equal(t, "charlie.acme/book/1", sig.GetSigner().ShortString())
	})
}

func GetAllSignatures(batch *database.Batch, transaction *database.Transaction, status *TransactionStatus, txnInitHash []byte) ([]Signature, error) {
	signatures := make([]Signature, 1)

	for _, signer := range status.Signers {
		sigset, err := database.GetSignaturesForSigner(transaction, signer)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		for _, sig := range sigset {
			if SignatureDidInitiate(sig, txnInitHash, nil) {
				signatures[0] = sig
			} else {
				signatures = append(signatures, sig)
			}
		}
	}

	if signatures[0] == nil {
		signatures = signatures[1:]
	}

	return signatures, nil
}
