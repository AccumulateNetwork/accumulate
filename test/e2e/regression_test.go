// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestOverwriteCreditBalance(t *testing.T) {
	const x = 0.05
	const y = 10000
	big.NewInt(x * y)

	// Tests AC-1555
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
	const oracle = InitialAcmeOracleValue
	acme := big.NewInt(AcmePrecision)
	acme.Mul(acme, big.NewInt(additionalBalance))
	acme.Div(acme, big.NewInt(CreditsPerDollar))
	acme.Mul(acme, big.NewInt(AcmeOraclePrecision))
	acme.Div(acme, big.NewInt(oracle))
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
	req := new(api.KeyPageIndexQuery)
	req.Url = alice.JoinPath("managed-tokens")
	req.Key = aliceKey[32:]
	_, err := sim.PartitionFor(req.Url).API.QueryKeyPageIndex(context.Background(), req)
	require.Error(t, err)
	require.IsType(t, jsonrpc2.Error{}, err)
	require.Equal(t, err.(jsonrpc2.Error).Data, fmt.Sprintf("no authority of %s holds %X", req.Url, req.Key))
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
	const oracle = InitialAcmeOracleValue
	acme := big.NewInt(AcmePrecision)
	acme.Mul(acme, big.NewInt(creditAmount))
	acme.Div(acme, big.NewInt(CreditsPerDollar))
	acme.Mul(acme, big.NewInt(AcmeOraclePrecision))
	acme.Div(acme, big.NewInt(oracle))
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

func TestSynthTxnWithMissingPrincipal(t *testing.T) {
	// Tests AC-1704
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup a lite token account for a token type that does not exist
	liteKey := acctesting.GenerateKey("Lite")
	lite := sim.CreateLiteTokenAccount(liteKey, url.MustParse("fake.acme/tokens"), 1e9, 1)

	// Burn credits
	txn := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithTimestampVar(&timestamp).
			WithBody(&BurnTokens{
				Amount: *big.NewInt(1),
			}).
			Initiate(SignatureTypeED25519, liteKey).
			Build(),
	)
	_, _, synth := sim.WaitForTransaction(delivered, txn[0].Transaction[0].GetHash(), 50)

	// The synthetic transaction must fail
	require.Len(t, synth, 1)
	hash := synth[0].Hash()
	_, status, _ := sim.WaitForTransaction(delivered, hash[:], 50)
	require.NotZero(t, status.Code, "The transaction did not fail")
}

func TestFaucetMultiNetwork(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	liteKey := acctesting.GenerateKey("Lite")
	lite := sim.CreateLiteTokenAccount(liteKey, AcmeUrl(), 1e9, 1e12)

	// Set the lite account routing to a different BVN from the faucet
	faucetBvn := sim.PartitionFor(FaucetUrl)
	for _, partition := range sim.Partitions[1:] {
		if faucetBvn.Partition.Id != partition.Id {
			sim.SetRouteFor(lite.RootIdentity(), partition.Id)
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

func TestSigningDeliveredTxnDoesNothing(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup
	aliceKey, bobKey := acctesting.GenerateKey("Alice"), acctesting.GenerateKey("Bob")
	alice := sim.CreateLiteTokenAccount(aliceKey, AcmeUrl(), 1e9, 2)
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

	// Execute
	_, txns := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithTimestampVar(&timestamp).
			WithSigner(alice, 1).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bob,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Verify
	require.Equal(t, 1, int(simulator.GetAccount[*LiteTokenAccount](sim, alice).Balance.Int64()))
	require.Equal(t, 1, int(simulator.GetAccount[*LiteTokenAccount](sim, bob).Balance.Int64()))

	// Send another signature
	_, err := sim.SubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithSigner(alice, 1).
			WithTxnHash(txns[0].GetHash()).
			Sign(SignatureTypeED25519, aliceKey).
			Build(),
	)

	// It should fail
	require.Error(t, err)

	// Verify no double-spend
	require.Equal(t, 1, int(simulator.GetAccount[*LiteTokenAccount](sim, alice).Balance.Int64()))
	require.Equal(t, 1, int(simulator.GetAccount[*LiteTokenAccount](sim, bob).Balance.Int64()))
}

func TestSynthTxnToDirectory(t *testing.T) {
	// Tests AC-2231
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name(), "alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey(t.Name(), "bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)

	// Put Alice on BVN0 and Bob on the DN
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl.RootIdentity(), "Directory")

	// Create Alice
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	// Send tokens from BVN to DN
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&SendTokens{
			To: []*TokenRecipient{{
				Url:    bobUrl,
				Amount: *big.NewInt(1e6),
			}},
		}).
		Initiate(SignatureTypeED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())
}

