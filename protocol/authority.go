// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Authority interface {
	Account
	GetSigners() []*url.URL
}

type Signer interface {
	AccountWithCredits
	Signer2
	GetAuthority() *url.URL
}

type Signer2 interface {
	GetUrl() *url.URL
	GetVersion() uint64
	GetSignatureThreshold() uint64
	EntryByKey(key []byte) (int, KeyEntry, bool)
	EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool)
	EntryByDelegate(owner *url.URL) (int, KeyEntry, bool)
}

func EqualSigner(a, b Signer) bool {
	return EqualAccount(a, b)
}

func CopySigner(v Signer) Signer {
	return v.CopyAsInterface().(Signer)
}

func UnmarshalSigner(data []byte) (Signer, error) {
	account, err := UnmarshalAccount(data)
	if err != nil {
		return nil, err
	}

	signer, ok := account.(Signer)
	if !ok {
		return nil, fmt.Errorf("account type %v is not a signer", account.Type())
	}

	return signer, nil
}

func UnmarshalSignerFrom(r io.Reader) (Signer, error) {
	account, err := UnmarshalAccountFrom(r)
	if err != nil {
		return nil, err
	}

	signer, ok := account.(Signer)
	if !ok {
		return nil, fmt.Errorf("account type %v is not a signer", account.Type())
	}

	return signer, nil
}

func UnmarshalSignerJSON(data []byte) (Signer, error) {
	account, err := UnmarshalAccountJSON(data)
	if err != nil {
		return nil, err
	}

	if account == nil {
		return nil, nil
	}

	signer, ok := account.(Signer)
	if !ok {
		return nil, fmt.Errorf("account type %v is not a signer", account.Type())
	}

	return signer, nil
}

// MakeLiteSigner returns a copy of the signer with some fields removed.
// This is used for forwarding signers and storing signers in the transaction
// status.
func MakeLiteSigner(signer Signer2) Signer {
	switch signer := signer.(type) {
	case *KeyPage:
		// Make a copy of the key page with no keys
		signer = signer.Copy()
		signer.CreditBalance = 0

		keys := signer.Keys
		signer.Keys = make([]*KeySpec, 0, len(keys))
		for _, key := range keys {
			if key.Delegate != nil {
				// It's OK to use append here because the keys are already in
				// order
				signer.Keys = append(signer.Keys, &KeySpec{Delegate: key.Delegate})
			}
		}
		return signer

	case Signer:
		return signer

	default:
		return &UnknownSigner{Url: signer.GetUrl(), Version: signer.GetVersion()}
	}
}

/* ***** Unknown signer ***** */

func (s *UnknownSigner) GetUrl() *url.URL                                   { return s.Url }
func (s *UnknownSigner) GetAuthority() *url.URL                             { return s.Url }
func (s *UnknownSigner) StripUrl()                                          { s.Url = s.GetUrl().StripExtras() }
func (s *UnknownSigner) GetVersion() uint64                                 { return s.Version }
func (*UnknownSigner) GetSignatureThreshold() uint64                        { return math.MaxUint64 }
func (*UnknownSigner) EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool)  { return -1, nil, false }
func (*UnknownSigner) EntryByKey(key []byte) (int, KeyEntry, bool)          { return -1, nil, false }
func (*UnknownSigner) EntryByDelegate(owner *url.URL) (int, KeyEntry, bool) { return -1, nil, false }
func (*UnknownSigner) GetCreditBalance() uint64                             { return 0 }
func (*UnknownSigner) CreditCredits(amount uint64)                          {}
func (*UnknownSigner) DebitCredits(amount uint64) bool                      { return false }
func (*UnknownSigner) CanDebitCredits(amount uint64) bool                   { return false }

/* ***** Lite identity auth ***** */

func (li *LiteIdentity) GetAuthority() *url.URL {
	return li.Url
}

func (li *LiteIdentity) GetVersion() uint64 {
	return 1
}

func (li *LiteIdentity) GetSignatureThreshold() uint64 {
	return 1
}

func (li *LiteIdentity) EntryByKey(key []byte) (int, KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return li.EntryByKeyHash(keyHash[:])
}

func (li *LiteIdentity) EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool) {
	myKey, _ := ParseLiteIdentity(li.Url)
	if myKey == nil {
		panic("lite identity URL is not valid")
	}

	if !bytes.Equal(myKey, keyHash[:20]) {
		return -1, nil, false
	}
	return 0, li, true
}

// EntryByDelegate returns -1, nil, false.
func (*LiteIdentity) EntryByDelegate(owner *url.URL) (int, KeyEntry, bool) {
	return -1, nil, false
}

/* ***** ADI account auth ***** */

// GetSigners returns URLs of the book's pages.
func (b *KeyBook) GetSigners() []*url.URL {
	pages := make([]*url.URL, b.PageCount)
	for i := uint64(0); i < b.PageCount; i++ {
		pages[i] = FormatKeyPageUrl(b.Url, i)
	}
	return pages
}

func (p *KeyPage) GetAuthority() *url.URL {
	book, _, _ := ParseKeyPageUrl(p.Url)
	return book
}

// GetVersion returns Version.
func (p *KeyPage) GetVersion() uint64 { return p.Version }

// GetSignatureThreshold returns Threshold.
func (p *KeyPage) GetSignatureThreshold() uint64 {
	if p.AcceptThreshold == 0 {
		return 1
	}
	return p.AcceptThreshold
}

// EntryByKeyHash finds the entry with a matching key hash.
func (p *KeyPage) EntryByKey(key []byte) (int, KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return p.EntryByKeyHash(keyHash[:])
}

// EntryByDelegate finds the entry with a matching owner.
func (p *KeyPage) EntryByDelegate(owner *url.URL) (int, KeyEntry, bool) {
	if book, _, ok := ParseKeyPageUrl(owner); ok {
		return p.EntryByDelegate(book)
	}

	for i, entry := range p.Keys {
		if owner.Equal(entry.Delegate) {
			return i, entry, true
		}
	}

	return -1, nil, false
}
