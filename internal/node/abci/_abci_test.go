// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestTransactionPriority(t *testing.T) {
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)
	dn := nodes[partitions[0]][0]
	bvn := nodes[partitions[1]][0]
	fooKey := generateKey()
	_ = bvn.db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
		return nil
	})
	type Case struct {
		Envelope       *messaging.Envelope
		ExpectPriority int64
	}
	cause := [32]byte{1}
	bodytosend := new(protocol.SyntheticDepositTokens)
	bodytosend.Amount = *big.NewInt(100)
	bodytosend.Token = protocol.AcmeUrl()
	bodytosend.SetCause(cause, bvn.network.NodeUrl())

	const maxPriority = (1 << 32) - 1
	cases := map[string]Case{
		"System": {
			Envelope: newTxn(bvn.network.AnchorPool().String()).
				WithSigner(bvn.network.OperatorsPage(), 1).
				WithBody(&protocol.DirectoryAnchor{
					PartitionAnchor: protocol.PartitionAnchor{
						Source:          dn.network.NodeUrl(),
						RootChainIndex:  1,
						MinorBlockIndex: 1,
					},
				}).
				InitiateSynthetic(bvn.network.NodeUrl()).
				Sign(protocol.SignatureTypeLegacyED25519, dn.key.Bytes()).
				Build(),
			ExpectPriority: maxPriority,
		},
		"Synthetic 1": {
			Envelope: newTxn("foo/tokens").
				WithSigner(bvn.network.OperatorsPage(), 1).
				WithBody(bodytosend).InitiateSynthetic(bvn.network.NodeUrl()).
				Sign(protocol.SignatureTypeLegacyED25519, bvn.exec.Key).
				SignFunc(func(txn *protocol.Transaction) protocol.Signature {
					// Add a receipt signature
					sig := new(protocol.ReceiptSignature)
					sig.SourceNetwork = url.MustParse("foo.acme")
					sig.TransactionHash = *(*[32]byte)(txn.GetHash())
					sig.Proof.Start = txn.GetHash()
					sig.Proof.Anchor = txn.GetHash()
					return sig
				}).
				Build(),
			ExpectPriority: maxPriority - 2,
		},
		"Synthetic 2": {
			Envelope: newTxn("foo/tokens").
				WithSigner(bvn.network.OperatorsPage(), 2).
				WithBody(bodytosend).InitiateSynthetic(bvn.network.NodeUrl()).
				Sign(protocol.SignatureTypeLegacyED25519, bvn.exec.Key).
				SignFunc(func(txn *protocol.Transaction) protocol.Signature {
					// Add a receipt signature
					sig := new(protocol.ReceiptSignature)
					sig.SourceNetwork = url.MustParse("foo.acme")
					sig.TransactionHash = *(*[32]byte)(txn.GetHash())
					sig.Proof.Start = txn.GetHash()
					sig.Proof.Anchor = txn.GetHash()
					return sig
				}).
				Build(),
			ExpectPriority: maxPriority - 3,
		},
		"User": {
			Envelope: newTxn("foo.acme/tokens").
				WithSigner(url.MustParse("foo.acme/book0/1"), 1).
				WithBody(&protocol.BurnTokens{
					Amount: *big.NewInt(100),
				}).
				Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
				Build(),
			ExpectPriority: 0,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Submit the envelope
			b, err := c.Envelope.MarshalBinary()
			require.NoError(t, err)
			resp := bvn.app.CheckTx(abci.RequestCheckTx{Tx: b, Type: abci.CheckTxType_Recheck})
			assert.Zero(t, resp.Code, resp.Log)

			// Check the results
			results := new(protocol.TransactionResultSet)
			require.NoError(t, results.UnmarshalBinary(resp.Data))
			for _, status := range results.Results {
				if status.Error != nil {
					assert.NoError(t, status.Error)
				}
			}
			if t.Failed() {
				return
			}

			// Verify the priority
			require.Equal(t, c.ExpectPriority, resp.Priority)
		})
	}
}

