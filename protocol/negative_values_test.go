// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNegativeValues(t *testing.T) {
	// NOTICE!!! If we update bigint marshalling to allow negative numbers, we
	// must add tests to verify that the transaction executors for these
	// transaction types reject negative values.

	t.Run("SendTokens", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(-10)}}}
		msg := "field To: failed to marshal field: field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("BurnTokens", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &BurnTokens{Amount: *big.NewInt(-10)}
		msg := "field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("CreateToken", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &CreateToken{Url: AccountUrl("bar"), SupplyLimit: big.NewInt(-10), Symbol: "BAR", Precision: 8}
		msg := "field SupplyLimit: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("IssueTokens", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &IssueTokens{To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(-10)}}}
		msg := "field To: failed to marshal field: field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("AddCredits", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &AddCredits{Recipient: AccountUrl("bar"), Amount: *big.NewInt(-10)}
		msg := "field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("SyntheticDepositToken", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &SyntheticDepositTokens{Token: AccountUrl("bar"), Amount: *big.NewInt(-10)}
		msg := "field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})

	t.Run("SyntheticBurnToken", func(t *testing.T) {
		txn := new(Transaction)
		txn.Header.Principal = AccountUrl("foo")
		txn.Body = &SyntheticBurnTokens{Amount: *big.NewInt(-10)}
		msg := "field Amount: negative big int values are not supported"
		require.PanicsWithError(t, msg, func() { txn.GetHash() })
	})
}
