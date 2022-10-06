// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TransactionBuilder struct {
	parser
	t protocol.Transaction
}

func (b TransactionBuilder) For(principal any, path ...any) TransactionBuilder {
	b.t.Header.Principal = b.parseUrl(principal, path...)
	return b
}

func (b TransactionBuilder) Initiator(init any) TransactionBuilder {
	b.t.Header.Initiator = b.parseHash32(init)
	return b
}

func (b TransactionBuilder) Memo(memo string) TransactionBuilder {
	b.t.Header.Memo = memo
	return b
}

func (b TransactionBuilder) Metadata(metadata []byte) TransactionBuilder {
	b.t.Header.Metadata = metadata
	return b
}

func (b TransactionBuilder) Body(body protocol.TransactionBody) TransactionBuilder {
	b.t.Body = body
	return b
}

func (b TransactionBuilder) Done() (*protocol.Transaction, error) {
	if b.ok() {
		return &b.t, nil
	}
	return nil, b.err()
}

func (b TransactionBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return SignatureBuilder{parser: b.parser, transaction: &b.t}.Url(signer, path...)
}

type CreateIdentityBuilder struct {
	t    TransactionBuilder
	body protocol.CreateIdentity
}

func (b TransactionBuilder) CreateIdentity(url any, path ...any) CreateIdentityBuilder {
	c := CreateIdentityBuilder{t: b}
	c.body.Url = b.parseUrl(url, path...)
	return c
}

func (b CreateIdentityBuilder) WithKey(key any, typ protocol.SignatureType) CreateIdentityBuilder {
	return b.WithKeyHash(b.t.hashKey(b.t.parsePublicKey(key), typ))
}

func (b CreateIdentityBuilder) WithKeyHash(hash any) CreateIdentityBuilder {
	b.body.KeyHash = b.t.parseHash(hash)
	return b
}

func (b CreateIdentityBuilder) WithKeyBook(book any, path ...any) CreateIdentityBuilder {
	b.body.KeyBookUrl = b.t.parseUrl(book, path...)
	return b
}

func (b CreateIdentityBuilder) WithAuthority(book any, path ...any) CreateIdentityBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateIdentityBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateIdentityBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateTokenAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateTokenAccount
}

func (b TransactionBuilder) CreateTokenAccount(url any, path ...any) CreateTokenAccountBuilder {
	c := CreateTokenAccountBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateTokenAccountBuilder) ForToken(token any, path ...any) CreateTokenAccountBuilder {
	b.body.TokenUrl = b.t.parseUrl(token, path...)
	return b
}

func (b CreateTokenAccountBuilder) WithAuthority(book any, path ...any) CreateTokenAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateTokenAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateTokenAccountBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type SendTokensBuilder struct {
	t      TransactionBuilder
	amount big.Int
	body   protocol.SendTokens
}

func (b TransactionBuilder) SendTokens(amount any, precision uint64) SendTokensBuilder {
	return SendTokensBuilder{t: b}.And(amount, precision)
}

func (b SendTokensBuilder) To(recipient any, path ...any) SendTokensBuilder {
	b.body.To = append(b.body.To, &protocol.TokenRecipient{
		Url:    b.t.parseUrl(recipient, path...),
		Amount: b.amount,
	})
	return b
}

func (b SendTokensBuilder) And(amount any, precision uint64) SendTokensBuilder {
	b.amount = *b.t.parseAmount(amount, precision)
	return b
}

func (b SendTokensBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b SendTokensBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateDataAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateDataAccount
}

func (b TransactionBuilder) CreateDataAccount() CreateDataAccountBuilder {
	return CreateDataAccountBuilder{t: b}
}

func (b CreateDataAccountBuilder) WithAuthority(book any, path ...any) CreateDataAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateDataAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateDataAccountBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type WriteDataBuilder struct {
	t    TransactionBuilder
	body protocol.WriteData
}

