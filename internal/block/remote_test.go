package block_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func delivered(status *protocol.TransactionStatus) bool {
	return status.Delivered
}

func pending(status *protocol.TransactionStatus) bool {
	return status.Pending
}

func SetupForRemoteSignatures(sim *simulator.Simulator, timestamp *uint64, alice, bob, charlie ed25519.PrivateKey) {
	aliceTm := tmed25519.PrivKey(alice)
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(aliceTm)
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")

	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Create and fund a lite address
	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	defer batch.Discard()
	require.NoError(sim, acctesting.CreateLiteTokenAccountWithCredits(batch, aliceTm, 1e9, 1e9))
	require.NoError(sim, batch.Commit())

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
			WithPrincipal(aliceUrl).
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
			WithPrincipal(aliceUrl).
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
	sim.WaitForTransactions(delivered, envs...)
}

func TestRemoteSignatures_SignPending(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Initiate with the local authority
	env := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&WriteData{
			Entry: DataEntry{Data: [][]byte{
				[]byte("foo"),
			}},
		}).
		WithSigner(bobUrl.JoinPath("book", "1"), 1).
		WithTimestampVar(&timestamp).
		Initiate(SignatureTypeED25519, bobKey).
		Build()

	// Sign with the remote authority
	sig := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&RemoteTransaction{}).
		WithTxnHash(env.Transaction[0].GetHash()).
		WithSigner(charlieUrl.JoinPath("book", "1"), 1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, charlieKey).
		Build()

	envs := sim.MustSubmitAndExecuteBlock(env, sig)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.SubnetFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	data, err := batch.Account(bobUrl.JoinPath("account")).Data()
	require.NoError(t, err)
	de, err := data.Entry(data.Height() - 1)
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.Data[0]))
}

func TestRemoteSignatures_SameBVN(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto the same BVN
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN1")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN1")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Initiate with the local authority
	env := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&WriteData{
			Entry: DataEntry{Data: [][]byte{
				[]byte("foo"),
			}},
		}).
		WithSigner(bobUrl.JoinPath("book", "1"), 1).
		WithTimestampVar(&timestamp).
		Initiate(SignatureTypeED25519, bobKey).
		Build()

	// Sign with the remote authority
	sig := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&RemoteTransaction{}).
		WithTxnHash(env.Transaction[0].GetHash()).
		WithSigner(charlieUrl.JoinPath("book", "1"), 1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, charlieKey).
		Build()

	envs := sim.MustSubmitAndExecuteBlock(env, sig)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.SubnetFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	data, err := batch.Account(bobUrl.JoinPath("account")).Data()
	require.NoError(t, err)
	de, err := data.Entry(data.Height() - 1)
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.Data[0]))
}

func TestRemoteSignatures_Initiate(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Initiate with the remote authority
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithBody(&WriteData{
				Entry: DataEntry{Data: [][]byte{
					[]byte("foo"),
				}},
			}).
			WithTimestampVar(&timestamp).
			WithSigner(charlieUrl.JoinPath("book", "1"), 1).
			Initiate(SignatureTypeED25519, charlieKey).
			Build(),
	)
	sim.WaitForTransactions(pending, envs...)

	// Sign with the local authority
	envs = sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithBody(&RemoteTransaction{}).
			WithTxnHash(envs[0].Transaction[0].GetHash()).
			WithTimestamp(0).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			Sign(SignatureTypeED25519, bobKey).
			Build(),
	)
	sim.WaitForTransactions(delivered, envs...)

	// Validate
	batch := sim.SubnetFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	data, err := batch.Account(bobUrl.JoinPath("account")).Data()
	require.NoError(t, err)
	de, err := data.Entry(data.Height() - 1)
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.Data[0]))
}

func TestRemoteSignatures_Singlesig(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(tmed25519.PrivKey(alice))
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	// Force the accounts onto different BVNs
	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Setup
	SetupForRemoteSignatures(sim, &timestamp, alice, bobKey, charlieKey)

	// Remove the second authority directly
	var account *protocol.DataAccount
	batch := sim.SubnetFor(bobUrl).Database.Begin(true)
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
				Entry: DataEntry{Data: [][]byte{
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
	batch = sim.SubnetFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	data, err := batch.Account(bobUrl.JoinPath("account")).Data()
	require.NoError(t, err)
	de, err := data.Entry(data.Height() - 1)
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.Data[0]))
}
