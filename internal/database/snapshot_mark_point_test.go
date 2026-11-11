// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A chain collected into a snapshot and restored into a fresh store reads back
// every entry, whether or not its count is a mark point (a multiple of 256).
func TestAChainRestoresAtAndAroundAMarkPoint(t *testing.T) {
	for _, n := range []int{255, 256, 257, 512} {
		t.Run(fmt.Sprintf("count=%d", n), func(t *testing.T) {
			pool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
			db := database.OpenInMemory(nil)
			batch := db.Begin(true)
			defer batch.Discard()
			require.NoError(t, batch.Account(pool).Main().Put(&protocol.AnchorLedger{Url: pool}))
			chain, err := batch.Account(pool).AnchorChain(protocol.Directory).Root().Get()
			require.NoError(t, err)
			for i := 0; i < n; i++ {
				r := sha256.Sum256([]byte(fmt.Sprintf("root %d", i)))
				require.NoError(t, chain.AddEntry(r[:], false))
			}
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			snap := new(ioutil.Buffer)
			_, err = db.Collect(snap, nil, nil)
			require.NoError(t, err)
			restored := database.OpenInMemory(nil)
			require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap.Bytes()), nil))

			view := restored.Begin(false)
			defer view.Discard()
			inner := view.Account(pool).AnchorChain(protocol.Directory).Root().Inner()
			head, err := inner.Head().Get()
			require.NoError(t, err)
			require.Equal(t, int64(n), head.Count)

			var failed []int
			var firstErr error
			for i := 0; i < n; i++ {
				r := sha256.Sum256([]byte(fmt.Sprintf("root %d", i)))
				h, err := inner.Entry(int64(i))
				if err == nil && string(h) != string(r[:]) {
					err = fmt.Errorf("entry %d: wrong hash", i)
				}
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					failed = append(failed, i)
				}
			}
			if len(failed) > 0 {
				t.Fatalf("%d of %d entries fail, %d to %d; first: %v", len(failed), n, failed[0], failed[len(failed)-1], firstErr)
			}
		})
	}
}
