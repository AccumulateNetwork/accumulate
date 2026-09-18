// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestReview_RestoreWallClock measures restore wall-clock and peak heap on a
// real on-disk store, which the branch did not do. REVIEW_ENTRIES sets the
// total number of chain entries and REVIEW_ACCOUNTS how they are divided.
func TestReview_RestoreWallClock(t *testing.T) {
	if os.Getenv("REVIEW_ENTRIES") == "" {
		t.Skip("set REVIEW_ENTRIES")
	}
	total, err := strconv.Atoi(os.Getenv("REVIEW_ENTRIES"))
	require.NoError(t, err)
	nAcct := 1
	if s := os.Getenv("REVIEW_ACCOUNTS"); s != "" {
		nAcct, err = strconv.Atoi(s)
		require.NoError(t, err)
	}
	per := total / nAcct

	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	for a := 0; a < nAcct; a++ {
		u := protocol.PartitionUrl(fmt.Sprintf("BVN%d", a)).JoinPath(protocol.AnchorPool)
		pool := new(protocol.AnchorLedger)
		pool.Url = u
		require.NoError(t, batch.Account(u).Main().Put(pool))
		_ = big.NewInt(0)
		c, err := batch.Account(u).AnchorChain(protocol.Directory).Root().Get()
		require.NoError(t, err)
		var buf [16]byte
		for i := 0; i < per; i++ {
			binary.BigEndian.PutUint64(buf[:8], uint64(a))
			binary.BigEndian.PutUint64(buf[8:], uint64(i))
			h := sha256.Sum256(buf[:])
			require.NoError(t, c.AddEntry(h[:], false))
		}
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()

	snap := reviewCollect(t, built)
	t.Logf("snapshot: %d accounts x %d entries = %d entries, %.1f MiB",
		nAcct, per, nAcct*per, float64(len(snap))/(1<<20))

	db, err := database.OpenBadger(filepath.Join(t.TempDir(), "restored.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	runtime.GC()
	var peak uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			if m.HeapInuse > peak {
				peak = m.HeapInuse
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	start := time.Now()
	require.NoError(t, database.Restore(db, ioutil.NewBuffer(snap), nil))
	elapsed := time.Since(start)
	close(stop)
	<-done

	t.Logf("RESTORE %d entries into badger: %v (%.0f entries/s), peak HeapInuse during restore %.0f MiB",
		nAcct*per, elapsed, float64(nAcct*per)/elapsed.Seconds(), float64(peak)/(1<<20))
}
