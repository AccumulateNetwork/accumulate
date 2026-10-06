// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"crypto/sha256"
	"encoding/binary"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func keyHashN(n uint64) [32]byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	return sha256.Sum256(b[:])
}

// The distinct-key count of the deep-read statistics is exact for a handful
// of keys and within five percent from hundreds to millions, and a key shown
// again adds nothing.
func TestDistinctKeysCountsWhatItWasShown(t *testing.T) {
	for _, n := range []int{0, 1, 2, 10, 300, 5_000, 200_000, 3_000_000} {
		var s distinctKeys
		for i := 0; i < n; i++ {
			s.add(keyHashN(uint64(i)))
			if i%3 == 0 {
				s.add(keyHashN(uint64(i))) // again
			}
		}
		got := s.count()
		if n <= 10 {
			require.Equal(t, n, got, "%d keys", n)
		} else {
			require.InEpsilon(t, n, got, 0.05, "%d keys", n)
		}
	}
}

// The database holds one fixed-size estimator a shape and none of the keys:
// three thousand different keys read deep leave the statistics no larger than
// one does, and Distinct still says about how many there were. The set of
// keys it replaces held every key, to 200,000 a shape: 195 MiB of a
// validator's 780 MiB live heap after a day and a half at 10 tps (#4542).
func TestHistoryReadsHoldNoKeys(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	const n = 3000
	shape := keyShape(record.NewKey("Message", [32]byte{1}, "Main"))
	for i := uint64(0); i < n; i++ {
		d.recordHistoryRead(shape, keyHashN(i), true)
	}
	hr := d.HistoryReads()[shape]
	require.Equal(t, uint64(n), hr.Hits)
	require.InEpsilon(t, n, hr.Distinct, 0.05)

	d.fallbackMu.Lock()
	defer d.fallbackMu.Unlock()
	require.Len(t, d.historyKeys, 1)
	require.LessOrEqual(t, int(unsafe.Sizeof(*d.historyKeys[shape])), 4096, "the statistics hold the keys they counted")
}
