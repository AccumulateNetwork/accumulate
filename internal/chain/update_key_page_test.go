package chain_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"golang.org/x/exp/rand"
)

var rng = rand.New(rand.NewSource(0))

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rng)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey, nonce uint64) func(hash []byte) (*transactions.ED25519Sig, error) {
	return func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(1, key, hash)
	}
}

func TestUpdateKeyPage_Priority(t *testing.T) {
	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	batch := db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page0", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/page2", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book", "foo/page0", "foo/page1", "foo/page2"))
	require.NoError(t, batch.Commit())

	for _, idx := range []uint64{0, 1, 2} {
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			body := new(protocol.UpdateKeyPage)
			body.Operation = protocol.KeyPageOperationUpdate
			body.Key = testKey.PubKey().Bytes()
			body.NewKey = newKey.PubKey().Bytes()

			u, err := url.Parse("foo/page1")
			require.NoError(t, err)

			tx, err := transactions.NewWith(&transactions.Header{
				Origin:       u,
				KeyPageIndex: idx,
			}, edSigner(testKey, 1), body)
			require.NoError(t, err)

			st, err := NewStateManager(db.Begin(), protocol.BvnUrl(t.Name()), tx)
			require.NoError(t, err)

			_, err = UpdateKeyPage{}.Validate(st, tx)
			if idx <= 1 {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, `cannot modify "acc://foo/page1" with a lower priority key page`)
			}

			// Do not store state changes
		})
	}
}
