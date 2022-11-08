// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestProofADI(t *testing.T) {
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)
	n := nodes[partitions[1]][0]

	const initialCredits = 1e6

	// Setup keys and the lite account
	liteKey, adiKey := generateKey(), generateKey()
	keyHash := sha256.Sum256(adiKey.PubKey().Bytes())
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, liteKey, protocol.AcmeFaucetAmount, initialCredits))
	require.NoError(t, batch.Commit())
	liteId := acctesting.AcmeLiteAddressTmPriv(liteKey).RootIdentity()

	// Create ADI
	n.MustExecuteAndWait(func(send func(*Tx)) {
		adi := new(protocol.CreateIdentity)
		adi.Url = protocol.AccountUrl("RoadRunner")
		var err error
		adi.KeyBookUrl, err = url.Parse(fmt.Sprintf("%s/book0", adi.Url))
		require.NoError(t, err)
		adi.KeyHash = keyHash[:]
		send(newTxn(liteId.String()).
			WithBody(adi).
			Initiate(protocol.SignatureTypeLegacyED25519, liteKey).
			Build())
	})

	liteIdentity := n.GetLiteIdentity(liteId.String())
	require.Less(t, liteIdentity.CreditBalance, uint64(initialCredits*protocol.CreditPrecision))
	require.Equal(t, keyHash[:], n.GetKeyPage("RoadRunner/book0/1").Keys[0].PublicKeyHash)

	batch = n.db.Begin(true)
	require.NoError(t, acctesting.AddCredits(batch, protocol.AccountUrl("RoadRunner", "book0", "1"), initialCredits))
	require.NoError(t, batch.Commit())

	// Create ADI token account
	n.MustExecuteAndWait(func(send func(*protocol.Envelope)) {
		tac := new(protocol.CreateTokenAccount)
		tac.Url = protocol.AccountUrl("RoadRunner", "Baz")
		tac.TokenUrl = protocol.AcmeUrl()
		send(newTxn("RoadRunner").
			WithBody(tac).
			WithSigner(protocol.AccountUrl("RoadRunner", "book0", "1"), 1).
			Initiate(protocol.SignatureTypeLegacyED25519, adiKey).
			Build())
	})

	require.Less(t, n.GetKeyPage("RoadRunner/book0/1").CreditBalance, uint64(initialCredits*protocol.CreditPrecision))
	n.GetADI("RoadRunner")
	n.GetTokenAccount("RoadRunner/Baz")

	// TODO Verify proofs
}