func TestSynthTxnFromDirectory(t *testing.T) {
	// Tests AC-2231
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name(), "alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey(t.Name(), "bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)

	// Put Alice on the DN and Bob on BVN0
	sim.SetRouteFor(aliceUrl.RootIdentity(), "Directory")
	sim.SetRouteFor(bobUrl.RootIdentity(), "BVN0")

	// Create Alice
	sim.CreateAccount(&LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&LiteTokenAccount{Url: aliceUrl, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e9)})

	// Send tokens from BVN to DN
	env := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithTimestampVar(&timestamp).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&SendTokens{
			To: []*TokenRecipient{{
				Url:    bobUrl,
				Amount: *big.NewInt(1e6),
			}},
		}).
		Initiate(SignatureTypeED25519, alice).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())
}

func TestSendDirectToWrongPartition(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Create the lite addresses and one account
	aliceKey, bobKey := acctesting.GenerateKey("alice"), acctesting.GenerateKey("bob")
	alice, bob := acctesting.AcmeLiteAddressStdPriv(aliceKey), acctesting.AcmeLiteAddressStdPriv(bobKey)

	goodBvn := sim.PartitionFor(alice)
	_ = goodBvn.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(aliceKey), 1e6, 1e9))
		return nil
	})

	// Set route to something else
	var badBvn *simulator.ExecEntry
	for _, partition := range sim.Partitions[1:] {
		if partition.Id != goodBvn.Partition.Id {
			badBvn = sim.Partition(partition.Id)
			break
		}
	}

	// Create the transaction
	env := acctesting.NewTransaction().
		WithPrincipal(alice).
		WithSigner(alice, 1).
		WithTimestamp(1).
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    bob,
				Amount: *big.NewInt(1),
			}},
		}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	// Submit the transaction directly to the wrong BVN
	badBvn.Submit(false, env)
	var found bool
	for i := 0; i < 50 && !found; i++ {
		ch := make(chan *protocol.TransactionStatus)
		go sim.ExecuteBlock(ch)
		for s := range ch {
			if s.TxID.Equal(env.Transaction[0].ID()) {
				found = true
			}
		}
	}

	helpers.View(t, badBvn, func(batch *database.Batch) {
		status, err := batch.Transaction(env.Signatures[0].Hash()).Status().Get()
		require.NoError(t, err)
		require.NotZero(t, status.Code)
		require.NotNil(t, status.Error)
		require.Equal(t, fmt.Sprintf("signature submitted to %s instead of %s", badBvn.Partition.Id, goodBvn.Partition.Id), status.Error.Message)
	})
}

func TestDelegateBetweenPartitions(t *testing.T) {
	// Tests AC-3069
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.SetRouteFor(alice, "BVN1")
	sim.SetRouteFor(bob, "BVN2")
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Submit with Alice
	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithTimestampVar(&timestamp).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
			&AddKeyOperation{
				Entry: KeySpecParams{
					Delegate: bob.JoinPath("book"),
				},
			},
		}}).
		Initiate(SignatureTypeED25519, aliceKey).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(pending, env.Transaction[0].GetHash())

	// Sign with Bob
	env = acctesting.NewTransaction().
		WithTransaction(env.Transaction[0]).
		WithSigner(bob.JoinPath("book", "1"), 1).
		Sign(SignatureTypeED25519, bobKey).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	page := simulator.GetAccount[*KeyPage](sim, alice.JoinPath("book", "1"))
	_, _, ok := page.EntryByDelegate(bob.JoinPath("book"))
	require.True(t, ok, "Expected Bob to be a delegate of Alice")
}

