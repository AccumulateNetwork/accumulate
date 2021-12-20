package abci_test

import (
	"crypto/sha256"
	"testing"
	"time"

	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	lite "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestProofADI(t *testing.T) {
	n := createAppWithMemDB(t, crypto.Address{}, true)

	// Setup keys and the lite account
	liteKey, adiKey := generateKey(), generateKey()
	keyHash := sha256.Sum256(adiKey.PubKey().Bytes())
	dbTx := n.db.Begin()
	require.NoError(n.t, acctesting.CreateLiteTokenAccount(dbTx, liteKey, 5e4))
	dbTx.Commit(n.NextHeight(), time.Unix(0, 0), nil)

	// Create ADI
	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.IdentityCreate)
		adi.Url = "RoadRunner"
		adi.KeyBookName = "book0"
		adi.KeyPageName = "page0"
		adi.PublicKey = keyHash[:]

		sponsorUrl := lite.GenerateAcmeAddress(liteKey.PubKey().Bytes())
		tx, err := transactions.New(sponsorUrl, 1, edSigner(liteKey, 1), adi)
		require.NoError(t, err)

		send(tx)
	})
	require.Equal(t, keyHash[:], n.GetKeyPage("RoadRunner/page0").Keys[0].PublicKey)

	// Create ADI token account
	n.Batch(func(send func(*transactions.GenTransaction)) {
		tac := new(protocol.TokenAccountCreate)
		tac.Url = "RoadRunner/Baz"
		tac.TokenUrl = protocol.AcmeUrl().String()
		tx, err := transactions.New("RoadRunner", 1, edSigner(adiKey, 1), tac)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, types.ChainTypeIdentity, n.GetADI("RoadRunner").Type)
	require.Equal(t, types.ChainTypeTokenAccount, n.GetTokenAccount("RoadRunner/Baz").Type)

	// TODO Verify proofs
}
