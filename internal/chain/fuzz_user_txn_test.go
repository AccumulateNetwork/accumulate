// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/fuzzutil"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func FuzzCreateIdentity(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateIdentity{
		Url:        AccountUrl("bar"),
		KeyBookUrl: AccountUrl("bar", "book"),
		KeyHash:    make([]byte, 32)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*CreateIdentity](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.CreateIdentity{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzCreateTokenAccount(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateTokenAccount{
		Url:      AccountUrl("foo", "bar"),
		TokenUrl: AcmeUrl()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, body := fuzzutil.UnpackTransaction[*CreateTokenAccount](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.CreateTokenAccount{}, h[txn.ID().Hash()], principal)

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			fuzzutil.ValidateTransaction(t, txn, chain.CreateTokenAccount{}, h[txn.ID().Hash()], principal)
		})
	})
}

func FuzzSendTokens(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &SendTokens{
		To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*SendTokens](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.Balance = *big.NewInt(1e12)
		fuzzutil.ValidateTransaction(t, txn, chain.SendTokens{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzCreateDataAccount(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateDataAccount{
		Url: AccountUrl("foo", "bar")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*CreateDataAccount](t, dataHeader, dataBody)
		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.CreateDataAccount{}, h[txn.ID().Hash()], principal)

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			fuzzutil.ValidateTransaction(t, txn, chain.CreateDataAccount{}, h[txn.ID().Hash()], principal)
		})
	})
}

func FuzzWriteData(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &WriteData{
		Entry: (&FactomDataEntry{AccountId: [32]byte{1, 2, 3}, Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}).Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(DataAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.WriteData{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzWriteData_Lite(f *testing.F) {
	factomEntry := &FactomDataEntry{Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}
	ldaAddr := ComputeLiteDataAccountId(factomEntry.Wrap())
	factomEntry.AccountId = *(*[32]byte)(ldaAddr)
	ldaUrl, err := LiteDataAddress(ldaAddr)
	require.NoError(f, err)

	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: ldaUrl}, &WriteData{
		Entry: factomEntry.Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*WriteData](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(LiteDataAccount)
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.WriteData{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzWriteDataTo(f *testing.F) {
	factomEntry := &FactomDataEntry{Data: []byte{1}, ExtIds: [][]byte{nil, {1}, {2, 3}}}
	ldaAddr := ComputeLiteDataAccountId(factomEntry.Wrap())
	factomEntry.AccountId = *(*[32]byte)(ldaAddr)
	ldaUrl, err := LiteDataAddress(ldaAddr)
	require.NoError(f, err)

	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: ldaUrl}, &WriteDataTo{
		Recipient: ldaUrl,
		Entry:     &AccumulateDataEntry{Data: [][]byte{nil, {1}, {2, 3}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: ldaUrl}, &WriteDataTo{
		Recipient: ldaUrl,
		Entry:     factomEntry.Wrap()})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*WriteDataTo](t, dataHeader, dataBody)
		if entry, ok := body.Entry.(*FactomDataEntryWrapper); ok && entry.AccountId == ([32]byte{}) {
			t.Skip()
		}
		principal := new(LiteDataAccount)
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.WriteDataTo{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzAcmeFaucet(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: FaucetUrl}, &AcmeFaucet{
		Url: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey())})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*AcmeFaucet](t, dataHeader, dataBody)
		principal := new(LiteIdentity)
		principal.Url = txn.Header.Principal
		faucet := new(LiteTokenAccount)
		faucet.Url = protocol.FaucetUrl
		faucet.TokenUrl = AcmeUrl()
		faucet.Balance = *big.NewInt(AcmeFaucetAmount * AcmePrecision)
		fuzzutil.ValidateTransaction(t, txn, chain.AcmeFaucet{}, h[txn.ID().Hash()], principal, faucet)

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			receiver := new(LiteTokenAccount)
			receiver.Url = body.Url
			receiver.TokenUrl = AcmeUrl()
			fuzzutil.ValidateTransaction(t, txn, chain.AcmeFaucet{}, h[txn.ID().Hash()], receiver, principal, faucet)
		})
	})
}

