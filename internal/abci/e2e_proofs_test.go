package abci_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestProofADI(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	nodes := RunTestNet(t, subnets, daemons, nil, true)
	n := nodes[subnets[1]][0]

	const initialCredits = 1e6

	// Setup keys and the lite account
	liteKey, adiKey := generateKey(), generateKey()
	keyHash := sha256.Sum256(adiKey.PubKey().Bytes())
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, acctesting.TestTokenAmount, initialCredits))
	require.NoError(t, batch.Commit())
	liteAddr := acctesting.AcmeLiteAddressTmPriv(liteKey).String()

	// Create ADI
	n.Batch(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = n.ParseUrl("RoadRunner")
		var err error
		adi.KeyBookUrl, err = url.Parse(fmt.Sprintf("%s/book0", adi.Url))
		require.NoError(t, err)
		adi.PublicKey = keyHash[:]
		send(newTxn(liteAddr).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey))
	})

	require.Less(t, n.GetLiteTokenAccount(liteAddr).CreditBalance, uint64(initialCredits*protocol.CreditPrecision))
	require.Equal(t, keyHash[:], n.GetKeyPage("RoadRunner/book0/1").Keys[0].PublicKey)

	batch = n.db.Begin(true)
	require.NoError(t, acctesting.AddCredits(batch, n.ParseUrl("RoadRunner/book0/1"), initialCredits))
	require.NoError(t, batch.Commit())

	// Create ADI token account
	n.Batch(func(send func(*protocol.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = n.ParseUrl("RoadRunner/Baz")
		tac.TokenUrl = protocol.AcmeUrl()
		send(newTxn("RoadRunner").
			WithBody(tac).
			WithSigner(url.MustParse("RoadRunner/book0/1"), 1).
			Initiate(protocol.SignatureTypeLegacyED25519, adiKey))
	})

	require.Less(t, n.GetKeyPage("RoadRunner/book0/1").CreditBalance, int64(initialCredits*protocol.CreditPrecision))
	n.GetADI("RoadRunner")
	n.GetTokenAccount("RoadRunner/Baz")

	// TODO Verify proofs
}
