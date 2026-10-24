// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proofs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4563: an entry that cannot be proven is parked with its error and the run
// goes on; it neither halts the builder nor rewinds its progress.
func TestRunParksAnEntryItCannotProve(t *testing.T) {
	acct := url.MustParse("alice.acme/data") // has no chains at all
	db := database.OpenInMemory(nil)
	ledgerUrl := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		ledger := batch.Account(ledgerUrl)
		if err := ledger.Main().Put(&protocol.SystemLedger{Url: ledgerUrl, Index: 2}); err != nil {
			return err
		}
		if err := ledger.BlockLedger(1).Put(&database.BlockLedger{Index: 1, Entries: []*protocol.BlockEntry{
			{Account: acct, Chain: "main", Index: 0},
		}}); err != nil {
			return err
		}
		return ledger.BlockLedger(2).Put(&database.BlockLedger{Index: 2})
	}))

	store := memory.New(nil)
	b := &Builder{Network: "n", Partition: "BVN0", Database: db, Accounts: []*url.URL{acct}, Store: store}

	keys := func() (progress *uint64, parked []pending) {
		st := store.Begin(nil, false)
		defer st.Discard()
		require.NoError(t, st.ForEach(func(k *record.Key, v []byte) error {
			switch k.Get(0) {
			case "Progress":
				progress = new(uint64)
				require.NoError(t, json.Unmarshal(v, progress))
			case "Pending":
				var p pending
				require.NoError(t, json.Unmarshal(v, &p))
				parked = append(parked, p)
			}
			return nil
		}))
		return
	}

	for run := 0; run < 2; run++ {
		_, err := b.Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "alice.acme/data")
		require.ErrorContains(t, err, "main")

		progress, parked := keys()
		require.NotNil(t, progress, "the run committed its progress")
		require.Equal(t, uint64(2), *progress)
		require.Len(t, parked, 1)
		require.NotEmpty(t, parked[0].Error)
		require.Equal(t, uint64(2), parked[0].Since, "since the run that first failed it")
	}
}
