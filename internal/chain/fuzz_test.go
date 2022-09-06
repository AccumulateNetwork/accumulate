package chain_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func FuzzCreateIdentity(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateIdentity{
		Url:        AccountUrl("bar"),
		KeyBookUrl: AccountUrl("bar", "book"),
		KeyHash:    make([]byte, 32)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*CreateIdentity](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.CreateIdentity{}, matchesAny(txn, h1))
	})
}

func FuzzCreateTokenAccount(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateTokenAccount{
		Url:      AccountUrl("foo", "bar"),
		TokenUrl: AcmeUrl()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, body := unpackTransaction[*CreateTokenAccount](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.CreateTokenAccount{}, matchesAny(txn, h1))

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			validateTransaction(t, txn, principal, chain.CreateTokenAccount{}, matchesAny(txn, h1))
		})
	})
}

func FuzzSendTokens(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &SendTokens{
		To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*SendTokens](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.Balance = *big.NewInt(1e12)
		validateTransaction(t, txn, principal, chain.SendTokens{}, matchesAny(txn, h1))
	})
}

func FuzzCreateDataAccount(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateDataAccount{
		Url: AccountUrl("foo", "bar")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*CreateDataAccount](t, dataHeader, dataBody)
		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.CreateDataAccount{}, matchesAny(txn, h1))

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			validateTransaction(t, txn, principal, chain.CreateDataAccount{}, matchesAny(txn, h1))
		})
	})
}

func FuzzWriteData(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	h2 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: (&FactomDataEntry{AccountId: [32]byte{1, 2, 3}, Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}).Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(DataAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.WriteData{}, matchesAny(txn, h1, h2))
	})
}

func FuzzWriteData_Lite(f *testing.F) {
	factomEntry := &FactomDataEntry{Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}
	ldaAddr := ComputeLiteDataAccountId(factomEntry.Wrap())
	factomEntry.AccountId = *(*[32]byte)(ldaAddr)
	ldaUrl, err := LiteDataAddress(ldaAddr)
	require.NoError(f, err)

	h1 := addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	h2 := addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: factomEntry.Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(LiteDataAccount)
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.WriteData{}, matchesAny(txn, h1, h2))
	})
}

func FuzzWriteDataTo(f *testing.F) {
	factomEntry := &FactomDataEntry{Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}
	ldaAddr := ComputeLiteDataAccountId(factomEntry.Wrap())
	factomEntry.AccountId = *(*[32]byte)(ldaAddr)
	ldaUrl, err := LiteDataAddress(ldaAddr)
	require.NoError(f, err)

	h1 := addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteDataTo{
		Recipient: ldaUrl,
		Entry:     &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	h2 := addTransaction(f, TransactionHeader{Principal: ldaUrl}, &WriteDataTo{
		Recipient: ldaUrl,
		Entry:     factomEntry.Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*WriteDataTo](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(LiteDataAccount)
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.WriteDataTo{}, matchesAny(txn, h1, h2))
	})
}

func FuzzAcmeFaucet(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: FaucetUrl}, &AcmeFaucet{
		Url: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey())})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*AcmeFaucet](t, dataHeader, dataBody)
		principal := new(LiteTokenAccount)
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		validateTransaction(t, txn, principal, chain.AcmeFaucet{}, matchesAny(txn, h1))

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			receiver := new(LiteTokenAccount)
			receiver.Url = body.Url
			receiver.TokenUrl = AcmeUrl()
			validateTransaction(t, txn, principal, chain.AcmeFaucet{}, matchesAny(txn, h1), receiver)
		})
	})
}

func FuzzCreateToken(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &CreateToken{
		Url: AccountUrl("foo", "bar")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := unpackTransaction[*CreateToken](t, dataHeader, dataBody)
		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.CreateToken{}, matchesAny(txn, h1))

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			validateTransaction(t, txn, principal, chain.CreateToken{}, matchesAny(txn, h1))
		})
	})
}

func FuzzIssueTokens(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &IssueTokens{
		To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*IssueTokens](t, dataHeader, dataBody)
		principal := new(TokenIssuer)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		validateTransaction(t, txn, principal, chain.IssueTokens{}, matchesAny(txn, h1))
	})
}

func FuzzBurnTokens(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &BurnTokens{
		Amount: *big.NewInt(123)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*BurnTokens](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		principal.Balance = *big.NewInt(1e15)
		validateTransaction(t, txn, principal, chain.BurnTokens{}, matchesAny(txn, h1))
	})
}

// func FuzzCreateLiteTokenAccount(f *testing.F)

// func FuzzCreateKeyPage(f *testing.F)

// func FuzzCreateKeyBook(f *testing.F)

func FuzzAddCredits(f *testing.F) {
	h1 := addTransaction(f, TransactionHeader{Principal: AccountUrl("foo")}, &AddCredits{
		Recipient: AccountUrl("bar"),
		Amount:    *big.NewInt(123 * AcmePrecision),
		Oracle:    InitialAcmeOracleValue})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := unpackTransaction[*AddCredits](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		principal.Balance = *big.NewInt(1e12)
		ledger := new(SystemLedger)
		ledger.Url = acctesting.BvnUrlForTest(t).JoinPath(Ledger)
		validateTransaction(t, txn, principal, chain.AddCredits{}, matchesAny(txn, h1), ledger)
	})
}

// func FuzzUpdateKeyPage(f *testing.F)

// func FuzzLockAccount(f *testing.F)

// func FuzzUpdateAccountAuth(f *testing.F)

// func FuzzUpdateKey(f *testing.F)

func addTransaction(f *testing.F, header TransactionHeader, body TransactionBody) []byte {
	dataHeader, err := header.MarshalBinary()
	require.NoError(f, err)
	dataBody, err := body.MarshalBinary()
	require.NoError(f, err)
	f.Add(dataHeader, dataBody)
	return (&Transaction{Header: header, Body: body}).GetHash()
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

func validateTransaction(t *testing.T, txn *Transaction, principal Account, executor chain.TransactionExecutor, requireSuccess bool, extraAccounts ...Account) {
	require.Equal(t, txn.Body.Type(), executor.Type())

	db := database.OpenInMemory(nil)
	err := TryMakeAccount(t, db, append(extraAccounts, principal)...)
	if err != nil {
		t.Skip()
	}

	st, err := chain.NewStateManagerForFuzz(t, db, txn)
	if err != nil {
		t.Skip()
	}
	defer st.Discard()

	_, err = executor.Validate(st, &chain.Delivery{Transaction: txn})
	if requireSuccess {
		require.NoError(t, err)
	}
}

func matchesAny(txn *Transaction, test ...[]byte) bool {
	value := txn.GetHash()
	for _, test := range test {
		if bytes.Equal(test, value) {
			return true
		}
	}
	return false
}
