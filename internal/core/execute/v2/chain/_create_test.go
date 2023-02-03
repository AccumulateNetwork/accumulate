// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreateAccountWithinNonAdi(t *testing.T) {
	// Setup
	db := database.OpenInMemory(nil)
	alice := protocol.AccountUrl("alice")
	aliceTokens := alice.JoinPath("tokens")
	badAccount := aliceTokens.JoinPath("account")
	_ = db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(alice), alice.String()))
		require.NoError(t, acctesting.CreateTokenAccount(batch, aliceTokens.String(), protocol.ACME, 0, false))
		return nil
	})

	cases := []struct {
		Body  protocol.TransactionBody
		Error string
	}{
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
		{&protocol.CreateIdentity{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
		{&protocol.CreateKeyBook{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
		{&protocol.CreateTokenAccount{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
		{&protocol.CreateToken{Url: badAccount}, "invalid principal: want account type identity, got tokenAccount"},
	}

	for _, c := range cases {
		t.Run(c.Body.Type().String(), func(t *testing.T) {
			txn := &protocol.Transaction{
				Header: protocol.TransactionHeader{
					Principal: aliceTokens,
				},
				Body: c.Body,
			}
			st := NewStateManagerForTest(t, db, txn)
			defer st.Discard()

			_, err := GetExecutor(t, c.Body.Type()).Validate(st, &Delivery{Transaction: txn})
			require.EqualError(t, err, c.Error)
		})
	}
}

func TestCreateAccountWithinOtherAdi(t *testing.T) {
	// Setup
	db := database.OpenInMemory(nil)
	alice := protocol.AccountUrl("alice")
	bob := protocol.AccountUrl("bob")
	badAccount := bob.JoinPath("account")
	_ = db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(alice), alice.String()))
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(bob), bob.String()))
		return nil
	})

	cases := []struct {
		Body  protocol.TransactionBody
		Error string
	}{
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
		{&protocol.CreateIdentity{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
		{&protocol.CreateKeyBook{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
		{&protocol.CreateTokenAccount{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
		{&protocol.CreateToken{Url: badAccount}, "invalid principal: cannot create acc://bob.acme/account as a child of acc://alice.acme"},
	}

	for _, c := range cases {
		t.Run(c.Body.Type().String(), func(t *testing.T) {
			txn := &protocol.Transaction{
				Header: protocol.TransactionHeader{
					Principal: alice,
				},
				Body: c.Body,
			}
			st := NewStateManagerForTest(t, db, txn)
			defer st.Discard()

			_, err := GetExecutor(t, c.Body.Type()).Validate(st, &Delivery{Transaction: txn})
			require.EqualError(t, err, c.Error)
		})
	}
}
