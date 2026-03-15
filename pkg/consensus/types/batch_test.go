// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestBatch_Digest(t *testing.T) {
	t.Run("empty batch", func(t *testing.T) {
		batch := types.NewBatch(nil)
		digest := batch.Digest()
		assert.False(t, digest.IsZero())
	})

	t.Run("single transaction", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("hello")})
		digest := batch.Digest()
		assert.False(t, digest.IsZero())
	})

	t.Run("multiple transactions", func(t *testing.T) {
		batch := types.NewBatch([][]byte{
			[]byte("tx1"),
			[]byte("tx2"),
			[]byte("tx3"),
		})
		digest := batch.Digest()
		assert.False(t, digest.IsZero())
	})

	t.Run("deterministic", func(t *testing.T) {
		txs := [][]byte{
			[]byte("tx1"),
			[]byte("tx2"),
		}

		batch1 := types.NewBatch(txs)
		batch2 := types.NewBatch(txs)

		assert.Equal(t, batch1.Digest(), batch2.Digest())
	})

	t.Run("different transactions produce different digests", func(t *testing.T) {
		batch1 := types.NewBatch([][]byte{[]byte("tx1")})
		batch2 := types.NewBatch([][]byte{[]byte("tx2")})

		assert.NotEqual(t, batch1.Digest(), batch2.Digest())
	})

	t.Run("order matters", func(t *testing.T) {
		batch1 := types.NewBatch([][]byte{[]byte("a"), []byte("b")})
		batch2 := types.NewBatch([][]byte{[]byte("b"), []byte("a")})

		assert.NotEqual(t, batch1.Digest(), batch2.Digest())
	})

	t.Run("caching works", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("test")})

		digest1 := batch.Digest()
		digest2 := batch.Digest()

		assert.Equal(t, digest1, digest2)
	})
}

func TestBatch_Marshal(t *testing.T) {
	t.Run("empty batch", func(t *testing.T) {
		batch := types.NewBatch(nil)
		data, err := batch.Marshal()
		require.NoError(t, err)
		assert.NotNil(t, data)

		unmarshaled, err := types.UnmarshalBatch(data)
		require.NoError(t, err)
		assert.Equal(t, 0, unmarshaled.Len())
	})

	t.Run("round trip", func(t *testing.T) {
		txs := [][]byte{
			[]byte("transaction 1"),
			[]byte("transaction 2"),
			[]byte("transaction 3"),
		}
		batch := types.NewBatch(txs)

		data, err := batch.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalBatch(data)
		require.NoError(t, err)

		require.Equal(t, len(txs), unmarshaled.Len())
		for i, tx := range unmarshaled.Transactions {
			assert.True(t, bytes.Equal(txs[i], tx))
		}
	})

	t.Run("digest preserved after marshal/unmarshal", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("test data")})
		originalDigest := batch.Digest()

		data, err := batch.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalBatch(data)
		require.NoError(t, err)

		assert.Equal(t, originalDigest, unmarshaled.Digest())
	})

	t.Run("large transactions", func(t *testing.T) {
		largeTx := make([]byte, 10000)
		for i := range largeTx {
			largeTx[i] = byte(i % 256)
		}

		batch := types.NewBatch([][]byte{largeTx})

		data, err := batch.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalBatch(data)
		require.NoError(t, err)

		require.Equal(t, 1, unmarshaled.Len())
		assert.True(t, bytes.Equal(largeTx, unmarshaled.Transactions[0]))
	})
}

func TestBatch_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := types.UnmarshalBatch([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("truncated transaction length", func(t *testing.T) {
		// Valid count but no transaction data
		data := []byte{0, 0, 0, 1} // 1 transaction
		_, err := types.UnmarshalBatch(data)
		assert.Error(t, err)
	})

	t.Run("truncated transaction data", func(t *testing.T) {
		// 1 transaction of length 10, but only 5 bytes provided
		data := []byte{
			0, 0, 0, 1, // 1 transaction
			0, 0, 0, 10, // length 10
			1, 2, 3, 4, 5, // only 5 bytes
		}
		_, err := types.UnmarshalBatch(data)
		assert.Error(t, err)
	})

	t.Run("trailing bytes", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("test")})
		data, _ := batch.Marshal()
		data = append(data, 0xFF) // Add trailing byte

		_, err := types.UnmarshalBatch(data)
		assert.Error(t, err)
	})
}

func TestBatch_Size(t *testing.T) {
	t.Run("empty batch", func(t *testing.T) {
		batch := types.NewBatch(nil)
		assert.Equal(t, 0, batch.Size())
	})

	t.Run("with transactions", func(t *testing.T) {
		batch := types.NewBatch([][]byte{
			[]byte("hello"),  // 5 bytes
			[]byte("world!"), // 6 bytes
		})
		assert.Equal(t, 11, batch.Size())
	})
}

func TestBatch_Clone(t *testing.T) {
	original := types.NewBatch([][]byte{
		[]byte("tx1"),
		[]byte("tx2"),
	})

	clone := original.Clone()

	// Same content
	assert.Equal(t, original.Digest(), clone.Digest())
	assert.Equal(t, original.Len(), clone.Len())

	// But different underlying slices
	clone.Transactions[0][0] = 'X'
	assert.NotEqual(t, original.Transactions[0], clone.Transactions[0])
}

func TestBatch_ConcurrentDigest(t *testing.T) {
	batch := types.NewBatch([][]byte{[]byte("concurrent test")})

	var wg sync.WaitGroup
	digests := make([]types.BatchDigest, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			digests[idx] = batch.Digest()
		}(i)
	}

	wg.Wait()

	// All digests should be the same
	for i := 1; i < len(digests); i++ {
		assert.Equal(t, digests[0], digests[i])
	}
}

func TestBatchDigest_String(t *testing.T) {
	batch := types.NewBatch([][]byte{[]byte("test")})
	digest := batch.Digest()

	str := digest.String()
	assert.Len(t, str, 64) // 32 bytes = 64 hex chars
}