func TestCheckTx_SharedBatch(t *testing.T) {
	t.Skip("https://accumulate.atlassian.net/browse/AC-1702")

	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)
	n := nodes[partitions[1]][0]

	alice, bob := generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	_ = n.db.Update(func(batch *database.Batch) error {
		require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, float64(protocol.FeeTransferTokens)/protocol.CreditPrecision))
		return nil
	})

	// Check a transaction
	resp := n.CheckTx(newTxn(aliceUrl.String()).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&protocol.SendTokens{To: []*protocol.TokenRecipient{{
			Url:    bobUrl,
			Amount: *big.NewInt(1),
		}}}).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build())

	// The first transaction should succeed
	require.Zero(t, resp.Code)

	// Check another transaction
	resp = n.CheckTx(newTxn(aliceUrl.String()).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&protocol.SendTokens{To: []*protocol.TokenRecipient{{
			Url:    bobUrl,
			Amount: *big.NewInt(1),
		}}}).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build())

	// The second transaction should fail due to insufficient credits
	require.NotZero(t, resp.Code)
}

func TestInvalidDeposit(t *testing.T) {
	// The lite address ends with `foo/tokens` but the token is `foo2/tokens` so
	// the synthetic transaction will fail. This test verifies that the
	// transaction fails, but more importantly it verifies that
	// `Executor.Commit()` does *not* break if DeliverTx fails with a
	// non-existent origin. This is motivated by a bug that has been fixed. This
	// bug could have been triggered by a failing SyntheticCreateChains,
	// SyntheticDepositTokens, or SyntheticDepositCredits.

	t.Skip("TODO Fix - generate a receipt")

	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)
	n := nodes[partitions[1]][0]

	liteKey := generateKey()
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], "foo/tokens", protocol.SignatureTypeED25519)
	require.NoError(t, err)

	id := n.MustExecuteAndWait(func(send func(*messaging.Envelope)) {
		body := new(protocol.SyntheticDepositTokens)
		body.Cause = n.network.NodeUrl().WithTxID([32]byte{1})
		body.Token = protocol.AccountUrl("foo2", "tokens")
		body.Amount.SetUint64(123)

		send(newTxn(liteAddr.String()).
			WithBody(body).
			InitiateSynthetic(n.network.NodeUrl()).
			Sign(protocol.SignatureTypeLegacyED25519, n.key.Bytes()).
			Build())
	})[0]

	tx := n.GetTx(id[:])
	require.NotZero(t, tx.Status.Code)
}

func TestEvilNode(t *testing.T) {

	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	//tell the TestNet that we have an evil node in the midst
	dns := partitions[0]
	bvn := partitions[1]
	partitions[0] = "evil-" + partitions[0]
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)

	dn := nodes[dns][0]
	n := nodes[bvn][0]

	var count = 11
	credits := 100.0
	originAddr, balances := testLiteTx(n, count, 1, credits)
	require.Equal(t, int64(protocol.AcmeFaucetAmount*protocol.AcmePrecision-count*1000), n.GetLiteTokenAccount(originAddr).Balance.Int64())
	for addr, bal := range balances {
		require.Equal(t, bal, n.GetLiteTokenAccount(addr.String()).Balance.Int64())
	}

	batch := dn.db.Begin(true)
	defer batch.Discard()
	// Check each anchor
	de, txId, _, err := indexing.Data(batch, dn.network.NodeUrl(protocol.Evidence)).GetLatestEntry()
	require.NoError(t, err)
	var ev []types2.Misbehavior
	require.NotEqual(t, de.GetData(), nil, "no data")
	err = json.Unmarshal(de.GetData()[0], &ev)
	require.NoError(t, err)
	require.Greaterf(t, len(ev), 0, "no evidence data")
	require.Greater(t, ev[0].Height, int64(0), "no valid evidence available")
	require.NotNilf(t, txId, "txId not returned")
}
