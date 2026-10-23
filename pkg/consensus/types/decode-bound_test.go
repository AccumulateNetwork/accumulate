// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"crypto/ed25519"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
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
// more than the same message with a count of one.
func TestUnmarshal_AllocationBoundedByInput(t *testing.T) {
	headerPrefix := make([]byte, 32+8+8+8) // author, round, epoch, timestamp

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	headerBytes, err := types.NewHeader(pub, 1, 0, nil, nil).Marshal()
	require.NoError(t, err)
	certPrefix := binary.BigEndian.AppendUint32(nil, uint32(len(headerBytes)))
	certPrefix = append(certPrefix, headerBytes...)

	// Signatures at their cap, each of length zero, so the authority count
	// passes the check that it matches the signature count.
	authPrefix := binary.BigEndian.AppendUint32(certPrefix, 10_000)
	authPrefix = append(authPrefix, make([]byte, 4*10_000)...)

	rows := []struct {
		name   string
		prefix []byte // the message up to the count field
		cap    uint32
		decode func([]byte) error
	}{
		{"batch transactions", nil, 1_000_000, func(b []byte) error { _, err := types.UnmarshalBatch(b); return err }},
		{"header payload", headerPrefix, 100_000, func(b []byte) error { _, err := types.UnmarshalHeader(b); return err }},
		{"header parents", binary.BigEndian.AppendUint32(headerPrefix, 0), 10_000, func(b []byte) error { _, err := types.UnmarshalHeader(b); return err }},
		{"certificate signatures", certPrefix, 10_000, func(b []byte) error { _, err := types.UnmarshalCertificate(b); return err }},
		{"certificate authorities", authPrefix, 10_000, func(b []byte) error { _, err := types.UnmarshalCertificate(b); return err }},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			one := binary.BigEndian.AppendUint32(append([]byte(nil), row.prefix...), 1)
			atCap := binary.BigEndian.AppendUint32(append([]byte(nil), row.prefix...), row.cap)
			require.Error(t, row.decode(one))
			require.Error(t, row.decode(atCap))

			perOne := bytesAllocatedPer(1000, func() { _ = row.decode(one) })
			perCap := bytesAllocatedPer(1000, func() { _ = row.decode(atCap) })
			assert.LessOrEqual(t, perCap, perOne+64, "bytes allocated per decode: count %d allocates %d, count 1 allocates %d", row.cap, perCap, perOne)
		})
	}
}
