package block_test

import (
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func TestRemoteSignatures(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	alice := acctesting.GenerateTmKey(t.Name())
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl, charlieUrl := url.MustParse("bob"), url.MustParse("charlie")
	bobKey, charlieKey := acctesting.GenerateKey(), acctesting.GenerateKey()

	sim.SetRouteFor(aliceUrl.RootIdentity(), "BVN0")
	sim.SetRouteFor(bobUrl, "BVN1")
	sim.SetRouteFor(charlieUrl, "BVN2")

	// Create and fund a lite address
	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
	defer batch.Discard()
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, 1e9, 1e9))
	require.NoError(t, batch.Commit())

	// Create the ADIs
	envs := sim.MustExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(&timestamp).
			WithBody(&CreateIdentity{
				Url:        bobUrl,
				KeyBookUrl: bobUrl.JoinPath("book"),
				KeyHash:    doSha256(bobKey[32:]),
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(&timestamp).
			WithBody(&CreateIdentity{
				Url:        charlieUrl,
				KeyBookUrl: charlieUrl.JoinPath("book"),
				KeyHash:    doSha256(charlieKey[32:]),
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
	)
	sim.WaitForTransactions(envs...)

	// Add credits to the key pages
	envs = sim.MustExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithSigner(aliceUrl, 1).
			WithTimestampVar(&timestamp).
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
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: charlieUrl.JoinPath("book", "1"),
				Amount:    *big.NewInt(AcmePrecision * 1e3),
				Oracle:    InitialAcmeOracleValue,
			}).
			Initiate(SignatureTypeED25519, alice).
			Build(),
	)
	sim.WaitForTransactions(envs...)

	// Create the data account
	envs = sim.MustExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&CreateDataAccount{
				Url: bobUrl.JoinPath("account"),
			}).
			Initiate(SignatureTypeED25519, bobKey).
			Build(),
	)
	sim.WaitForTransactions(envs...)

	// Add the second authority
	envs = sim.MustExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(bobUrl.JoinPath("account")).
			WithSigner(bobUrl.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&UpdateAccountAuth{Operations: []AccountAuthOperation{
				&AddAccountAuthorityOperation{Authority: charlieUrl.JoinPath("book")},
			}}).
			Initiate(SignatureTypeED25519, bobKey).
			Build(),
	)
	sim.WaitForTransactions(envs...)

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
	sig := acctesting.NewTransaction().
		WithPrincipal(bobUrl.JoinPath("account")).
		WithBody(&RemoteTransactionBody{}).
		WithTxnHash(env.GetTxHash()).
		WithSigner(charlieUrl.JoinPath("book", "1"), 1).
		WithTimestamp(0).
		Sign(SignatureTypeED25519, charlieKey).
		Build()
	envs = sim.MustExecuteBlock(env, sig)
	sim.WaitForTransactions(envs...)

	batch = sim.SubnetFor(bobUrl).Database.Begin(true)
	defer batch.Discard()
	data, err := batch.Account(bobUrl.JoinPath("account")).Data()
	require.NoError(t, err)
	de, err := data.Entry(data.Height() - 1)
	require.NoError(t, err)
	require.Equal(t, "foo", string(de.Data[0]))
}
