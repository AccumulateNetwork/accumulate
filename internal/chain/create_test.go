package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func TestCreateAccountWithinNonAdi(t *testing.T) {
	// Setup
	db := database.OpenInMemory(nil)
	alice := url.MustParse("alice")
	aliceTokens := alice.JoinPath("tokens")
	badAccount := aliceTokens.JoinPath("account")
	_ = db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(alice), types.String(alice.String())))
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
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	badAccount := bob.JoinPath("account")
	_ = db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(alice), types.String(alice.String())))
		require.NoError(t, acctesting.CreateADI(batch, acctesting.GenerateTmKey(bob), types.String(bob.String())))
		return nil
	})

	cases := []struct {
		Body  protocol.TransactionBody
		Error string
	}{
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
		{&protocol.CreateIdentity{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
		{&protocol.CreateKeyBook{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
		{&protocol.CreateDataAccount{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
		{&protocol.CreateTokenAccount{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
		{&protocol.CreateToken{Url: badAccount}, "invalid principal: cannot create acc://bob/account as a child of acc://alice"},
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
