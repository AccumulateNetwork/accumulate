package build

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TransactionBuilder struct {
	parser
	t protocol.Transaction
}

func (b TransactionBuilder) WithPrincipal(principal any, path ...string) TransactionBuilder {
	b.t.Header.Principal = b.parseUrl(principal, path...)
	return b
}

func (b TransactionBuilder) WithInitiator(init any) TransactionBuilder {
	b.t.Header.Initiator = b.parseHash32(init)
	return b
}

func (b TransactionBuilder) WithMemo(memo string) TransactionBuilder {
	b.t.Header.Memo = memo
	return b
}

func (b TransactionBuilder) WithMetadata(metadata []byte) TransactionBuilder {
	b.t.Header.Metadata = metadata
	return b
}

func (b TransactionBuilder) WithBody(body protocol.TransactionBody) TransactionBuilder {
	b.t.Body = body
	return b
}

func (b TransactionBuilder) Build() (*protocol.Transaction, error) {
	if b.ok() {
		return &b.t, nil
	}
	return nil, b.err()
}

func (b TransactionBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	if b.ok() {
		return EnvelopeBuilder{transaction: &b.t}.SignWith(signer, path...)
	}

	// Set up a fake transaction
	var c EnvelopeBuilder
	c.transaction = new(protocol.Transaction)
	c.transaction.Header.Principal = protocol.AccountUrl(protocol.Unknown)
	c.transaction.Body = new(protocol.RemoteTransaction)

	// Transfer errors
	c.record(b.errs...)
	return c.SignWith(signer, path...)
}

type CreateIdentityBuilder struct {
	t    TransactionBuilder
	body protocol.CreateIdentity
}

func (b TransactionBuilder) CreateIdentity(url any) CreateIdentityBuilder {
	c := CreateIdentityBuilder{t: b}
	c.body.Url = b.parseUrl(url)
	return c
}

func (b CreateIdentityBuilder) WithKey(key any, typ protocol.SignatureType) CreateIdentityBuilder {
	return b.WithKeyHash(b.t.hashKey(b.t.parsePublicKey(key), typ))
}

func (b CreateIdentityBuilder) WithKeyHash(hash any) CreateIdentityBuilder {
	b.body.KeyHash = b.t.parseHash(hash)
	return b
}

func (b CreateIdentityBuilder) WithKeyBook(book any) CreateIdentityBuilder {
	b.body.KeyBookUrl = b.t.parseUrl(book)
	return b
}

func (b CreateIdentityBuilder) WithAuthority(book any) CreateIdentityBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book))
	return b
}

func (b CreateIdentityBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateIdentityBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type CreateTokenAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateTokenAccount
}

func (b TransactionBuilder) CreateTokenAccount(url, token any) CreateTokenAccountBuilder {
	c := CreateTokenAccountBuilder{t: b}
	c.body.Url = b.parseUrl(url)
	c.body.TokenUrl = b.parseUrl(token)
	return c
}

func (b CreateTokenAccountBuilder) WithAuthority(book any) CreateTokenAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book))
	return b
}

func (b CreateTokenAccountBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateTokenAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type SendTokensBuilder struct {
	t    TransactionBuilder
	body protocol.SendTokens
}

func (b TransactionBuilder) SendTokens() SendTokensBuilder {
	return SendTokensBuilder{t: b}
}

func (b SendTokensBuilder) To(recipient any, amount any, precision uint64) SendTokensBuilder {
	b.body.To = append(b.body.To, &protocol.TokenRecipient{
		Url:    b.t.parseUrl(recipient),
		Amount: *b.t.parseAmount(amount, precision),
	})
	return b
}

func (b SendTokensBuilder) AndTo(recipient any, amount any, precision uint64) SendTokensBuilder {
	return b.To(recipient, amount, precision)
}

func (b SendTokensBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b SendTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type CreateDataAccountBuilder struct {
	t    TransactionBuilder
	body protocol.CreateDataAccount
}

func (b TransactionBuilder) CreateDataAccount() CreateDataAccountBuilder {
	return CreateDataAccountBuilder{t: b}
}

func (b CreateDataAccountBuilder) WithAuthority(book any) CreateDataAccountBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book))
	return b
}

func (b CreateDataAccountBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateDataAccountBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
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

func (b WriteDataBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b WriteDataBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type WriteDataToBuilder struct {
	t    TransactionBuilder
	body protocol.WriteDataTo
}

func (b TransactionBuilder) WriteDataTo(recipient any, data ...[]byte) WriteDataToBuilder {
	c := WriteDataToBuilder{t: b}
	c.body.Recipient = b.parseUrl(recipient)
	c.body.Entry = &protocol.AccumulateDataEntry{Data: data}
	return c
}

func (b WriteDataToBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b WriteDataToBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type CreateTokenBuilder struct {
	t    TransactionBuilder
	body protocol.CreateToken
}

func (b TransactionBuilder) CreateToken(url any, symbol string, precision uint64) CreateTokenBuilder {
	c := CreateTokenBuilder{t: b}
	c.body.Url = b.parseUrl(url)
	c.body.Symbol = symbol
	c.body.Precision = precision
	return c
}

func (b CreateTokenBuilder) WithAuthority(book any) CreateTokenBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book))
	return b
}

