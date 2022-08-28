package simulator_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func hash(b ...[]byte) []byte {
	h := sha256.New()
	for _, b := range b {
		_, _ = h.Write(b)
	}
	return h.Sum(nil)
}

func TestSimulator(t *testing.T) {
	var timestamp uint64
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")

	sim, err := simulator.New(
		acctesting.NewTestLogger(t),
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.MemoryDatabase,
	)
	require.NoError(t, err)
	require.NoError(t, sim.InitFromGenesis())

	require.NoError(t, sim.Update(alice, func(batch *database.Batch) error {
		identity := new(ADI)
		identity.Url = alice
		identity.AddAuthority(alice.JoinPath("book"))

		book := new(KeyBook)
		book.Url = alice.JoinPath("book")
		book.AddAuthority(alice.JoinPath("book"))
		book.PageCount = 1

		page := new(KeyPage)
		page.Url = FormatKeyPageUrl(alice.JoinPath("book"), 0)
		page.AcceptThreshold = 1
		page.Version = 1

		key := new(KeySpec)
		key.PublicKeyHash = hash(aliceKey[32:])
		page.AddKeySpec(key)
		page.CreditBalance = 1e9

		for _, a := range []protocol.Account{identity, book, page} {
			require.NoError(t, batch.Account(a.GetUrl()).Main().Put(a))
		}
		return nil
	}))

	st, err := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book", "1")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			UseSimpleHash(). // Test AC-2953
			WithBody(&UpdateKey{NewKeyHash: hash(otherKey[32:])}).
			Initiate(SignatureTypeED25519, aliceKey).
			BuildDelivery())
	require.NoError(t, err)
	if st.Error != nil {
		require.NoError(t, st.Error)
	}

	require.NoError(t, sim.Step())
	require.NoError(t, sim.Step())

	require.NoError(t, sim.View(alice, func(batch *database.Batch) error {
		h := st.TxID.Hash()
		_, err := batch.Transaction(h[:]).Main().Get()
		require.NoError(t, err)
		var page *protocol.KeyPage
		require.NoError(t, batch.Account(alice.JoinPath("book", "1")).Main().GetAs(&page))
		require.Len(t, page.Keys, 2)
		return nil
	}))
}
