// Copyright 2023 The Accumulate Authors
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

func (b TransactionBuilder) For(principal any, path ...string) TransactionBuilder {
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

func (b TransactionBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return SignatureBuilder{parser: b.parser, transaction: &b.t}.Url(signer, path...)
}

type CreateIdentityBuilder struct {
	t    TransactionBuilder
	body protocol.CreateIdentity
}

func (b TransactionBuilder) CreateIdentity(url any, path ...string) CreateIdentityBuilder {
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

func (b CreateIdentityBuilder) WithKeyBook(book any, path ...string) CreateIdentityBuilder {
	b.body.KeyBookUrl = b.t.parseUrl(book, path...)
	return b
}

func (b CreateIdentityBuilder) WithAuthority(book any, path ...string) CreateIdentityBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateIdentityBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateIdentityBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateTokenAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateTokenAccount
}

func (b TransactionBuilder) CreateTokenAccount(url any, path ...string) CreateTokenAccountBuilder {
	c := CreateTokenAccountBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateTokenAccountBuilder) ForToken(token any, path ...string) CreateTokenAccountBuilder {
	b.body.TokenUrl = b.t.parseUrl(token, path...)
	return b
}

func (b CreateTokenAccountBuilder) WithAuthority(book any, path ...string) CreateTokenAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateTokenAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateTokenAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateLiteTokenAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateLiteTokenAccount
}

func (b TransactionBuilder) CreateLiteTokenAccount() CreateLiteTokenAccountBuilder {
	c := CreateLiteTokenAccountBuilder{t: b}
	return c
}

func (b CreateLiteTokenAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateLiteTokenAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b SendTokensBuilder) To(recipient any, path ...string) SendTokensBuilder {
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

func (b SendTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateDataAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateDataAccount
}

func (b TransactionBuilder) CreateDataAccount(url any, path ...string) CreateDataAccountBuilder {
	c := CreateDataAccountBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateDataAccountBuilder) WithAuthority(book any, path ...string) CreateDataAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book, path...))
	return b
}

func (b CreateDataAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateDataAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b WriteDataBuilder) Entry(e protocol.DataEntry) WriteDataBuilder {
	b.body.Entry = e
	return b
}

func (b WriteDataBuilder) Scratch() WriteDataBuilder {
	b.body.Scratch = true
	return b
}

func (b WriteDataBuilder) ToState() WriteDataBuilder {
	b.body.WriteToState = true
	return b
}

func (b WriteDataBuilder) To(recipient any, path ...string) WriteDataToBuilder {
	c := WriteDataToBuilder{t: b.t}
	c.body.Entry = b.body.Entry
	c.body.Recipient = c.t.parseUrl(recipient, path...)
	return c
}

