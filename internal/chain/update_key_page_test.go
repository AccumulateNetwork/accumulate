package chain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/rand"
)

func init() { acctesting.EnableDebugFeatures() }

var rng = rand.New(rand.NewSource(0))

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rng)
	return tmed25519.PrivKey(key)
}

func TestUpdateKeyPage_Priority(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey, testKey, newKey := generateKey(), generateKey(), generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateKeyBook(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, acctesting.CreateKeyPage(batch, "foo/book", testKey.PubKey().Bytes()))
	require.NoError(t, batch.Commit())

	bookUrl := protocol.AccountUrl("foo", "book")

	for _, idx := range []uint64{0, 1, 2} {
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			op := new(protocol.UpdateKeyOperation)
			kh := sha256.Sum256(testKey.PubKey().Bytes())
			nkh := sha256.Sum256(newKey.PubKey().Bytes())
			op.OldEntry.KeyHash = kh[:]
			op.NewEntry.KeyHash = nkh[:]
			body := new(protocol.UpdateKeyPage)
			body.Operation = append(body.Operation, op)

			env := acctesting.NewTransaction().
				WithPrincipal(protocol.FormatKeyPageUrl(bookUrl, 1)).
				WithSigner(protocol.FormatKeyPageUrl(bookUrl, idx), 1).
				WithTimestamp(1).
				WithBody(body).
				Initiate(protocol.SignatureTypeED25519, testKey).
				Build()

			batch := db.Begin(true)
			defer batch.Discard()

			var signer protocol.Signer
			require.NoError(t, batch.Account(env.Signatures[0].GetSigner()).GetStateAs(&signer))

			_, err := UpdateKeyPage{}.SignerIsAuthorized(nil, batch, env.Transaction[0], signer, true)
			if idx <= 1 {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, fmt.Sprintf(`signer acc://foo.acme/book/%d is lower priority than the principal acc://foo.acme/book/2`, idx+1))
			}

			// Do not store state changes
		})
	}
}
