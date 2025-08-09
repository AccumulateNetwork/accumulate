// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !race
// +build !race

package protocol

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestExecutorVersionSimple tests version methods
func TestExecutorVersionSimple(t *testing.T) {
	t.Parallel()
	
	// Test V1
	v1 := ExecutorVersionV1
	assert.True(t, v1.SignatureAnchoringEnabled())
	assert.False(t, v1.V2Enabled())
	assert.False(t, v1.HaltV1())
	
	// Test V2
	v2 := ExecutorVersionV2
	assert.True(t, v2.SignatureAnchoringEnabled())
	assert.True(t, v2.V2Enabled())
	assert.True(t, v2.HaltV1())
	assert.True(t, v2.DoubleHashEntriesEnabled())
	
	// Test V2Baikonur
	v2b := ExecutorVersionV2Baikonur
	assert.True(t, v2b.V2BaikonurEnabled())
	assert.True(t, v2b.V2Enabled())
	
	// Test V2Vandenberg
	v2v := ExecutorVersionV2Vandenberg
	assert.True(t, v2v.V2VandenbergEnabled())
	assert.True(t, v2v.V2BaikonurEnabled())
	
	// Test V2Jiuquan
	v2j := ExecutorVersionV2Jiuquan
	assert.True(t, v2j.V2JiuquanEnabled())
	assert.True(t, v2j.V2VandenbergEnabled())
}

// TestRationalSimple tests Rational methods
func TestRationalSimple(t *testing.T) {
	t.Parallel()
	
	r := &Rational{}
	
	// Test Set and Threshold
	r.Set(1, 2)
	assert.Equal(t, uint64(1), r.Numerator)
	assert.Equal(t, uint64(2), r.Denominator)
	assert.Equal(t, uint64(2), r.Threshold(4)) // 1/2 of 4 = 2
	
	r.Set(2, 3)
	assert.Equal(t, uint64(4), r.Threshold(6)) // 2/3 of 6 = 4
	
	r.Set(3, 4)
	assert.Equal(t, uint64(6), r.Threshold(8)) // 3/4 of 8 = 6
}

// TestFormatAmountSimple tests amount formatting
func TestFormatAmountSimple(t *testing.T) {
	t.Parallel()
	
	// Test FormatAmount
	assert.Equal(t, "0.00000000", FormatAmount(0, 8))
	assert.Equal(t, "1.00000000", FormatAmount(100000000, 8))
	assert.Equal(t, "0.12345678", FormatAmount(12345678, 8))
	assert.Equal(t, "0.00", FormatAmount(0, 2))
	assert.Equal(t, "12.34", FormatAmount(1234, 2))
	
	// Test FormatBigAmount
	assert.Equal(t, "0.00000000", FormatBigAmount(big.NewInt(0), 8))
	assert.Equal(t, "1.00000000", FormatBigAmount(big.NewInt(100000000), 8))
	assert.Equal(t, "0.12345678", FormatBigAmount(big.NewInt(12345678), 8))
}

// TestDataEntrySizeSimple tests data entry size functions
func TestDataEntrySizeSimple(t *testing.T) {
	t.Parallel()
	
	// Test with AccumulateDataEntry
	entry := &AccumulateDataEntry{
		Data: [][]byte{
			[]byte("test"),
			[]byte("data"),
		},
	}
	
	size, err := CheckDataEntrySize(entry)
	assert.NoError(t, err)
	assert.Greater(t, size, 0)
	
	cost, err := DataEntryCost(entry)
	assert.NoError(t, err)
	assert.Greater(t, cost, uint64(0))
	
	// Test with DoubleHashDataEntry
	entry2 := &DoubleHashDataEntry{
		Data: [][]byte{
			[]byte("double"),
			[]byte("hash"),
		},
	}
	
	size2, err := CheckDataEntrySize(entry2)
	assert.NoError(t, err)
	assert.Greater(t, size2, 0)
	
	cost2, err := DataEntryCost(entry2)
	assert.NoError(t, err)
	assert.Greater(t, cost2, uint64(0))
}