func (b WriteDataBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b WriteDataBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type WriteDataToBuilder struct {
	t    TransactionBuilder
	body protocol.WriteDataTo
}

func (b WriteDataToBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b WriteDataToBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateTokenBuilder struct {
	t    TransactionBuilder
	body protocol.CreateToken
}

func (b TransactionBuilder) CreateToken(url any, path ...string) CreateTokenBuilder {
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

func (b CreateTokenBuilder) WithAuthority(book any, path ...string) CreateTokenBuilder {
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

func (b CreateTokenBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b IssueTokensBuilder) To(recipient any, path ...string) IssueTokensBuilder {
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

func (b IssueTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b BurnTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b CreateKeyPageBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type CreateKeyBookBuilder struct {
	t    TransactionBuilder
	body protocol.CreateKeyBook
}

func (b CreateKeyBookBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b CreateKeyBookBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

func (b TransactionBuilder) CreateKeyBook(url any, path ...string) CreateKeyBookBuilder {
	c := CreateKeyBookBuilder{t: b}
	c.body.Url = c.t.parseUrl(url, path...)
	return c
}

func (b CreateKeyBookBuilder) WithAuthority(book any, path ...string) CreateKeyBookBuilder {
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

func (b TransactionBuilder) AddCredits() AddCreditsBuilder {
	c := AddCreditsBuilder{t: b}
	return c
}

func (b AddCreditsBuilder) WithOracle(value float64) AddCreditsBuilder {
	b.body.Oracle = b.t.parseAmount(value, protocol.AcmeOraclePrecisionPower).Uint64()
	return b
}

func (b AddCreditsBuilder) Spend(amount float64) AddCreditsBuilder {
	b.body.Amount = *b.t.parseAmount(amount, protocol.AcmePrecisionPower)
	return b
}

func (b AddCreditsBuilder) Purchase(amount float64) AddCreditsBuilder {
	// ACME = credits ÷ oracle ÷ credits-per-dollar
	x := big.NewRat(b.t.parseAmount(amount, protocol.CreditPrecisionPower).Int64(), protocol.CreditPrecision)
	x.Quo(x, big.NewRat(int64(b.body.Oracle), protocol.AcmeOraclePrecision))
	x.Quo(x, big.NewRat(protocol.CreditsPerDollar, 1))

	// Convert rational to an ACME balance
	x.Mul(x, big.NewRat(protocol.AcmePrecision, 1))
	y := x.Num()
	if !x.IsInt() {
		y.Div(y, x.Denom())
	}

	b.body.Amount = *y
	return b
}

func (b AddCreditsBuilder) To(url any, path ...string) AddCreditsBuilder {
	b.body.Recipient = b.t.parseUrl(url, path...)
	return b
}

func (b AddCreditsBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b AddCreditsBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b UpdateKeyPageBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type UpdateAccountAuthBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateAccountAuth
}

func (b TransactionBuilder) UpdateAccountAuth() UpdateAccountAuthBuilder {
	return UpdateAccountAuthBuilder{t: b}
}

func (b UpdateAccountAuthBuilder) Enable(authority any, path ...string) UpdateAccountAuthBuilder {
	op := new(protocol.EnableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	b.body.Operations = append(b.body.Operations, op)
	return b
}

func (b UpdateAccountAuthBuilder) Disable(authority any, path ...string) UpdateAccountAuthBuilder {
	op := new(protocol.DisableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	b.body.Operations = append(b.body.Operations, op)
	return b
}

func (b UpdateAccountAuthBuilder) Add(authority any, path ...string) UpdateAccountAuthBuilder {
	op := new(protocol.AddAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	b.body.Operations = append(b.body.Operations, op)
	return b
}

func (b UpdateAccountAuthBuilder) Remove(authority any, path ...string) UpdateAccountAuthBuilder {
	op := new(protocol.RemoveAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority, path...)
	b.body.Operations = append(b.body.Operations, op)
	return b
}

func (b UpdateAccountAuthBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b UpdateAccountAuthBuilder) SignWith(signer any, path ...string) SignatureBuilder {
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

func (b UpdateKeyBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type ActivateProtocolVersionBuilder struct {
	t    TransactionBuilder
	body protocol.ActivateProtocolVersion
}

func (b TransactionBuilder) ActivateProtocolVersion(version protocol.ExecutorVersion) ActivateProtocolVersionBuilder {
	c := ActivateProtocolVersionBuilder{t: b}
	c.body.Version = version
	return c
}

func (b ActivateProtocolVersionBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b ActivateProtocolVersionBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}

type LockAccountBuilder struct {
	t    TransactionBuilder
	body protocol.LockAccount
}

func (b TransactionBuilder) LockAccount(height uint64) LockAccountBuilder {
	c := LockAccountBuilder{t: b}
	c.body.Height = height
	return c
}

func (b LockAccountBuilder) Done() (*protocol.Transaction, error) {
	return b.t.Body(&b.body).Done()
}

func (b LockAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.Body(&b.body).SignWith(signer, path...)
}
