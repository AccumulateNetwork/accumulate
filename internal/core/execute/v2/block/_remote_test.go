// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

var delivered = (*TransactionStatus).Delivered
var pending = (*TransactionStatus).Pending

func SetupForRemoteSignatures(sim *simulator.Simulator, timestamp *uint64, alice, bob, charlie ed25519.PrivateKey) {
	aliceTm := tmed25519.PrivKey(alice)
	aliceAcmeUrl := acctesting.AcmeLiteAddressTmPriv(aliceTm)
	aliceUrl := aliceAcmeUrl.RootIdentity()
	bobUrl, charlieUrl := AccountUrl("bob"), AccountUrl("charlie")

	sim.SetRouteFor(aliceUrl, "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Create and fund a lite address
	batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
	defer batch.Discard()
	require.NoError(sim.TB, acctesting.CreateLiteTokenAccountWithCredits(batch, aliceTm, 1e9, 1e9))
	require.NoError(sim.TB, batch.Commit())

	// Create the ADIs
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateIdentity{
				Url:        bobUrl,
				KeyBookUrl: bobUrl.JoinPath("book"),
				KeyHash:    doSha256(bob[32:]),
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateIdentity{
				Url:        charlieUrl,
				KeyBookUrl: charlieUrl.JoinPath("book"),
				KeyHash:    doSha256(charlie[32:]),
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Add credits to the key pages
	envs = sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceAcmeUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: bobUrl.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
		acctesting.NewTransaction().
			WithPrincipal(aliceAcmeUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(timestamp).
			WithBody(&AddCredits{
				Recipient: charlieUrl.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Create the data account
	envs = sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			WithTimestampVar(timestamp).
			WithBody(&CreateDataAccount{
				Url: bobUrl.JoinPath("account"),
			}).
			Initiate(SignatureTypeED25519, bob).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Add the second authority
	envs = sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			WithTimestampVar(timestamp).
			WithBody(&UpdateAccountAuth{Operations: []AccountAuthOperation{
				&AddAccountAuthorityOperation{Authority: charlieUrl.JoinPath("book")},
			}}).
			Initiate(SignatureTypeED25519, bob).
			Build(),
	)
	sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(envs[0].Transaction[0]).
			WithSigner(charlieUrl.JoinPath("book", "1"), 1).
			Sign(SignatureTypeED25519, charlie).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)
}

func TestRemoteSignatures_SignPending(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name())
	aliceAcmeUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceUrl := aliceAcmeUrl.RootIdentity()
	bobUrl, charlieUrl := AccountUrl("bob"), AccountUrl("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl, "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Initiate with the local authority
	env := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&WriteData{
			Entry: &AccumulateDataEntry{Data: [][]byte{
				[]byte("foo"),
			}},
		}).
		WithSigner(bobUrl.JoinPath("book", "1"), 1).
		WithTimestampVar(&timestamp).
		Initiate(SignatureTypeED25519, bobKey).
		Build()

	// Sign with the remote authority
	sig := acctesting.NewTransaction().
		WithTransaction(env.Transaction[0]).
		WithSigner(charlieUrl.JoinPath("book", "1"), 1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, charlieKey).
		Build()

	envs := sim.MustSubmitAndExecuteBlock(env, sig)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	de, _, _, err := indexing.Data(batch, bobUrl.JoinPath("account")).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.GetData()[0]))
}

func TestRemoteSignatures_SameBVN(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name())
	aliceAcmeUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceUrl := aliceAcmeUrl.RootIdentity()
	bobUrl, charlieUrl := AccountUrl("bob"), AccountUrl("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto the same BVN
	sim.SetRouteFor(aliceUrl, "BVN1")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN1")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Initiate with the local authority
	env := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&WriteData{
			Entry: &AccumulateDataEntry{Data: [][]byte{
				[]byte("foo"),
			}},
		}).
		WithSigner(bobUrl.JoinPath("book", "1"), 1).
		WithTimestampVar(&timestamp).
		Initiate(SignatureTypeED25519, bobKey).
		Build()

	// Sign with the remote authority
	sig := acctesting.NewTransaction().
		WithTransaction(env.Transaction[0]).
		WithSigner(charlieUrl.JoinPath("book", "1"), 1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, charlieKey).
		Build()

	envs := sim.MustSubmitAndExecuteBlock(env, sig)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	de, _, _, err := indexing.Data(batch, bobUrl.JoinPath("account")).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.GetData()[0]))
}

func TestRemoteSignatures_Initiate(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name())
	aliceAcmeUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceUrl := aliceAcmeUrl.RootIdentity()
	bobUrl, charlieUrl := AccountUrl("bob"), AccountUrl("charlie")
	bobKey := acctesting.GenerateKey(t.Name(), bobUrl)
	charlieKey1 := acctesting.GenerateKey(t.Name(), charlieUrl, 1)
	charlieKey2 := acctesting.GenerateKey(t.Name(), charlieUrl, 2)
	charlieKey3 := acctesting.GenerateKey(t.Name(), charlieUrl, 3)

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl, "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey1)

	updateAccountNew(sim, charlieUrl.JoinPath("book", "1"), func(p *KeyPage) {
		hash2 := sha256.Sum256(charlieKey2[32:])
		hash3 := sha256.Sum256(charlieKey3[32:])
		p.AddKeySpec(&KeySpec{PublicKeyHash: hash2[:]})
		p.AddKeySpec(&KeySpec{PublicKeyHash: hash3[:]})
		p.AcceptThreshold = 2
	})

	// Initiate with the remote authority
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithTimestampVar(&timestamp).
			WithSigner(charlieUrl.JoinPath("book", "1"), 1).
			Initiate(SignatureTypeED25519, charlieKey1).
			Sign(SignatureTypeED25519, charlieKey2).
			Sign(SignatureTypeED25519, charlieKey3).
			Build(),
	)
	sim.WaitForTransactions(pending, envs...)

	// Sign with the local authority
	envs = sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithTransaction(envs[0].Transaction[0]).
			WithTimestamp(0).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			Sign(SignatureTypeED25519, bobKey).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	de, _, _, err := indexing.Data(batch, bobUrl.JoinPath("account")).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.GetData()[0]))
}

func TestRemoteSignatures_Singlesig(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey(t.Name())
	aliceAcmeUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	aliceUrl := aliceAcmeUrl.RootIdentity()
	bobUrl, charlieUrl := AccountUrl("bob"), AccountUrl("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl, "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Remove the second authority directly
	var account *DataAccount
	batch := sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(bobUrl.JoinPath("account")).GetStateAs(&account))
	require.True(t, account.RemoveAuthority(bobUrl.JoinPath("book")))
	require.Len(t, account.Authorities, 1)
	require.NoError(t, batch.Account(bobUrl.JoinPath("account")).PutState(account))
	require.NoError(t, batch.Commit())

	// Execute with the remote authority
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithBody(&WriteData{
				Entry: &AccumulateDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithTimestampVar(&timestamp).
			WithSigner(charlieUrl.JoinPath("book", "1"), 1).
			Initiate(SignatureTypeED25519, charlieKey).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch = sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	de, _, _, err := indexing.Data(batch, bobUrl.JoinPath("account")).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.GetData()[0]))
}
