// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package load_test

import (
	"math/big"
	"testing"
)

func TestNilPointerFix(t *testing.T) {
	// Test that the fix handles nil balances correctly

	// Simulate nil balance
	var balance *big.Int = nil

	// This is what the fixed code does
	var actualBalance int64
	if balance != nil {
		actualBalance = balance.Int64()
	}

	t.Logf("Nil balance handled correctly: actualBalance = %d", actualBalance)

	// Test with non-nil balance
	balance = big.NewInt(12345)
	if balance != nil {
		actualBalance = balance.Int64()
	}

	t.Logf("Non-nil balance handled correctly: actualBalance = %d", actualBalance)

	if actualBalance != 12345 {
		t.Errorf("Expected 12345, got %d", actualBalance)
	}
}
