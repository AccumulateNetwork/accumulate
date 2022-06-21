package pkg

import (
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Session) AcmeAmount(v float64) *big.Int {
	return s.Amount(v, protocol.AcmePrecision)
}

func (s *Session) Amount(v float64, precision uint64) *big.Int {
	p := new(big.Float)
	p.SetUint64(precision)
	p.Mul(p, big.NewFloat(v))
	a, _ := p.Int(nil)
	return a
}

func (b bldTxn) precisionFromPrincipal() int {
	switch principal := b.s.GetAccount(b.transaction.Header.Principal).(type) {
	case protocol.AccountWithTokens:
		var issuer *protocol.TokenIssuer
		b.s.GetAccountAs(principal.GetTokenUrl(), &issuer)
		return int(issuer.Precision)
	case *protocol.TokenIssuer:
		return int(principal.Precision)
	default:
		b.s.Abortf("Cannot get precision from account type %v", principal.Type())
		panic("unreachable")
	}
}

type bldKeyEntry struct {
	s *Session
	*protocol.KeySpecParams
}

func (s *Session) KeyEntry() bldKeyEntry {
	var b bldKeyEntry
	b.s = s
	b.KeySpecParams = new(protocol.KeySpecParams)
	return b
}

func (b bldKeyEntry) WithOwner(owner Urlish) bldKeyEntry {
	b.Delegate = b.s.url(owner)
	return b
}

func (b bldKeyEntry) WithHash(hash []byte) bldKeyEntry {
	b.KeyHash = hash
	return b
}

func (b bldKeyEntry) WithKey(key Keyish) bldKeyEntry {
	b.KeyHash = b.s.pubkeyhash(key)
	return b
}

type (
	bldCreateIdentity struct {
		bldTxn
		body *protocol.CreateIdentity
	}

	bldCreateTokenAccount struct {
		bldTxn
		body *protocol.CreateTokenAccount
	}

	bldSendTokens struct {
		bldTxn
		body *protocol.SendTokens
	}

	bldCreateDataAccount struct {
		bldTxn
		body *protocol.CreateDataAccount
	}

	bldWriteData struct {
		bldTxn
		body *protocol.WriteData
	}

	bldWriteDataTo struct {
		bldTxn
		body *protocol.WriteDataTo
	}

	bldCreateToken struct {
		bldTxn
		body *protocol.CreateToken
	}

	bldIssueTokens struct {
		bldTxn
		body *protocol.IssueTokens
	}

	bldBurnTokens struct {
		bldTxn
		body *protocol.BurnTokens
	}

	bldCreateKeyPage struct {
		bldTxn
		body *protocol.CreateKeyPage
	}

	bldCreateKeyBook struct {
		bldTxn
		body *protocol.CreateKeyBook
	}

	bldAddCredits struct {
		bldTxn
		body *protocol.AddCredits
	}

	bldUpdateKeyPage struct {
		bldTxn
		body *protocol.UpdateKeyPage
	}

	bldUpdateAccountAuth struct {
		bldTxn
		body *protocol.UpdateAccountAuth
	}

	bldUpdateKey struct {
		bldTxn
		body *protocol.UpdateKey
	}
)

func (b bldTxn) CreateIdentity(url Urlish) bldCreateIdentity {
	var c bldCreateIdentity
	c.body = new(protocol.CreateIdentity)
	c.bldTxn = b.WithBody(c.body)
	c.body.Url = b.s.url(url)
	return c
}

func (b bldCreateIdentity) WithKey(key Keyish) bldCreateIdentity {
	return b.WithKeyHash(b.s.pubkeyhash(key))
}

func (b bldCreateIdentity) WithKeyHash(hash []byte) bldCreateIdentity {
	b.body.KeyHash = hash
	return b
}

func (b bldCreateIdentity) WithKeyBook(book Urlish) bldCreateIdentity {
	b.body.KeyBookUrl = b.s.url(book)
	return b
}

func (b bldCreateIdentity) WithAuthority(book Urlish) bldCreateIdentity {
	b.body.Authorities = append(b.body.Authorities, b.s.url(book))
	return b
}

func (b bldTxn) CreateTokenAccount(url, token Urlish) bldCreateTokenAccount {
	var c bldCreateTokenAccount
	c.body = new(protocol.CreateTokenAccount)
	c.bldTxn = b.WithBody(c.body)
	c.body.Url = b.s.url(url)
	c.body.TokenUrl = b.s.url(token)
	return c
}

func (b bldCreateTokenAccount) AsScratch() bldCreateTokenAccount {
	b.body.Scratch = true
	return b
}

func (b bldCreateTokenAccount) WithAuthority(book Urlish) bldCreateTokenAccount {
	b.body.Authorities = append(b.body.Authorities, b.s.url(book))
	return b
}