func (b CreateTokenBuilder) WithSupplyLimit(limit any) CreateTokenBuilder {
	b.body.SupplyLimit = b.t.parseAmount(limit, b.body.Precision)
	return b
}

func (b CreateTokenBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateTokenBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type IssueTokensBuilder struct {
	t    TransactionBuilder
	body protocol.IssueTokens
}

func (b TransactionBuilder) IssueTokens() IssueTokensBuilder {
	return IssueTokensBuilder{t: b}
}

func (b IssueTokensBuilder) To(recipient any, amount any, precision uint64) IssueTokensBuilder {
	b.body.To = append(b.body.To, &protocol.TokenRecipient{
		Url:    b.t.parseUrl(recipient),
		Amount: *b.t.parseAmount(amount, precision),
	})
	return b
}

func (b IssueTokensBuilder) AndTo(recipient any, amount any, precision uint64) IssueTokensBuilder {
	return b.To(recipient, amount, precision)
}

func (b IssueTokensBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b IssueTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type BurnTokensBuilder struct {
	t    TransactionBuilder
	body protocol.BurnTokens
}

func (b TransactionBuilder) BurnTokens(amount any, precision uint64) BurnTokensBuilder {
	c := BurnTokensBuilder{t: b}
	c.body.Amount = *b.parseAmount(amount, precision)
	return c
}

func (b BurnTokensBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b BurnTokensBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
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

func (b CreateKeyPageBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateKeyPageBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type CreateKeyBookBuilder struct {
	t    TransactionBuilder
	body protocol.CreateKeyBook
}

func (b CreateKeyBookBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b CreateKeyBookBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

func (b TransactionBuilder) CreateKeyBook(url any) CreateKeyBookBuilder {
	c := CreateKeyBookBuilder{t: b}
	c.body.Url = b.parseUrl(url)
	return c
}

func (b CreateKeyBookBuilder) WithAuthority(book any) CreateKeyBookBuilder {
	b.body.Authorities = append(b.body.Authorities, b.t.parseUrl(book))
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

func (b TransactionBuilder) AddCredits(url any, spend float64) AddCreditsBuilder {
	c := AddCreditsBuilder{t: b}
	c.body.Recipient = b.parseUrl(url)
	c.body.Amount = *b.parseAmount(spend, protocol.AcmePrecision)
	return c
}

func (b AddCreditsBuilder) Oracle(value float64) AddCreditsBuilder {
	b.body.Oracle = b.t.parseAmount(value, protocol.CreditPrecision).Uint64()
	return b
}

func (b AddCreditsBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b AddCreditsBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
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

func (b UpdateKeyPageBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b UpdateKeyPageBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type UpdateAccountAuthBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateAccountAuth
}

func (b TransactionBuilder) UpdateAccountAuth() UpdateAccountAuthBuilder {
	return UpdateAccountAuthBuilder{t: b}
}

func (b UpdateAccountAuthBuilder) Enable(authority any) UpdateAccountAuthBuilder {
	op := new(protocol.EnableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority)
	return b
}

func (b UpdateAccountAuthBuilder) Disable(authority any) UpdateAccountAuthBuilder {
	op := new(protocol.DisableAccountAuthOperation)
	op.Authority = b.t.parseUrl(authority)
	return b
}

func (b UpdateAccountAuthBuilder) Add(authority any) UpdateAccountAuthBuilder {
	op := new(protocol.AddAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority)
	return b
}

func (b UpdateAccountAuthBuilder) Remove(authority any) UpdateAccountAuthBuilder {
	op := new(protocol.RemoveAccountAuthorityOperation)
	op.Authority = b.t.parseUrl(authority)
	return b
}

func (b UpdateAccountAuthBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b UpdateAccountAuthBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}

type UpdateKeyBuilder struct {
	t    TransactionBuilder
	body protocol.UpdateKey
}

func (b TransactionBuilder) UpdateKey(newKey any, typ protocol.SignatureType) UpdateKeyBuilder {
	c := UpdateKeyBuilder{t: b}
	c.body.NewKeyHash = b.hashKey(b.parsePublicKey(newKey), typ)
	return c
}

func (b UpdateKeyBuilder) Build() (*protocol.Transaction, error) {
	return b.t.WithBody(&b.body).Build()
}

func (b UpdateKeyBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.t.WithBody(&b.body).SignWith(signer, path...)
}
