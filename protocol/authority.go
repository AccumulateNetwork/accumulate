package protocol

import (
	"bytes"
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type Authority interface {
	Account
	GetSigners() []*url.URL
}

type Signer interface {
	AccountWithCredits
	GetVersion() uint64
	GetSignatureThreshold() uint64
	EntryByKey(key []byte) (int, KeyEntry, bool)
	EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool)
}

/* ***** Lite account auth ***** */

// GetSigners returns the lite address.
func (l *LiteTokenAccount) GetSigners() []*url.URL { return []*url.URL{l.Url} }

// GetVersion returns 1.
func (*LiteTokenAccount) GetVersion() uint64 { return 1 }

// GetSignatureThreshold returns 1.
func (*LiteTokenAccount) GetSignatureThreshold() uint64 { return 1 }

// EntryByKeyHash checks if the key hash matches the lite token account URL.
// EntryByKeyHash will panic if the lite token account URL is invalid.
func (l *LiteTokenAccount) EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool) {
	myKey, _, _ := ParseLiteTokenAddress(l.Url)
	if myKey == nil {
		panic("lite token account URL is not a valid lite address")
	}

	if !bytes.Equal(myKey, keyHash[:20]) {
		return -1, nil, false
	}
	return 0, l, true
}

// EntryByKey checks if the key's hash matches the lite token account URL.
// EntryByKey will panic if the lite token account URL is invalid.
func (l *LiteTokenAccount) EntryByKey(key []byte) (int, KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return l.EntryByKeyHash(keyHash[:])
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

// GetVersion returns Version.
func (p *KeyPage) GetVersion() uint64 { return p.Version }

// GetSignatureThreshold returns Threshold.
func (p *KeyPage) GetSignatureThreshold() uint64 {
	if p.Threshold == 0 {
		return 1
	}
	return p.Threshold
}

// EntryByKeyHash finds the entry with a matching key hash.
func (p *KeyPage) EntryByKey(key []byte) (int, KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return p.EntryByKeyHash(keyHash[:])
}

// EntryByKeyHash finds the entry with a matching key hash.
func (p *KeyPage) EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool) {
	for i, entry := range p.Keys {
		if bytes.Equal(entry.PublicKeyHash, keyHash) {
			return i, entry, true
		}
	}

	return -1, nil, false
}