func (b bldTxn) SendTokens() bldSendTokens {
	var c bldSendTokens
	c.body = new(protocol.SendTokens)
	c.bldTxn = b.WithBody(c.body)
	return c
}

func (b bldSendTokens) To(recipient Urlish, amount Numish) bldSendTokens {
	b.body.To = append(b.body.To, &protocol.TokenRecipient{
		Url:    b.s.url(recipient),
		Amount: *b.s.bigint(amount, b.precisionFromPrincipal()),
	})
	return b
}

func (b bldSendTokens) AndTo(recipient Urlish, amount Numish) bldSendTokens {
	return b.To(recipient, amount)
}

func (b bldTxn) CreateDataAccount() bldCreateDataAccount {
	var c bldCreateDataAccount
	c.body = new(protocol.CreateDataAccount)
	c.bldTxn = b.WithBody(c.body)
	return c
}

func (b bldCreateDataAccount) AsScratch() bldCreateDataAccount {
	b.body.Scratch = true
	return b
}

func (b bldCreateDataAccount) WithAuthority(book Urlish) bldCreateDataAccount {
	b.body.Authorities = append(b.body.Authorities, b.s.url(book))
	return b
}

func (b bldTxn) WriteData(data ...[]byte) bldWriteData {
	var c bldWriteData
	c.body = new(protocol.WriteData)
	c.bldTxn = b.WithBody(c.body)
	c.body.Entry = &protocol.AccumulateDataEntry{Data: data}
	return c
}

func (b bldTxn) WriteDataTo(recipient Urlish, data ...[]byte) bldWriteDataTo {
	var c bldWriteDataTo
	c.body = new(protocol.WriteDataTo)
	c.bldTxn = b.WithBody(c.body)
	c.body.Recipient = b.s.url(recipient)
	c.body.Entry = &protocol.AccumulateDataEntry{Data: data}
	return c
}

func (b bldTxn) CreateToken(url Urlish, symbol string, precision uint64) bldCreateToken {
	var c bldCreateToken
	c.body = new(protocol.CreateToken)
	c.bldTxn = b.WithBody(c.body)
	c.body.Url = b.s.url(url)
	c.body.Symbol = symbol
	c.body.Precision = precision
	return c
}

func (b bldCreateToken) WithAuthority(book Urlish) bldCreateToken {
	b.body.Authorities = append(b.body.Authorities, b.s.url(book))
	return b
}

func (b bldCreateToken) WithSupplyLimit(limit Numish) bldCreateToken {
	b.body.SupplyLimit = b.s.bigint(limit, int(b.body.Precision))
	return b
}

func (b bldTxn) IssueTokens(recipient Urlish, amount Numish) bldIssueTokens {
	var c bldIssueTokens
	c.body = new(protocol.IssueTokens)
	c.bldTxn = b.WithBody(c.body)
	c.body.Recipient = b.s.url(recipient)
	c.body.Amount = *b.s.bigint(amount, b.precisionFromPrincipal())
	return c
}

func (b bldTxn) BurnTokens(amount Numish) bldBurnTokens {
	var c bldBurnTokens
	c.body = new(protocol.BurnTokens)
	c.bldTxn = b.WithBody(c.body)
	c.body.Amount = *b.s.bigint(amount, b.precisionFromPrincipal())
	return c
}

func (b bldTxn) CreateKeyPage() bldCreateKeyPage {
	var c bldCreateKeyPage
	c.body = new(protocol.CreateKeyPage)
	c.bldTxn = b.WithBody(c.body)
	return c
}

func (b bldCreateKeyPage) WithEntry(e bldKeyEntry) bldCreateKeyPage {
	b.body.Keys = append(b.body.Keys, e.KeySpecParams)
	return b
}

func (b bldCreateKeyPage) AndEntry(e bldKeyEntry) bldCreateKeyPage {
	return b.WithEntry(e)
}

func (b bldTxn) CreateKeyBook(url Urlish) bldCreateKeyBook {
	var c bldCreateKeyBook
	c.body = new(protocol.CreateKeyBook)
	c.bldTxn = b.WithBody(c.body)
	c.body.Url = b.s.url(url)
	return c
}

func (b bldCreateKeyBook) WithAuthority(book Urlish) bldCreateKeyBook {
	b.body.Authorities = append(b.body.Authorities, b.s.url(book))
	return b
}

func (b bldCreateKeyBook) WithKeyHash(hash []byte) bldCreateKeyBook {
	b.body.PublicKeyHash = hash
	return b
}

func (b bldCreateKeyBook) WithKey(key Keyish) bldCreateKeyBook {
	return b.WithKeyHash(b.s.pubkeyhash(key))
}

