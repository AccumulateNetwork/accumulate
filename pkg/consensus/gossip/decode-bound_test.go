// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bytesAllocatedPer reports the bytes fn allocates per run: the least of
// three trials of n runs each. TotalAlloc counts the whole process, so a
// trial can also catch what the runtime allocates in the background; on the
// 2-vCPU CI runner one measurement in sixteen did, 5.5 KB over ten runs. The
// least trial is the decode's own figure, and a slice sized by the count
// would show in every trial.
func bytesAllocatedPer(n int, fn func()) uint64 {
	least := ^uint64(0)
	for trial := 0; trial < 3; trial++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for i := 0; i < n; i++ {
			fn()
		}
		runtime.ReadMemStats(&after)
		if per := (after.TotalAlloc - before.TotalAlloc) / uint64(n); per < least {
			least = per
		}
	}
	return least
}

// A count field is checked against the bytes that remain before anything is
// allocated for it (#4599). Each row ends a message at a count field, writes
// the count at its cap, and requires that the decode fails and allocates no
// more than the same message with a count of one. A row's pad keeps the
// message at the decoder's minimum length.
func TestUnmarshal_AllocationBoundedByInput(t *testing.T) {
	requestID := make([]byte, 8)
	noRequester := binary.BigEndian.AppendUint32(requestID, 0)

	rows := []struct {
		name   string
		prefix []byte // the message up to the count field
		cap    uint32
		pad    int
		decode func([]byte) error
	}{
		{"certificate list", nil, 100_000, 0,
			func(b []byte) error { _, err := unmarshalCertificateList(b); return err }},
		{"cert sync request digests", noRequester, MaxSyncDigests, 0,
			func(b []byte) error { _, err := UnmarshalCertSyncRequest(b); return err }},
		{"cert sync request rounds", binary.BigEndian.AppendUint32(noRequester, 0), MaxSyncRounds, 0,
			func(b []byte) error { _, err := UnmarshalCertSyncRequest(b); return err }},
		{"cert sync response certificates", noRequester, MaxSyncCertificates, 4,
			func(b []byte) error { _, err := UnmarshalCertSyncResponse(b); return err }},
		{"cert sync response missing", binary.BigEndian.AppendUint32(noRequester, 0), MaxSyncDigests, 0,
			func(b []byte) error { _, err := UnmarshalCertSyncResponse(b); return err }},
		{"bpt sync request keys", noRequester, MaxBPTSyncKeys, 0,
			func(b []byte) error { _, err := UnmarshalBPTSyncRequest(b); return err }},
		{"bpt sync response entries", requestID, MaxBPTSyncEntries, 4,
			func(b []byte) error { _, err := UnmarshalBPTSyncResponse(b); return err }},
		{"bpt sync response missing", binary.BigEndian.AppendUint32(requestID, 0), MaxBPTSyncKeys, 0,
			func(b []byte) error { _, err := UnmarshalBPTSyncResponse(b); return err }},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			msg := func(count uint32) []byte {
				b := binary.BigEndian.AppendUint32(append([]byte(nil), row.prefix...), count)
				return append(b, make([]byte, row.pad)...)
			}
			one, atCap := msg(1), msg(row.cap)
			require.Error(t, row.decode(one))
			require.Error(t, row.decode(atCap))

			perOne := bytesAllocatedPer(1000, func() { _ = row.decode(one) })
			perCap := bytesAllocatedPer(1000, func() { _ = row.decode(atCap) })
			assert.LessOrEqual(t, perCap, perOne+64, "bytes allocated per decode: count %d allocates %d, count 1 allocates %d", row.cap, perCap, perOne)
		})
	}
}
