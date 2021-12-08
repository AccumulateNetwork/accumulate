package chain_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
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
	db := new(state.StateDB)
	require.NoError(t, db.Open("mem", true, true, nil))

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	dbtx := db.Begin()
	require.NoError(t, acctesting.CreateADI(dbtx, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyPage(dbtx, "foo/page0", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(dbtx, "foo/page1", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(dbtx, "foo/page2", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyBook(dbtx, "foo/book", "foo/page0", "foo/page1", "foo/page2"))
	_, err := dbtx.Commit(1, time.Unix(0, 0), nil)
	require.NoError(t, err)

	for _, idx := range []uint64{0, 1, 2} {
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			body := new(protocol.UpdateKeyPage)
			body.Operation = protocol.UpdateKey
			body.Key = testKey.PubKey().Bytes()
			body.NewKey = newKey.PubKey().Bytes()

			tx, err := transactions.NewWith(&transactions.SignatureInfo{
				URL:          "foo/page1",
				KeyPageIndex: idx,
			}, edSigner(testKey, 1), body)
			require.NoError(t, err)

			st, err := NewStateManager(db.Begin(), tx)
			require.NoError(t, err)

			err = UpdateKeyPage{}.DeliverTx(st, tx)
			if idx <= 1 {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, `cannot modify "acc://foo/page1" with a lower priority key page`)
			}

			// Do not store state changes
		})
	}
}
