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
)

// TestExecutorVersionMethods tests version check methods
func TestExecutorVersionMethods(t *testing.T) {
	t.Parallel()

	// Test V2 versions
	assert.True(t, ExecutorVersionV2.V2Enabled())
	assert.True(t, ExecutorVersionV2Baikonur.V2BaikonurEnabled())
	assert.True(t, ExecutorVersionV2Vandenberg.V2VandenbergEnabled())
	assert.True(t, ExecutorVersionV2Jiuquan.V2JiuquanEnabled())

	// Test version comparison
	assert.True(t, ExecutorVersionV2 < ExecutorVersionV2Baikonur)
	assert.True(t, ExecutorVersionV2Baikonur < ExecutorVersionV2Vandenberg)
}

// TestRationalCalculation tests Rational threshold calculation
func TestRationalCalculation(t *testing.T) {
	t.Parallel()

	r := &Rational{}
	r.Set(1, 2)
	assert.Equal(t, uint64(2), r.Threshold(4)) // 1/2 of 4 = 2

	r.Set(2, 3)
	assert.Equal(t, uint64(4), r.Threshold(6)) // 2/3 of 6 = 4
}

// TestFormatAmountFunctions tests amount formatting
func TestFormatAmountFunctions(t *testing.T) {
	t.Parallel()

	// Test FormatAmount
	assert.Equal(t, "0.00000000", FormatAmount(0, 8))
	assert.Equal(t, "1.00000000", FormatAmount(100000000, 8))

	// Test FormatBigAmount
	assert.Equal(t, "0.00000000", FormatBigAmount(big.NewInt(0), 8))
	assert.Equal(t, "1.00000000", FormatBigAmount(big.NewInt(100000000), 8))
}

// TestDataEntryFunctions tests data entry functions
func TestDataEntryFunctions(t *testing.T) {
	t.Parallel()

	entry := &AccumulateDataEntry{
		Data: [][]byte{[]byte("test")},
	}

	// Test CheckDataEntrySize
	size, err := CheckDataEntrySize(entry)
	assert.NoError(t, err)
	assert.Greater(t, size, 0)

	// Test DataEntryCost
	cost, err := DataEntryCost(entry)
	assert.NoError(t, err)
	assert.Greater(t, cost, uint64(0))

	// Test Hash
	hash := entry.Hash()
	assert.NotNil(t, hash)

	// Test GetData
	data := entry.GetData()
	assert.Len(t, data, 1)
}

// TestComputeLiteDataAccountIdFunction tests ComputeLiteDataAccountId
func TestComputeLiteDataAccountIdFunction(t *testing.T) {
	t.Parallel()

	entry := &AccumulateDataEntry{
		Data: [][]byte{[]byte("test")},
	}

	id := ComputeLiteDataAccountId(entry)
	assert.NotNil(t, id)
	assert.Len(t, id, 32)
}

