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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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
// STAGING IS REGISTERED, and that is the point. Production registers the
// partition's staging in exactly one place — cmd/accumulated/run/dagbft.go —
// and nowhere else in the tree did, so stagingFor() answered nil under test,
// the derived-value path was a no-op, and every simulator test of the pull,
// the join and the receipt contract was exercising a querier that does not
// ship.
func TestServedBodyHashesToItsReceipt(t *testing.T) {
	// Its own partition name: the staging registry is global, so a name
	// shared with another test would hand this querier that test's staging.
	const partitionID = "ServedBody"
	part := protocol.PartitionUrl(partitionID)
	source := protocol.DnUrl()

	anchors := part.JoinPath(protocol.AnchorPool)
	synth := part.JoinPath(protocol.Synthetic)
	sysLedger := part.JoinPath(protocol.Ledger)

	db := database.OpenInMemory(nil)

	// Two sequence ledgers, each with a stream that has delivered something.
	// Both matter: withSighted switched on both, and on an idle network the
	// synthetic ledger's Sequence is empty, so it verified by accident.
	batch := db.Begin(true)
	al := new(protocol.AnchorLedger)
	al.Url = anchors
	al.Partition(source).Delivered = 3
	require.NoError(t, batch.Account(anchors).Main().Put(al))

	sl := new(protocol.SyntheticLedger)
	sl.Url = synth
	sl.Partition(source).Delivered = 5
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

	// Staging sighted well ahead of what either ledger has delivered, so a
	// value derived from it is nowhere near the stored one. 222 is what the
	// live network served for acc://dn.acme on a stored Received of 0.
	staging := execute.NewStaging()
	tx := staging.Begin()
	tx.Hold(execute.StreamID{Ledger: anchors, Source: source}, 222,
		&execute.Held{ID: source.WithTxID([32]byte{1})})
	tx.Hold(execute.StreamID{Ledger: synth, Source: source}, 9,
		&execute.Held{ID: source.WithTxID([32]byte{2})})
	tx.Commit()
	execute.RegisterStaging(partitionID, staging)
	require.Same(t, staging, execute.StagingFor(partitionID),
		"the querier must find this staging the way the node's does")

	q := api.Querier2{Querier: NewQuerier(QuerierParams{Database: db, Partition: partitionID})}
	ctx := context.Background()

	for _, c := range []struct {
		name    string
		url     *url.URL
		sighted uint64
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

			// And the derived number is still answered, beside the body. A
			// fix that simply stopped reporting it would pass the line above
			// and put every operator surface back to reading zero (#4189).
			var got uint64
			var found bool
			for _, s := range rec.Sighted {
				if s.Source.Equal(source) {
					got, found = s.Received, true
				}
			}
			require.True(t, found, "the stream's sighted count was not served beside the body")
			require.Equal(t, c.sighted, got)

			// And a reader that wants it merged gets it, on its own side.
			merged := rec.SightedAccount()
			require.Equal(t, c.sighted, receivedFrom(t, merged, source))
			require.NotEqual(t, c.sighted, receivedFrom(t, rec.Account, source),
				"SightedAccount must copy: the record must be left as it was served")
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