func TestAuthorityBetweenPartitions(t *testing.T) {
	// Tests AC-3069
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.SetRouteFor(alice, "BVN1")
	sim.SetRouteFor(bob, "BVN2")
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Submit with Alice
	env := acctesting.NewTransaction().
		WithPrincipal(alice).
		WithTimestampVar(&timestamp).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithBody(&UpdateAccountAuth{Operations: []AccountAuthOperation{
			&AddAccountAuthorityOperation{
				Authority: bob.JoinPath("book"),
			},
		}}).
		Initiate(SignatureTypeED25519, aliceKey).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(pending, env.Transaction[0].GetHash())

	// Sign with Bob
	env = acctesting.NewTransaction().
		WithTransaction(env.Transaction[0]).
		WithSigner(bob.JoinPath("book", "1"), 1).
		Sign(SignatureTypeED25519, bobKey).
		Build()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	adi := simulator.GetAccount[*ADI](sim, alice)
	_, ok := adi.GetAuthority(bob.JoinPath("book"))
	require.True(t, ok, "Expected Bob to be an authority of Alice")
}

func TestPendingTransactionForMissingAccount(t *testing.T) {
	// Tests AC-3174
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.SetRouteFor(alice, "BVN1")
	sim.SetRouteFor(bob, "BVN2")
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	// Create charlie (but don't sign with bob)
	env := acctesting.NewTransaction().
		WithPrincipal(charlie).
		WithTimestampVar(&timestamp).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithBody(&CreateIdentity{Url: charlie, Authorities: []*url.URL{bob.JoinPath("book")}}).
		Initiate(SignatureTypeED25519, aliceKey).
		Build()
	sim.MustSubmitAndExecuteBlock(env)

	// Should be pending because bob hasn't signed
	sim.WaitForTransactionFlow(pending, env.Transaction[0].GetHash())

	// Create a snapshot
	buf := new(ioutil2.Buffer)
	helpers.View(t, sim.PartitionFor(charlie), func(batch *database.Batch) {
		_, err := snapshot.Collect(batch, new(snapshot.Header), buf, nil, func(account *database.Account) (bool, error) { return false, nil })
		require.NoError(t, err)
	})

	// Restore the snapshot
	db2 := database.OpenInMemory(nil)
	helpers.Update(t, db2, func(batch *database.Batch) {
		require.NoError(t, snapshot.Restore(batch, buf, nil))
	})
}

func TestDnAnchorAcknowledged(t *testing.T) {
	t.Skip("This functionality has been reverted")

	aliceKey, bobKey := acctesting.GenerateKey("Alice"), acctesting.GenerateKey("Bob")
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	alice := sim.CreateLiteTokenAccount(aliceKey, AcmeUrl(), 1e9, 2)

	// Create some history
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithTimestampVar(&timestamp).
			WithSigner(alice, 1).
			WithBody(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bob,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Wait a few blocks
	sim.ExecuteBlocks(10)

	// Verify that Acknowledged equals Produced
	x := sim.Partition(Directory)
	helpers.View(t, x, func(batch *database.Batch) {
		var ledger1 *AnchorLedger
		require.NoError(t, batch.Account(x.Executor.Describe.AnchorPool()).Main().GetAs(&ledger1))
		ledger2 := ledger1.Anchor(DnUrl())
		require.Greater(t, ledger2.Produced, uint64(1))
		// require.Equal(t, ledger2.Produced, ledger2.Acknowledged)
	})
}
