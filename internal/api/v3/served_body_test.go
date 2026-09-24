// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestServedBodyHashesToItsReceipt is the whole contract of an account query
// that carries a receipt: the body served hashes to the leaf the receipt
// served with it proves. The receipt starts at a simple hash of the account's
// main state (internal/database/observer_prod.go, hashState), so the check is
// sha256(body) == Receipt.Start and nothing else.
//
// It is the check a joining node makes — pull.Verify's third, the one that
// says the peer served a true body and not just a true receipt — and it is
// the one that failed on every anchor ledger on the live twelve-node network
// of 2026-09-18, which left acc-bvn1-val1 at block 76 while its peers passed
// 1,099, refusing "the state served does not hash into the anchored root"
// every two seconds (#4295).
//
// RECEIVED IS STORED (#4412). Every block writes each stream's Received, so
// the body served carries it as stored, and nothing is derived from the
// serving node's staging -- not into the body, and not beside it.
func TestServedBodyHashesToItsReceipt(t *testing.T) {
	const partitionID = "ServedBody"
	part := protocol.PartitionUrl(partitionID)
	source := protocol.DnUrl()

	anchors := part.JoinPath(protocol.AnchorPool)
	synth := part.JoinPath(protocol.Synthetic)
	sysLedger := part.JoinPath(protocol.Ledger)

	db := database.OpenInMemory(nil)

	// Two sequence ledgers, each with a stream that has delivered something
	// and received further. Both matter: the old derivation switched on both,
	// and on an idle network the synthetic ledger's Sequence is empty, so it
	// verified by accident. 222 is what the live network served for
	// acc://dn.acme when Received was derived from staging (#4295).
	batch := db.Begin(true)
	al := new(protocol.AnchorLedger)
	al.Url = anchors
	al.Partition(source).Delivered = 3
	al.Partition(source).Received = 222
	require.NoError(t, batch.Account(anchors).Main().Put(al))

	sl := new(protocol.SyntheticLedger)
	sl.Url = synth
	sl.Partition(source).Delivered = 5
	sl.Partition(source).Received = 9
	require.NoError(t, batch.Account(synth).Main().Put(sl))

	ledger := new(protocol.SystemLedger)
	ledger.Url = sysLedger
	ledger.Index = 76
	require.NoError(t, batch.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// A receipt for an account's state is reported at the block the root
	// index chain ends at, so there has to be one.
	batch = db.Begin(true)
	root, err := batch.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, root.AddEntry(data, false))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	q := api.Querier2{Querier: NewQuerier(QuerierParams{Database: db, Partition: partitionID})}
	ctx := context.Background()

	for _, c := range []struct {
		name     string
		url      *url.URL
		received uint64
	}{
		{"anchor ledger", anchors, 222},
		{"synthetic ledger", synth, 9},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec, err := q.QueryAccount(ctx, c.url, &api.DefaultQuery{
				IncludeReceipt: &api.ReceiptOptions{ForAny: true},
			})
			require.NoError(t, err)
			require.NotNil(t, rec.Receipt, "no receipt was served")

			body, err := rec.Account.MarshalBinary()
			require.NoError(t, err)
			h := sha256.Sum256(body)
			require.Equal(t,
				hex.EncodeToString(rec.Receipt.Receipt.Start),
				hex.EncodeToString(h[:]),
				"the body served does not hash to the leaf the receipt served with it proves")

			// And the number every operator surface reads -- how far the
			// stream has arrived -- is the stored one, in the body. Nothing
			// is served beside it (#4412).
			require.Equal(t, c.received, receivedFrom(t, rec.Account, source))
			require.Empty(t, rec.Sighted, "nothing derived from staging is served beside the body")
		})
	}
}

func receivedFrom(t *testing.T, account protocol.Account, source *url.URL) uint64 {
	t.Helper()
	switch l := account.(type) {
	case *protocol.AnchorLedger:
		return l.Partition(source).Received
	case *protocol.SyntheticLedger:
		return l.Partition(source).Received
	default:
		t.Fatalf("%T is not a sequence ledger", account)
		return 0
	}
}