func (b TransactionBuilder) WriteData(data ...[]byte) WriteDataBuilder {
	c := WriteDataBuilder{t: b}
	c.body.Entry = &protocol.AccumulateDataEntry{Data: data}
	return c
}

func (b WriteDataBuilder) Scratch() WriteDataBuilder {
	b.body.Scratch = true
	return b
}

func (b WriteDataBuilder) ToState() WriteDataBuilder {
	b.body.WriteToState = true
	return b
}

func (b WriteDataBuilder) To(recipient any, path ...any) WriteDataToBuilder {
	c := WriteDataToBuilder{t: b.t}
	c.body.Entry = b.body.Entry
	c.body.Recipient = c.t.parseUrl(recipient, path...)
	return c
}

func (b WriteDataBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b WriteDataBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type WriteDataToBuilder struct {
	t    TransactionBuilder
	body protocol.WriteDataTo
}

func (b WriteDataToBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b WriteDataToBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateTokenBuilder struct {
	t    TransactionBuilder
	body protocol.CreateToken
}

func (b TransactionBuilder) CreateToken(url any, path ...any) CreateTokenBuilder {
	c := CreateTokenBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateTokenBuilder) WithSymbol(symbol string) CreateTokenBuilder {
	b.body.Symbol = symbol
	return b
}

func (b CreateTokenBuilder) WithPrecision(precision any) CreateTokenBuilder {
	b.body.Precision = b.t.parseUint(precision)
	return b
}

func (b CreateTokenBuilder) WithAuthority(book any, path ...any) CreateTokenBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateTokenBuilder) WithSupplyLimit(limit any) CreateTokenBuilder {
	b.body.SupplyLimit = b.t.parseAmount(limit, b.body.Precision)
	return b
}

func (b CreateTokenBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateTokenBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type IssueTokensBuilder struct {
	t      TransactionBuilder
	amount big.Int
	body   protocol.IssueTokens
}

func (b TransactionBuilder) IssueTokens(amount any, precision uint64) IssueTokensBuilder {
	return IssueTokensBuilder{t: b}.And(amount, precision)
}

func (b IssueTokensBuilder) To(recipient any, path ...any) IssueTokensBuilder {
	b.body.To = append(b.body.To, &protocol.TokenRecipient{
		Url:    b.t.parseUrl(recipient, path...),
		Amount: b.amount,
	})
	return b
}

func (b IssueTokensBuilder) And(amount any, precision uint64) IssueTokensBuilder {
	b.amount = *b.t.parseAmount(amount, precision)
	return b
}

func (b IssueTokensBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b IssueTokensBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type BurnTokensBuilder struct {
	t    TransactionBuilder
	body protocol.BurnTokens
}

func (b TransactionBuilder) BurnTokens(amount any, precision uint64) BurnTokensBuilder {
	c := BurnTokensBuilder{t: b}
	c.body.Amount = *c.t.parseAmount(amount, precision)
	return c
}

func (b BurnTokensBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b BurnTokensBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateKeyPageBuilder struct {
	t    TransactionBuilder
	body protocol.CreateKeyPage
}

func (b TransactionBuilder) CreateKeyPage() CreateKeyPageBuilder {
	return CreateKeyPageBuilder{t: b}
}

func (b CreateKeyPageBuilder) WithEntry() KeyPageEntryBuilder[CreateKeyPageBuilder] {
	return KeyPageEntryBuilder[CreateKeyPageBuilder]{t: b}
}

func (b CreateKeyPageBuilder) addEntry(entry protocol.KeySpecParams, err []error) CreateKeyPageBuilder {
	b.t.record(err...)
	b.body.Keys = append(b.body.Keys, &entry)
	return b
}

func (b CreateKeyPageBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateKeyPageBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateKeyBookBuilder struct {
	t    TransactionBuilder
	body protocol.CreateKeyBook
}

func (b CreateKeyBookBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateKeyBookBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

func (b TransactionBuilder) CreateKeyBook(url any, path ...any) CreateKeyBookBuilder {
	c := CreateKeyBookBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateKeyBookBuilder) WithAuthority(book any, path ...any) CreateKeyBookBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateKeyBookBuilder) WithKeyHash(hash any) CreateKeyBookBuilder {
	b.body.PublicKeyHash = b.t.parseHash(hash)
	return b
}

func (b CreateKeyBookBuilder) WithKey(key any, typ protocol.SignatureType) CreateKeyBookBuilder {
	return b.WithKeyHash(b.t.hashKey(b.t.parsePublicKey(key), typ))
}

type AddCreditsBuilder struct {
	t    TransactionBuilder
	body protocol.AddCredits
}

func (b TransactionBuilder) AddCredits(spend float64) AddCreditsBuilder {
	c := AddCreditsBuilder{t: b}
	c.body.Amount = *c.t.parseAmount(spend, protocol.AcmePrecision)
	return c
}

func (b AddCreditsBuilder) To(url any, path ...any) AddCreditsBuilder {
	b.body.Recipient = b.t.parseUrl(url, path...)
	return b
}

func (b AddCreditsBuilder) Oracle(value float64) AddCreditsBuilder {
	b.body.Oracle = b.t.parseAmount(value, protocol.CreditPrecision).Uint64()
	return b
}

func (b AddCreditsBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b AddCreditsBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type UpdateKeyPageBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateKeyPage
}

func (b TransactionBuilder) UpdateKeyPage() UpdateKeyPageBuilder {
	return UpdateKeyPageBuilder{t: b}
}

func (b UpdateKeyPageBuilder) Add() AddKeyOperationBuilder {
	return AddKeyOperationBuilder{b: b}
}

func (b UpdateKeyPageBuilder) Update() UpdateKeyOperationBuilder {
	return UpdateKeyOperationBuilder{b: b}
}

func (b UpdateKeyPageBuilder) Remove() RemoveKeyOperationBuilder {
	return RemoveKeyOperationBuilder{b: b}
}

func (b UpdateKeyPageBuilder) SetThreshold(v uint64) UpdateKeyPageBuilder {
	op := new(protocol.SetThresholdKeyPageOperation)
	op.Threshold = v
	b.body.Operation = append(b.body.Operation, op)
	return b
}

func (b UpdateKeyPageBuilder) UpdateAllowed() UpdateAllowedKeyPageOperationBuilder {
	return UpdateAllowedKeyPageOperationBuilder{b: b}
}

func (b UpdateKeyPageBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b UpdateKeyPageBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type UpdateAccountAuthBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateAccountAuth
}

func (b TransactionBuilder) UpdateAccountAuth() UpdateAccountAuthBuilder {
	return UpdateAccountAuthBuilder{t: b}
}

func (b UpdateAccountAuthBuilder) Enable(authority any, path ...any) UpdateAccountAuthBuilder {
	op := new(protocol.EnableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	return b
}

func (b UpdateAccountAuthBuilder) Disable(authority any, path ...any) UpdateAccountAuthBuilder {
	op := new(protocol.DisableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	return b
}

func (b UpdateAccountAuthBuilder) Add(authority any, path ...any) UpdateAccountAuthBuilder {
	op := new(protocol.AddAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	return b
}

func (b UpdateAccountAuthBuilder) Remove(authority any, path ...any) UpdateAccountAuthBuilder {
	op := new(protocol.RemoveAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	return b
}

func (b UpdateAccountAuthBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b UpdateAccountAuthBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type UpdateKeyBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateKey
}

func (b TransactionBuilder) UpdateKey(newKey any, typ protocol.SignatureType) UpdateKeyBuilder {
	c := UpdateKeyBuilder{t: b}
	c.body.NewKeyHash = c.t.hashKey(c.t.parsePublicKey(newKey), typ)
	return c
}

func (b UpdateKeyBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b UpdateKeyBuilder) SignWith(signer any, path ...any) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}
