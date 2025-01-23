// Copyright 2025 The Accumulate Authors
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

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
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
		MustBuild(sim.TB, build.Transaction().
			For(aliceUrl).
			Body(&CreateIdentity{
				Url:        bobUrl,
				KeyBookUrl: bobUrl.JoinPath("book"),
				KeyHash:    doSha256(bob[32:]),
			}).
			SignWith(aliceUrl).Version(1).Timestamp(timestamp).PrivateKey(alice)),
		MustBuild(sim.TB, build.Transaction().
			For(aliceUrl).
			Body(&CreateIdentity{
				Url:        charlieUrl,
				KeyBookUrl: charlieUrl.JoinPath("book"),
				KeyHash:    doSha256(charlie[32:]),
			}).
			SignWith(aliceUrl).Version(1).Timestamp(timestamp).PrivateKey(alice)),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Add credits to the key pages
	envs = sim.MustSubmitAndExecuteBlock(
		MustBuild(sim.TB, build.Transaction().
			For(aliceAcmeUrl).
			Body(&AddCredits{
				Recipient: bobUrl.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			SignWith(aliceUrl).Version(1).Timestamp(timestamp).PrivateKey(alice)),
		MustBuild(sim.TB, build.Transaction().
			For(aliceAcmeUrl).
			Body(&AddCredits{
				Recipient: charlieUrl.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			SignWith(aliceUrl).Version(1).Timestamp(timestamp).PrivateKey(alice)),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Create the data account
	envs = sim.MustSubmitAndExecuteBlock(
		MustBuild(sim.TB, build.Transaction().
			For(bobUrl).
			Body(&CreateDataAccount{
				Url: bobUrl.JoinPath("account"),
			}).
			SignWith(bobUrl.JoinPath("book", "1")).Version(1).Timestamp(timestamp).PrivateKey(bob)),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Add the second authority
	envs = sim.MustSubmitAndExecuteBlock(
		MustBuild(sim.TB, build.Transaction().
			For(bobUrl.JoinPath("account")).
			Body(&UpdateAccountAuth{Operations: []AccountAuthOperation{
				&AddAccountAuthorityOperation{Authority: charlieUrl.JoinPath("book")},
			}}).
			SignWith(bobUrl.JoinPath("book", "1")).Version(1).Timestamp(timestamp).PrivateKey(bob)),
	)
	sim.MustSubmitAndExecuteBlock(
		MustBuild(sim.TB, build.SignatureForTransaction(envs[0].Transaction[0]).
			Url(charlieUrl.JoinPath("book", "1")).Version(1).PrivateKey(charlie)),
	)
	sim.WaitForTransactions(delivered, envs...)
}

func TestRemoteSignatures_SignPending(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(&network.GlobalValues{ExecutorVersion: ExecutorVersionV1DoubleHashEntries})

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
	env :=
		MustBuild(t, build.Transaction().
			For(bobUrl.JoinPath("account")).
			Body(&WriteData{
				Entry: &DoubleHashDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			SignWith(bobUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// Sign with the remote authority
	sig :=
		MustBuild(t, build.SignatureForTransaction(env.Transaction[0]).
			Url(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(0).PrivateKey(charlieKey))

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
	sim.InitFromGenesisWith(&network.GlobalValues{ExecutorVersion: ExecutorVersionV1DoubleHashEntries})

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
	env :=
		MustBuild(t, build.Transaction().
			For(bobUrl.JoinPath("account")).
			Body(&WriteData{
				Entry: &DoubleHashDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			SignWith(bobUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// Sign with the remote authority
	sig :=
		MustBuild(t, build.SignatureForTransaction(env.Transaction[0]).
			Url(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(0).PrivateKey(charlieKey))

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
	sim.InitFromGenesisWith(&network.GlobalValues{ExecutorVersion: ExecutorVersionV1DoubleHashEntries})

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
		MustBuild(t, build.Transaction().
			For(bobUrl.JoinPath("account")).
			Body(&WriteData{
				Entry: &DoubleHashDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			SignWith(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(charlieKey1).
			SignWith(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(charlieKey2).
			SignWith(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(charlieKey3)),
	)
	sim.WaitForTransactions(pending, envs...)

	// Sign with the local authority
	envs = sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.SignatureForTransaction(envs[0].Transaction[0]).
			Url(bobUrl.JoinPath("book", "1")).Version(1).Timestamp(0).PrivateKey(bobKey)),
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
	sim.InitFromGenesisWith(&network.GlobalValues{ExecutorVersion: ExecutorVersionV1DoubleHashEntries})

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
	require.NoError(t, batch.Account(bobUrl.JoinPath("account")).Main().GetAs(&account))
	require.True(t, account.RemoveAuthority(bobUrl.JoinPath("book")))
	require.Len(t, account.Authorities, 1)
	require.NoError(t, batch.Account(bobUrl.JoinPath("account")).Main().Put(account))
	require.NoError(t, batch.Commit())

	// Execute with the remote authority
	envs := sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(bobUrl.JoinPath("account")).
			Body(&WriteData{
				Entry: &DoubleHashDataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			SignWith(charlieUrl.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(charlieKey)),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch = sim.PartitionFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	de, _, _, err := indexing.Data(batch, bobUrl.JoinPath("account")).GetLatestEntry()
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.GetData()[0]))
}