func FuzzCreateToken(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateToken{
		Url: AccountUrl("foo", "bar")})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, body := fuzzutil.UnpackTransaction[*CreateToken](t, dataHeader, dataBody)
		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.CreateToken{}, h[txn.ID().Hash()], principal)

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			fuzzutil.ValidateTransaction(t, txn, chain.CreateToken{}, h[txn.ID().Hash()], principal)
		})
	})
}

func FuzzIssueTokens(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &IssueTokens{
		To: []*TokenRecipient{{Url: AccountUrl("bar"), Amount: *big.NewInt(123)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*IssueTokens](t, dataHeader, dataBody)
		principal := new(TokenIssuer)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.IssueTokens{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzBurnTokens(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &BurnTokens{
		Amount: *big.NewInt(123)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*BurnTokens](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		principal.Balance = *big.NewInt(1e15)
		fuzzutil.ValidateTransaction(t, txn, chain.BurnTokens{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzCreateLiteTokenAccount(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey())}, &CreateLiteTokenAccount{})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*CreateLiteTokenAccount](t, dataHeader, dataBody)
		fuzzutil.ValidateTransaction(t, txn, chain.CreateLiteTokenAccount{}, h[txn.ID().Hash()])
	})
}

func FuzzCreateKeyPage(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateKeyPage{
		Keys: []*KeySpecParams{{KeyHash: make([]byte, 32)}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, _ := fuzzutil.UnpackTransaction[*CreateKeyPage](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		principal := new(KeyBook)
		principal.Url = txn.Header.Principal
		principal.AddAuthority(principal.Url)
		fuzzutil.ValidateTransaction(t, txn, chain.CreateKeyPage{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzCreateKeyBook(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &CreateKeyBook{
		Url:           AccountUrl("foo", "bar"),
		PublicKeyHash: make([]byte, 32)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()

		txn, body := fuzzutil.UnpackTransaction[*CreateKeyBook](t, dataHeader, dataBody)
		if txn.Header.Principal == nil {
			t.Skip()
		}

		principal := new(ADI)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		fuzzutil.ValidateTransaction(t, txn, chain.CreateKeyBook{}, h[txn.ID().Hash()], principal)

		t.Run("Adjusted", func(t *testing.T) {
			if body.Url == nil {
				t.Skip()
			}
			body := body.Copy()
			body.Url.Authority = principal.Url.Authority
			txn := txn.Copy()
			txn.Body = body
			fuzzutil.ValidateTransaction(t, txn, chain.CreateKeyBook{}, h[txn.ID().Hash()], principal)
		})
	})
}

func FuzzAddCredits(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &AddCredits{
		Recipient: AccountUrl("bar"),
		Amount:    *big.NewInt(123 * AcmePrecision),
		Oracle:    InitialAcmeOracleValue})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*AddCredits](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(&url.URL{Authority: Unknown})
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		principal.Balance = *big.NewInt(1e12)
		ledger := new(SystemLedger)
		ledger.Url = acctesting.BvnUrlForTest(t).JoinPath(Ledger)
		fuzzutil.ValidateTransaction(t, txn, chain.AddCredits{}, h[txn.ID().Hash()], ledger, principal)
	})
}

func FuzzUpdateKeyPage(f *testing.F) {
	existingKey := helpers.Hash(acctesting.GenerateKey()[32:])
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&AddKeyOperation{Entry: KeySpecParams{KeyHash: make([]byte, 32)}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&AddKeyOperation{Entry: KeySpecParams{KeyHash: make([]byte, 32), Delegate: AccountUrl("bar")}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&AddKeyOperation{Entry: KeySpecParams{KeyHash: make([]byte, 32)}},
		&SetThresholdKeyPageOperation{Threshold: 1}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&UpdateKeyOperation{OldEntry: KeySpecParams{KeyHash: existingKey}, NewEntry: KeySpecParams{KeyHash: make([]byte, 32)}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&RemoveKeyOperation{Entry: KeySpecParams{KeyHash: existingKey}}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&SetThresholdKeyPageOperation{Threshold: 1}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "2")}, &UpdateKeyPage{Operation: []KeyPageOperation{
		&UpdateAllowedKeyPageOperation{Allow: []TransactionType{TransactionTypeUpdateKeyPage}, Deny: []TransactionType{TransactionTypeUpdateAccountAuth}}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*UpdateKeyPage](t, dataHeader, dataBody)
		parent, ok := txn.Header.Principal.Parent()
		if !ok {
			t.Skip()
		}
		principal := new(KeyPage)
		principal.Url = txn.Header.Principal
		principal.AddKeySpec(&KeySpec{PublicKeyHash: helpers.Hash([]byte("foo"))})
		principal.AddKeySpec(&KeySpec{PublicKeyHash: existingKey})
		book := new(KeyBook)
		book.Url = parent
		book.AddAuthority(book.Url)
		fuzzutil.ValidateTransaction(t, txn, chain.UpdateKeyPage{}, h[txn.ID().Hash()], book, principal)
	})
}

func FuzzLockAccount(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey())}, &LockAccount{
		Height: 123})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*LockAccount](t, dataHeader, dataBody)
		principal := new(LiteTokenAccount)
		principal.Url = txn.Header.Principal
		principal.TokenUrl = AcmeUrl()
		fuzzutil.ValidateTransaction(t, txn, chain.LockAccount{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzUpdateAccountAuth(f *testing.F) {
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &UpdateAccountAuth{Operations: []AccountAuthOperation{
		&AddAccountAuthorityOperation{Authority: AccountUrl("bar")}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &UpdateAccountAuth{Operations: []AccountAuthOperation{
		&RemoveAccountAuthorityOperation{Authority: AccountUrl("baz")}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &UpdateAccountAuth{Operations: []AccountAuthOperation{
		&EnableAccountAuthOperation{Authority: AccountUrl("baz")}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &UpdateAccountAuth{Operations: []AccountAuthOperation{
		&DisableAccountAuthOperation{Authority: AccountUrl("baz")}}})
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo")}, &UpdateAccountAuth{Operations: []AccountAuthOperation{
		&AddAccountAuthorityOperation{Authority: AccountUrl("bar")},
		&RemoveAccountAuthorityOperation{Authority: AccountUrl("baz")}}})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*UpdateAccountAuth](t, dataHeader, dataBody)
		principal := new(TokenAccount)
		principal.AddAuthority(AccountUrl("foo"))
		principal.AddAuthority(AccountUrl("baz"))
		principal.Url = txn.Header.Principal
		principal.Balance = *big.NewInt(1e12)
		fuzzutil.ValidateTransaction(t, txn, chain.UpdateAccountAuth{}, h[txn.ID().Hash()], principal)
	})
}

func FuzzUpdateKey(f *testing.F) {
	existingKey := acctesting.GenerateKey()
	h := map[[32]byte]bool{}
	fuzzutil.AddTransaction(f, h, TransactionHeader{Principal: AccountUrl("foo", "1")}, &UpdateKey{
		NewKeyHash: make([]byte, 32)})

	f.Fuzz(func(t *testing.T, dataHeader, dataBody []byte) {
		t.Parallel()
		txn, _ := fuzzutil.UnpackTransaction[*UpdateKey](t, dataHeader, dataBody)
		parent, ok := txn.Header.Principal.Parent()
		if !ok {
			t.Skip()
		}

		principal := new(KeyPage)
		principal.Url = txn.Header.Principal
		principal.AddKeySpec(&KeySpec{PublicKeyHash: helpers.Hash(existingKey[32:])})
		book := new(KeyBook)
		book.Url = parent
		book.AddAuthority(book.Url)

		sig, err := new(signing.Builder).
			SetType(SignatureTypeED25519).
			SetUrl(principal.Url).
			SetVersion(1).
			SetPrivateKey(existingKey).
			SetTimestamp(1).
			UseSimpleHash().
			Initiate(txn)
		require.NoError(t, err)

		db := database.OpenInMemory(nil)
		Update(t, db, func(batch *database.Batch) {
			_, err := batch.Transaction(txn.GetHash()).AddSignature(0, sig)
			require.NoError(t, err)
		})

		fuzzutil.ValidateTransactionDb(t, db, txn, chain.UpdateKey{}, h[txn.ID().Hash()], book, principal)
	})
}
