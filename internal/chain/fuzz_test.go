package chain_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func FuzzCreateIdentity(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateIdentity{
		Url:        AccountUrl("bar"),
		KeyBookUrl: AccountUrl("bar", "book")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*CreateIdentity](t, dataHeader, dataBody)
		sender := new(TokenAccount)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		validateTransaction(t, txn, sender, chain.CreateIdentity{})
	})
}

func FuzzCreateTokenAccount(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateTokenAccount{
		Url:      AccountUrl("foo", "bar"),
		TokenUrl: AcmeUrl()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, body := unpackTransaction[*CreateTokenAccount](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		sender := new(ADI)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		validateTransaction(t, txn, sender, chain.CreateTokenAccount{})

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = sender.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			validateTransaction(t, txn, sender, chain.CreateTokenAccount{})
		})
	})
}

func FuzzSendTokens(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &SendTokens{
		To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*SendTokens](t, dataHeader, dataBody)
		sender := new(TokenAccount)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		sender.Balance = *big.NewInt(1e12)
		validateTransaction(t, txn, sender, chain.SendTokens{})
	})
}

func FuzzCreateDataAccount(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateDataAccount{
		Url: AccountUrl("foo", "bar")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*CreateDataAccount](t, dataHeader, dataBody)
		sender := new(ADI)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		validateTransaction(t, txn, sender, chain.WriteData{})

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = sender.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			validateTransaction(t, txn, sender, chain.WriteData{})
		})
	})
}

func FuzzWriteData(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: (&FactomDataEntry{AccountId: [32]byte{1, 2, 3}, Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}).Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		sender := new(DataAccount)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		validateTransaction(t, txn, sender, chain.WriteData{})
	})
}

func FuzzWriteData_Lite(f *testing.F) {
	factomEntry := &FactomDataEntry{Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}
	ldaAddr := ComputeLiteDataAccountId(factomEntry.Wrap())
	factomEntry.AccountId = *(*[32]byte)(ldaAddr)
	ldaUrl, err := LiteDataAddress(ldaAddr)
	require.NoError(f, err)

	addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: factomEntry.Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		sender := new(LiteDataAccount)
		sender.Url = txn.Header.Principal
		validateTransaction(t, txn, sender, chain.WriteData{})
	})
}

func FuzzAddCredits(f *testing.F) {
	addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &AddCredits{
		Recipient: AccountUrl("bar"),
		Amount:    *big.NewInt(123),
		Oracle:    InitialAcmeOracleValue})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*AddCredits](t, dataHeader, dataBody)
		sender := new(TokenAccount)
		sender.AddAuthority(&url.URL{Authority: Unknown})
		sender.Url = txn.Header.Principal
		sender.Balance = *big.NewInt(1e12)
		validateTransaction(t, txn, sender, chain.AddCredits{})
	})
}

func addTransaction(f *testing.F, header TransactionHeader, body TransactionBody) {
	dataHeader, err := header.MarshalBinary()
	require.NoError(f, err)
	dataBody, err := body.MarshalBinary()
	require.NoError(f, err)
	f.Add(dataHeader, dataBody)
}

type bodyPtr[T any] interface {
	*T
	TransactionBody
}

func unpackTransaction[PT bodyPtr[T], T any](t *testing.T, dataHeader, dataBody []byte) (*Transaction, PT) {
	var header TransactionHeader
	body := PT(new(T))
	if header.UnmarshalBinary(dataHeader) != nil {
		t.Skip()
	}
	if body.UnmarshalBinary(dataBody) != nil {
		t.Skip()
	}
	if header.Principal == nil {
		t.Skip()
	}
	return &Transaction{Header: header, Body: body}, body
}

func validateTransaction(t *testing.T, txn *Transaction, principal Account, executor chain.TransactionExecutor) {
	db := database.OpenInMemory(nil)
	err := TryMakeAccount(t, db, principal)
	if err != nil {
		t.Skip()
	}

	st, err := chain.NewStateManagerForFuzz(t, db, txn)
	if err != nil {
		t.Skip()
	}
	defer st.Discard()

	_, _ = (chain.WriteData{}).Validate(st, &chain.Delivery{Transaction: txn})
}