// TestDataEntryHashSimple tests data entry hash methods
func TestDataEntryHashSimple(t *testing.T) {
	t.Parallel()
	
	// Test AccumulateDataEntry Hash
	accEntry := &AccumulateDataEntry{
		Data: [][]byte{
			[]byte("accumulate"),
		},
	}
	hash1 := accEntry.Hash()
	assert.NotNil(t, hash1)
	assert.Equal(t, []byte("accumulate"), hash1) // Returns first data element
	
	// Test GetData
	data := accEntry.GetData()
	assert.Len(t, data, 1)
	assert.Equal(t, []byte("accumulate"), data[0])
	
	// Test DoubleHashDataEntry Hash
	dhEntry := &DoubleHashDataEntry{
		Data: [][]byte{
			[]byte("double"),
			[]byte("hash"),
		},
	}
	hash2 := dhEntry.Hash()
	assert.NotNil(t, hash2)
	assert.Len(t, hash2, 32) // Should be sha256 hash
	
	// Test GetData
	data2 := dhEntry.GetData()
	assert.Len(t, data2, 2)
	assert.Equal(t, []byte("double"), data2[0])
	assert.Equal(t, []byte("hash"), data2[1])
}

// TestComputeLiteDataAccountIdSimple tests lite data account ID
func TestComputeLiteDataAccountIdSimple(t *testing.T) {
	t.Parallel()
	
	// Test with AccumulateDataEntry
	entry1 := &AccumulateDataEntry{
		Data: [][]byte{
			[]byte("test1"),
		},
	}
	id1 := ComputeLiteDataAccountId(entry1)
	assert.NotNil(t, id1)
	assert.Len(t, id1, 32)
	
	// Test with different data
	entry2 := &AccumulateDataEntry{
		Data: [][]byte{
			[]byte("test2"),
		},
	}
	id2 := ComputeLiteDataAccountId(entry2)
	assert.NotNil(t, id2)
	assert.NotEqual(t, id1, id2)
	
	// Test with DoubleHashDataEntry
	entry3 := &DoubleHashDataEntry{
		Data: [][]byte{
			[]byte("test3"),
		},
	}
	id3 := ComputeLiteDataAccountId(entry3)
	assert.NotNil(t, id3)
	assert.Len(t, id3, 32)
}

// TestLiteDataAccountSimple tests LiteDataAccount methods
func TestLiteDataAccountSimple(t *testing.T) {
	t.Parallel()
	
	// Test with URL
	account := &LiteDataAccount{
		Url: AccountUrl("alice/data"),
	}
	
	id, err := account.AccountId()
	require.NoError(t, err)
	assert.NotNil(t, id)
	
	// Test without URL - should error
	account2 := &LiteDataAccount{}
	id2, err := account2.AccountId()
	assert.Error(t, err)
	assert.Nil(t, id2)
}

// TestPartitionSyntheticLedgerAdd tests ledger Add method
func TestPartitionSyntheticLedgerAdd(t *testing.T) {
	t.Parallel()
	
	ledger := &PartitionSyntheticLedger{}
	
	// Test adding pending transaction
	txid1 := &url.TxID{}
	dirty := ledger.Add(false, 1, txid1)
	assert.True(t, dirty)
	assert.Equal(t, uint64(1), ledger.Produced)
	
	// Test adding delivered transaction
	txid2 := &url.TxID{}
	dirty = ledger.Add(true, 2, txid2)
	assert.True(t, dirty)
	assert.Equal(t, uint64(2), ledger.Produced)
	
	// Test adding duplicate (should not be dirty)
	dirty = ledger.Add(true, 2, txid2)
	assert.False(t, dirty)
}

// TestPartitionSyntheticLedgerGetSimple tests ledger Get method  
func TestPartitionSyntheticLedgerGetSimple(t *testing.T) {
	t.Parallel()
	
	ledger := &PartitionSyntheticLedger{}
	
	// Add some transactions first
	txid1 := &url.TxID{}
	txid2 := &url.TxID{}
	ledger.Add(false, 1, txid1)
	ledger.Add(true, 2, txid2)
	
	// Test getting existing
	txid, ok := ledger.Get(1)
	assert.True(t, ok)
	assert.NotNil(t, txid)
	
	txid, ok = ledger.Get(2)
	assert.True(t, ok)
	assert.NotNil(t, txid)
	
	// Test getting non-existent
	txid, ok = ledger.Get(10)
	assert.False(t, ok)
	assert.Nil(t, txid)
}