func (b bldTxn) AddCredits(url Urlish, spend float64) bldAddCredits {
	var c bldAddCredits
	c.body = new(protocol.AddCredits)
	c.bldTxn = b.WithBody(c.body)
	c.body.Recipient = b.s.url(url)
	c.body.Amount = *b.s.Amount(spend, protocol.AcmePrecision)
	return c
}

func (b bldAddCredits) Oracle(value float64) bldAddCredits {
	b.body.Oracle = b.s.Amount(value, protocol.CreditPrecision).Uint64()
	return b
}

func (b bldTxn) UpdateKeyPage() bldUpdateKeyPage {
	var c bldUpdateKeyPage
	c.body = new(protocol.UpdateKeyPage)
	c.bldTxn = b.WithBody(c.body)
	return c
}

func (b bldUpdateKeyPage) Add(e bldKeyEntry) bldUpdateKeyPage {
	op := new(protocol.AddKeyOperation)
	op.Entry = *e.KeySpecParams
	b.body.Operation = append(b.body.Operation, op)
	return b
}

type bldUpdateKeyOperation struct {
	bldUpdateKeyPage
	*protocol.UpdateKeyOperation
}

func (b bldUpdateKeyPage) Update(e bldKeyEntry) bldUpdateKeyOperation {
	op := new(protocol.UpdateKeyOperation)
	op.OldEntry = *e.KeySpecParams
	b.body.Operation = append(b.body.Operation, op)
	return bldUpdateKeyOperation{b, op}
}

func (b bldUpdateKeyOperation) To(e bldKeyEntry) bldUpdateKeyPage {
	b.NewEntry = *e.KeySpecParams
	return b.bldUpdateKeyPage
}

func (b bldUpdateKeyPage) Remove(e bldKeyEntry) bldUpdateKeyPage {
	op := new(protocol.RemoveKeyOperation)
	op.Entry = *e.KeySpecParams
	b.body.Operation = append(b.body.Operation, op)
	return b
}

func (b bldUpdateKeyPage) SetThreshold(v uint64) bldUpdateKeyPage {
	op := new(protocol.SetThresholdKeyPageOperation)
	op.Threshold = v
	b.body.Operation = append(b.body.Operation, op)
	return b
}

type bldUpdateAllowedKeyPageOperation struct {
	bldUpdateKeyPage
	*protocol.UpdateAllowedKeyPageOperation
}

func (b bldUpdateKeyPage) UpdateAllowed() bldUpdateAllowedKeyPageOperation {
	op := new(protocol.UpdateAllowedKeyPageOperation)
	b.body.Operation = append(b.body.Operation, op)
	return bldUpdateAllowedKeyPageOperation{b, op}
}

func (b bldUpdateAllowedKeyPageOperation) Allow(typ protocol.TransactionType) bldUpdateAllowedKeyPageOperation {
	b.UpdateAllowedKeyPageOperation.Allow = append(b.UpdateAllowedKeyPageOperation.Allow, typ)
	return b
}

func (b bldUpdateAllowedKeyPageOperation) Deny(typ protocol.TransactionType) bldUpdateAllowedKeyPageOperation {
	b.UpdateAllowedKeyPageOperation.Deny = append(b.UpdateAllowedKeyPageOperation.Deny, typ)
	return b
}

func (b bldTxn) UpdateAccountAuth() bldUpdateAccountAuth {
	var c bldUpdateAccountAuth
	c.body = new(protocol.UpdateAccountAuth)
	c.bldTxn = b.WithBody(c.body)
	return c
}

func (b bldUpdateAccountAuth) Enable(authority Urlish) bldUpdateAccountAuth {
	op := new(protocol.EnableAccountAuthOperation)
	op.Authority = b.s.url(authority)
	return b
}

func (b bldUpdateAccountAuth) Disable(authority Urlish) bldUpdateAccountAuth {
	op := new(protocol.DisableAccountAuthOperation)
	op.Authority = b.s.url(authority)
	return b
}

func (b bldUpdateAccountAuth) Add(authority Urlish) bldUpdateAccountAuth {
	op := new(protocol.AddAccountAuthorityOperation)
	op.Authority = b.s.url(authority)
	return b
}

func (b bldUpdateAccountAuth) Remove(authority Urlish) bldUpdateAccountAuth {
	op := new(protocol.RemoveAccountAuthorityOperation)
	op.Authority = b.s.url(authority)
	return b
}

func (b bldTxn) UpdateKey(newKey Keyish) bldUpdateKey {
	var c bldUpdateKey
	c.body = new(protocol.UpdateKey)
	c.bldTxn = b.WithBody(c.body)
	c.body.NewKeyHash = b.s.pubkeyhash(newKey)
	return c
}
