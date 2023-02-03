// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/simulator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { EnableDebugFeatures() }

func TestTransactionIsReady(tt *testing.T) {
	// Initialize
	sim := simulator.New(tt, 1)
	sim.InitFromGenesis()
	x := sim.Partition(sim.Partitions[0].Id)
	exec := x.Executor
	t := NewBatchTest(tt, x.Database)
	defer t.Discard()

	// Foo's first authority and signer
	authority := new(FakeAuthority)
	authority.Url = protocol.AccountUrl("foo", "authority")
	authority.AddAuthority(authority.Url)
	t.PutAccount(authority)

	signer := new(FakeSigner)
	signer.Url = authority.Url.JoinPath("signer")
	signer.Version = 1
	t.PutAccount(signer)

	// Foo's second authority and signer
	authority2 := new(FakeAuthority)
	authority2.Url = protocol.AccountUrl("foo", "authority2")
	authority2.AddAuthority(authority2.Url)
	t.PutAccount(authority2)

	signer2 := new(FakeSigner)
	signer2.Url = authority2.Url.JoinPath("signer")
	signer2.Version = 1
	t.PutAccount(signer2)

	// An authority and signer that will not be an authority of foo
	unauthAuthority := new(FakeAuthority)
	unauthAuthority.Url = url.MustParse("foo/unauth-authority")
	unauthAuthority.AddAuthority(unauthAuthority.Url)
	t.PutAccount(unauthAuthority)

	unauthSigner := new(FakeSigner)
	unauthSigner.Url = unauthAuthority.Url.JoinPath("signer")
	unauthSigner.Version = 1
	t.PutAccount(unauthSigner)

	// An authority and signer belonging to a different root identity
	remoteAuthority := new(FakeAuthority)
	remoteAuthority.Url = protocol.AccountUrl("bar", "authority")
	remoteAuthority.AddAuthority(remoteAuthority.Url)
	t.PutAccount(remoteAuthority)

	remoteSigner := new(FakeSigner)
	remoteSigner.Url = remoteAuthority.Url.JoinPath("signer")
	remoteSigner.Version = 1
	t.PutAccount(remoteSigner)

	// Foo's account
	account := new(FakeAccount)
	account.Url = protocol.AccountUrl("foo", "account")
	account.AddAuthority(authority.Url)
	t.PutAccount(account)

	// The transaction
	body := new(FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = account.Url
	txn.Body = body
	delivery := new(chain.Delivery)
	delivery.Transaction = txn

	// The first signature
	sig := new(FakeSignature)
	sig.Signer = signer.Url
	sig.SignerVersion = signer.Version
	sig.Timestamp = 1
	sig.PublicKey = []byte{1}

	// Add a bogus initiator
	_ = t.Transaction(txn.GetHash()).PutStatus(&protocol.TransactionStatus{Initiator: protocol.AccountUrl("x")})

	// Singlesig unsigned
	t.Run("Unsigned", func(t BatchTest) {
		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Singlesig ready
	t.Run("Ready", func(t BatchTest) {
		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multisig pending
	t.Run("Multisig Pending", func(t BatchTest) {
		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multisig ready
	t.Run("Multisig Ready", func(t BatchTest) {
		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		t.AddSignature(txn.GetHash(), 0, sig)

		sig2 := sig.Copy()
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 1, sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multibook singlesig with one book ready
	t.Run("Multibook Pending", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook singlesig with all books ready
	t.Run("Multibook Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		t.AddSignature(txn.GetHash(), 0, sig)

		sig2 := sig.Copy()
		sig2.Signer = signer2.Url
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 1, sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Disabled book with no signatures
	t.Run("Disabled Unsigned", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Disabled book with an unauthorized signature
	t.Run("Disabled Single", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		sig := sig.Copy()
		sig.Signer = unauthSigner.Url
		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multibook with one book disabled and an unauthorized signature
	t.Run("Disabled Pending", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook with one disabled book and an authorized signature for the
	// enabled book
	t.Run("Disabled Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		sig1 := sig.Copy()
		sig1.Signer = url.MustParse("foo/non-authority/signer")
		t.AddSignature(txn.GetHash(), 0, sig1)

		sig2 := sig.Copy()
		sig2.Signer = signer2.Url
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 1, sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multisig with an invalidated signature and a new signature
	t.Run("Invalidated", func(t BatchTest) {
		// This is not a unit test, because it's verifying AddSigner,
		// AddSignature, and TransactionIsReady in combination not in isolation, account.
		// But that's ok.

		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		// Signature @ version 1
		t.AddSignature(txn.GetHash(), 0, sig)
		require.Equal(t, 1, t.GetSignatures(txn.GetHash(), signer.Url).Count())

		// Update the version
		signer.Version = 2

		// Signature @ version 2
		sig2 := sig.Copy()
		sig2.SignerVersion = 2
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 1, sig2)
		require.Equal(t, 1, t.GetSignatures(txn.GetHash(), signer.Url).Count())

		// Transaction is not ready
		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook multisig with old signatures that are still valid because they're the same version
	t.Run("Old Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2
		signer2 := t.PutAccountCopy(signer2).(*FakeSigner)
		signer2.Threshold = 2

		// Two signatures for signer 1 @ version 1
		sig1_1 := sig.Copy()
		sig1_2 := sig.Copy()
		sig1_2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 0, sig1_1)
		t.AddSignature(txn.GetHash(), 1, sig1_2)

		signer.Version++
		signer2.Version++

		// Two signatures for signer 1 @ version 1
		sig2_1 := sig.Copy()
		sig2_1.Signer = signer2.Url
		sig2_1.SignerVersion = 2
		sig2_2 := sig2_1.Copy()
		sig2_2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), 0, sig2_1)
		t.AddSignature(txn.GetHash(), 1, sig2_2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})
}

func TestAddAuthority(tt *testing.T) {
	// Initialize
	sim := simulator.New(tt, 1)
	sim.InitFromGenesis()

	// Main identity
	alice := protocol.AccountUrl("alice")
	aliceKey := GenerateKey(tt.Name(), alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateAccount(&protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e9)})
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	// Second identity
	bob := protocol.AccountUrl("bob")
	bobKey := GenerateKey(tt.Name(), bob)
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	// Test setup
	execAlice := sim.PartitionFor(alice).Executor
	execBob := sim.PartitionFor(bob).Executor
	t := NewBatchTest(tt, sim.PartitionFor(alice).Database)
	defer t.Discard()

	t.Run("UpdateAccountAuthority.Add", func(t BatchTest) {
		tx := NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{
				&protocol.AddAccountAuthorityOperation{Authority: bob.JoinPath("book")},
			}}).
			Initiate(protocol.SignatureTypeED25519, aliceKey).
			WithSigner(bob.JoinPath("book", "1"), 1).
			Sign(protocol.SignatureTypeED25519, bobKey).
			BuildDelivery()

		// First signature is accepted
		_, err := execAlice.ProcessSignature(t.Batch, tx, tx.Signatures[0])
		require.NoError(t, err)

		// Transaction is not ready
		status, err := t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err := execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.False(t, ready)

		// Second signature is accepted
		_, err = execBob.ProcessSignature(t.Batch, tx, tx.Signatures[1])
		require.NoError(t, err)
		fwd := forwardSignature(tx.Transaction, tx.Signatures[1])
		_, err = execAlice.ProcessSignature(t.Batch, fwd, fwd.Signatures[0])
		require.NoError(t, err)

		// Transaction is now ready
		status, err = t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err = execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.True(t, ready)
	})

	t.Run("UpdateKeyPage.Add w/Owner", func(t BatchTest) {
		tx := NewTransaction().
			WithPrincipal(alice.JoinPath("book", "1")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&protocol.UpdateKeyPage{Operation: []protocol.KeyPageOperation{
				&protocol.AddKeyOperation{Entry: protocol.KeySpecParams{Delegate: bob.JoinPath("book")}},
			}}).
			Initiate(protocol.SignatureTypeED25519, aliceKey).
			WithSigner(bob.JoinPath("book", "1"), 1).
			Sign(protocol.SignatureTypeED25519, bobKey).
			BuildDelivery()

		// First signature is accepted
		_, err := execAlice.ProcessSignature(t.Batch, tx, tx.Signatures[0])
		require.NoError(t, err)

		// Transaction is not ready
		status, err := t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err := execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.False(t, ready)

		// Second signature is accepted
		_, err = execBob.ProcessSignature(t.Batch, tx, tx.Signatures[1])
		require.NoError(t, err)
		fwd := forwardSignature(tx.Transaction, tx.Signatures[1])
		_, err = execAlice.ProcessSignature(t.Batch, fwd, fwd.Signatures[0])
		require.NoError(t, err)

		// Transaction is now ready
		status, err = t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err = execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.True(t, ready)
	})

	t.Run("Create Account", func(t BatchTest) {
		tx := NewTransaction().
			WithPrincipal(alice).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&protocol.CreateDataAccount{
				Url:         alice.JoinPath("data"),
				Authorities: []*url.URL{bob.JoinPath("book")},
			}).
			Initiate(protocol.SignatureTypeED25519, aliceKey).
			WithSigner(bob.JoinPath("book", "1"), 1).
			Sign(protocol.SignatureTypeED25519, bobKey).
			BuildDelivery()

		// First signature is accepted
		_, err := execAlice.ProcessSignature(t.Batch, tx, tx.Signatures[0])
		require.NoError(t, err)

		// Transaction is not ready
		status, err := t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err := execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.False(t, ready)

		// Second signature is accepted
		_, err = execBob.ProcessSignature(t.Batch, tx, tx.Signatures[1])
		require.NoError(t, err)
		fwd := forwardSignature(tx.Transaction, tx.Signatures[1])
		_, err = execAlice.ProcessSignature(t.Batch, fwd, fwd.Signatures[0])
		require.NoError(t, err)

		// Transaction is now ready
		status, err = t.Batch.Transaction(tx.Transaction.GetHash()).GetStatus()
		require.NoError(t, err)
		ready, err = execAlice.TransactionIsReady(t.Batch, tx, status, simulator.GetAccount[protocol.Account](sim, tx.Transaction.Header.Principal))
		require.NoError(t, err)
		require.True(t, ready)
	})
}

// TestCannotDisableAuthForAuthTxns verifies that authorization is always enforced
// for transactions that modify authorization, such as UpdateKeyPage and
// UpdateAccountAuth, even when an authority is disabled.
func TestCannotDisableAuthForAuthTxns(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	alice := protocol.AccountUrl("alice")
	lite := AcmeLiteAddressStdPriv(GenerateKey(t.Name(), "Lite"))
	mainKey := GenerateKey(t.Name(), alice)
	unauthKey := GenerateKey(t.Name(), alice, "unauth")
	sim.CreateIdentity(alice, mainKey[32:])
	sim.CreateAccount(&protocol.TokenAccount{Url: alice.JoinPath("tokens"), Balance: *big.NewInt(1e9), TokenUrl: protocol.AcmeUrl()})
	sim.CreateIdentity(alice.JoinPath("unauth"), unauthKey[32:])
	updateAccount(sim, alice.JoinPath("tokens"), func(t *protocol.TokenAccount) { t.Authorities[0].Disabled = true })
	updateAccount(sim, alice.JoinPath("unauth", "book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	// An unauthorized signer must not be allowed to enable auth
	_, err := sim.SubmitAndExecuteBlock(
		NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("unauth", "book", "1"), 1).
			WithBody(&protocol.UpdateAccountAuth{
				Operations: []protocol.AccountAuthOperation{
					&protocol.EnableAccountAuthOperation{
						Authority: alice.JoinPath("book"),
					},
				},
			}).
			WithTimestampVar(&timestamp).
			Initiate(protocol.SignatureTypeLegacyED25519, unauthKey).
			Build(),
	)
	require.EqualError(t, err, "signature 0: acc://alice.acme/unauth/book/1 is not authorized to sign transactions for acc://alice.acme/tokens")

	// An unauthorized signer should be able to send tokens
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("unauth", "book", "1"), 1).
			WithBody(&protocol.SendTokens{
				To: []*protocol.TokenRecipient{{
					Url:    lite,
					Amount: *big.NewInt(68),
				}},
			}).
			WithTimestampVar(&timestamp).
			Initiate(protocol.SignatureTypeLegacyED25519, unauthKey).
			Build(),
	)...)

	// A signature for updating auth is still required from all key books, even
	// disabled ones
	t.Run("Ready", func(tt *testing.T) {
		x := sim.PartitionFor(alice)
		t := NewBatchTest(tt, x.Database)
		defer t.Discard()

		var account *protocol.TokenAccount
		require.NoError(t, t.Account(alice.JoinPath("tokens")).GetStateAs(&account))
		account = t.PutAccountCopy(account).(*protocol.TokenAccount)
		a, _ := account.AddAuthority(protocol.AccountUrl("foo"))
		a.Disabled = true

		// The transaction
		txn := new(protocol.Transaction)
		txn.Header.Principal = account.Url
		txn.Body = new(protocol.UpdateAccountAuth)
		delivery := new(chain.Delivery)
		delivery.Transaction = txn

		// The signature
		sig := new(FakeSignature)
		sig.Signer = alice.JoinPath("book", "1")
		sig.SignerVersion = 1
		sig.Timestamp = timestamp + 1
		sig.PublicKey = []byte{1}
		t.AddSignature(txn.GetHash(), 0, sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := x.Executor.TransactionIsReady(t.Batch, delivery, status, account)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be pending")
	})
}

func forwardSignature(txn *protocol.Transaction, sig protocol.Signature) *chain.Delivery {
	body := &protocol.SyntheticForwardTransaction{
		Transaction: txn,
		Signatures: []protocol.RemoteSignature{{
			Destination: txn.Header.Principal,
			Signature: &protocol.SignatureSet{
				Vote:            protocol.VoteTypeAccept,
				Signer:          sig.GetSigner(),
				TransactionHash: *(*[32]byte)(txn.GetHash()),
				Signatures:      []protocol.Signature{sig},
			},
		}},
	}
	parent := &chain.Delivery{Transaction: &protocol.Transaction{Body: body}}
	return parent.NewForwarded(body)
}

func TestValidateKeyForSynthTxns(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	// Setup
	aliceKey, bobKey := GenerateKey("alice"), GenerateKey("bob")
	alice, bob := AcmeLiteAddressStdPriv(aliceKey), AcmeLiteAddressStdPriv(bobKey)
	sim.SetRouteFor(alice.RootIdentity(), "BVN0")
	sim.SetRouteFor(bob.RootIdentity(), "BVN1")
	sim.CreateAccount(&protocol.LiteIdentity{Url: alice.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&protocol.LiteTokenAccount{Url: alice, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Grab the deposit
	var deposit *chain.Delivery
	sim.Executors["BVN1"].SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if env.Transaction.Body.Type() == protocol.TransactionTypeSyntheticDepositTokens {
				fmt.Printf("Dropping %X\n", env.Transaction.GetHash()[:4])
				deposit = env
				return append(envelopes[:i], envelopes[i+1:]...), false
			}
		}
		return envelopes, true
	}

	// Generate a deposit
	envs := sim.MustSubmitAndExecuteBlock(
		NewTransaction().
			WithPrincipal(alice).
			WithSigner(alice, 1).
			WithTimestampVar(&timestamp).
			WithBody(&protocol.SendTokens{
				To: []*protocol.TokenRecipient{
					{Url: bob, Amount: *big.NewInt(68)},
				},
			}).
			Initiate(protocol.SignatureTypeED25519, aliceKey).
			Build(),
	)
	txnHash := envs[0].Transaction[0].GetHash()
	if txn, _, _ := sim.WaitForTransaction(delivered, txnHash, 50); txn == nil {
		t.Fatal("Transaction has not been delivered after 50 blocks")
	}
	for i := 0; i < 50 && deposit == nil; i++ {
		sim.ExecuteBlock(nil)
	}
	require.NotNil(t, deposit, "Did not intercept deposit within 50 blocks")

	// Replace the signature, using an invalid key to sign it
	index := -1
	for i, sig := range deposit.Signatures {
		sig, ok := sig.(*protocol.ED25519Signature)
		if !ok {
			continue
		}

		index = i
		signer, err := new(signing.Builder).Import(sig)
		require.NoError(t, err)
		signer.SetPrivateKey(GenerateKey("Not a valid key"))
		sig2, err := signer.Sign(sig.TransactionHash[:])
		require.NoError(t, err)
		deposit.Signatures[i] = sig2
	}
	require.True(t, index >= 0, "Failed to replace the key signature")

	x := sim.Executors["BVN1"]
	batch := x.Database.Begin(false)
	defer batch.Discard()
	_, err := x.Executor.ValidateEnvelope(batch, deposit)
	require.EqualError(t, err, fmt.Sprintf("signature %d: key is not an active validator for BVN0", index))
}

// Checks if the key used to sign the synthetic transaction belongs to the same subnet
func TestKeySignaturePartition(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	aliceKey, bobKey := GenerateKey("alice"), GenerateKey("bob")
	alice, bob := AcmeLiteAddressStdPriv(aliceKey), AcmeLiteAddressStdPriv(bobKey)
	sim.SetRouteFor(alice.RootIdentity(), "BVN0")
	sim.SetRouteFor(bob.RootIdentity(), "BVN1")
	sim.CreateAccount(&protocol.LiteIdentity{Url: alice.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&protocol.LiteTokenAccount{Url: alice, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e12)})

	// Grab the deposit
	var deposit *chain.Delivery
	sim.Executors["BVN1"].SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if env.Transaction.Body.Type() == protocol.TransactionTypeSyntheticDepositTokens {
				fmt.Printf("Dropping %X\n", env.Transaction.GetHash()[:4])
				deposit = env
				return append(envelopes[:i], envelopes[i+1:]...), false
			}
		}
		return envelopes, true
	}

	// Generate a deposit
	envs := sim.MustSubmitAndExecuteBlock(NewTransaction().
		WithPrincipal(alice).
		WithSigner(alice, 1).
		WithTimestampVar(&timestamp).
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bob, Amount: *big.NewInt(68)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build())
	if txn, _, _ := sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50); txn == nil {
		t.Fatal("Transaction has not been delivered after 50 blocks")
	}
	for i := 0; i < 50 && deposit == nil; i++ {
		sim.ExecuteBlock(nil)
	}
	require.NotNil(t, deposit, "Did not intercept deposit within 50 blocks")

	// Replace the signature, using another subnet's key to sign it
	index := -1
	for i, sig := range deposit.Signatures {
		sig, ok := sig.(*protocol.ED25519Signature)
		if !ok {
			continue
		}

		index = i
		signer, err := new(signing.Builder).Import(sig)
		require.NoError(t, err)
		signer.SetPrivateKey(sim.Executors["BVN2"].Executor.Key)
		sig2, err := signer.Sign(sig.TransactionHash[:])
		require.NoError(t, err)
		deposit.Signatures[i] = sig2
	}
	require.True(t, index >= 0, "Failed to replace the key signature")

	x := sim.Executors["BVN1"]
	batch := x.Database.Begin(false)
	defer batch.Discard()
	_, err := x.Executor.ValidateEnvelope(batch, deposit)
	require.EqualError(t, err, fmt.Sprintf("signature %d: key is not an active validator for BVN0", index))
}
