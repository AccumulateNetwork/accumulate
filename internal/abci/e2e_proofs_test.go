package abci_test

import (
	"crypto/sha256"
	"testing"

	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
)

func TestProofADI(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	// Setup keys and the lite account
	liteKey, adiKey := generateKey(), generateKey()
	keyHash := sha256.Sum256(adiKey.PubKey().Bytes())
	batch := n.db.Begin()
	require.NoError(t, acctesting.CreateLiteTokenAccount(batch, liteKey, 5e4))
	require.NoError(t, batch.Commit())

	// Create ADI
	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = "RoadRunner"
		adi.KeyBookName = "book0"
		adi.KeyPageName = "page0"
		adi.PublicKey = keyHash[:]

		sponsorUrl := acctesting.AcmeLiteAddressTmPriv(liteKey).String()
		tx, err := transactions.New(sponsorUrl, 1, edSigner(liteKey, 1), adi)
		require.NoError(t, err)

		send(tx)
	})
	require.Equal(t, keyHash[:], n.GetKeyPage("RoadRunner/page0").Keys[0].PublicKey)

	// Create ADI token account
	n.Batch(func(send func(*transactions.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = "RoadRunner/Baz"
		tac.TokenUrl = protocol.AcmeUrl().String()
		tx, err := transactions.New("RoadRunner", 1, edSigner(adiKey, 1), tac)
		require.NoError(t, err)
		send(tx)
	})

	require.Equal(t, types.AccountTypeIdentity, n.GetADI("RoadRunner").Type)
	require.Equal(t, types.AccountTypeTokenAccount, n.GetTokenAccount("RoadRunner/Baz").Type)

	// TODO Verify proofs
}
