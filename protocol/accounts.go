// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Account interface {
	encoding.BinaryValue
	Type() AccountType
	GetUrl() *url.URL
	StripUrl()
}

type FullAccount interface {
	Account
	GetAuth() *AccountAuth
}

func (a *UnknownAccount) GetUrl() *url.URL { return a.Url }

func (a *LiteDataAccount) GetUrl() *url.URL  { return a.Url }
func (a *LiteIdentity) GetUrl() *url.URL     { return a.Url }
func (a *LiteTokenAccount) GetUrl() *url.URL { return a.Url }

func (a *ADI) GetUrl() *url.URL             { return a.Url }
func (a *AnchorLedger) GetUrl() *url.URL    { return a.Url }
func (a *DataAccount) GetUrl() *url.URL     { return a.Url }
func (a *SystemLedger) GetUrl() *url.URL    { return a.Url }
func (a *BlockLedger) GetUrl() *url.URL     { return a.Url }
func (a *KeyBook) GetUrl() *url.URL         { return a.Url }
func (a *KeyPage) GetUrl() *url.URL         { return a.Url }
func (a *TokenAccount) GetUrl() *url.URL    { return a.Url }
func (a *TokenIssuer) GetUrl() *url.URL     { return a.Url }
func (a *SyntheticLedger) GetUrl() *url.URL { return a.Url }

func (a *UnknownAccount) StripUrl()   { a.Url = a.Url.StripExtras() }
func (a *LiteDataAccount) StripUrl()  { a.Url = a.Url.StripExtras() }
func (a *LiteIdentity) StripUrl()     { a.Url = a.Url.StripExtras() }
func (a *LiteTokenAccount) StripUrl() { a.Url = a.Url.StripExtras() }
func (a *ADI) StripUrl()              { a.Url = a.Url.StripExtras() }
func (a *AnchorLedger) StripUrl()     { a.Url = a.Url.StripExtras() }
func (a *DataAccount) StripUrl()      { a.Url = a.Url.StripExtras() }
func (a *SystemLedger) StripUrl()     { a.Url = a.Url.StripExtras() }
func (a *BlockLedger) StripUrl()      { a.Url = a.Url.StripExtras() }
func (a *KeyBook) StripUrl()          { a.Url = a.Url.StripExtras() }
func (a *KeyPage) StripUrl()          { a.Url = a.Url.StripExtras() }
func (a *TokenAccount) StripUrl()     { a.Url = a.Url.StripExtras() }
func (a *TokenIssuer) StripUrl()      { a.Url = a.Url.StripExtras() }
func (a *SyntheticLedger) StripUrl()  { a.Url = a.Url.StripExtras() }

func (a *ADI) GetAuth() *AccountAuth          { return &a.AccountAuth }
func (a *DataAccount) GetAuth() *AccountAuth  { return &a.AccountAuth }
func (a *KeyBook) GetAuth() *AccountAuth      { return &a.AccountAuth }
func (a *TokenAccount) GetAuth() *AccountAuth { return &a.AccountAuth }
func (a *TokenIssuer) GetAuth() *AccountAuth  { return &a.AccountAuth }

// KeyBook is a backwards compatability shim for the API
func (a *KeyPage) KeyBook() *url.URL {
	if a.Url == nil {
		return nil
	}
	book, _, _ := ParseKeyPageUrl(a.Url)
	return book
}
