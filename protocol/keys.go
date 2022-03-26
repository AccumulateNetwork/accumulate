package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func (p *KeyPage) GetSignatureThreshold() uint64 {
	return p.Threshold
}

func (*LiteTokenAccount) GetSignatureThreshold() uint64 {
	return 1
}

func (k *KeySpecParams) IsEmpty() bool {
	return len(k.KeyHash) == 0 && k.Owner == nil
}

func (ms *KeyPage) FindKey(pubKey []byte) *KeySpec {
	// Check each key
	x := sha256.Sum256(pubKey)
	for _, candidate := range ms.Keys {
		// Try with each supported hash algorithm
		if bytes.Equal(candidate.PublicKeyHash[:], x[:]) {
			return candidate
		}

	}

	return nil
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

// EntryByOwner finds the entry with a matching owner
func (p *KeyPage) EntryByOwner(owner *url.URL) (int, *KeySpec, bool) {
	for i, entry := range p.Keys {
		if owner.Equal(entry.Owner) {
			return i, entry, true
		}
	}

	return -1, nil, false
}

// GetMofN
// return the signature requirements of the Key Page.  Each Key Page requires
// m of n signatures, where m <= n, and n is the number of keys on the key page.
// m is the Threshold number of signatures required to validate a transaction
func (ms *KeyPage) GetMofN() (m, n uint64) {
	m = ms.Threshold
	n = uint64(len(ms.Keys))
	return m, n
}

// SetThreshold
// set the signature threshold to M.  Returns an error if m > n
func (ms *KeyPage) SetThreshold(m uint64) error {
	if m <= uint64(len(ms.Keys)) {
		ms.Threshold = m
	} else {
		return fmt.Errorf("cannot require %d signatures on a key page with %d keys", m, len(ms.Keys))
	}
	return nil
}

type KeyHolder interface {
	EntryByKey(key []byte) (int, KeyEntry, bool)
	EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool)
}

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

// EntryByKeyHash checks if the key's hash matches the lite token account URL.
func (l *LiteTokenAccount) EntryByKey(key []byte) (int, KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return l.EntryByKeyHash(keyHash[:])
}

type KeyEntry interface {
	GetLastUsedOn() uint64
	SetLastUsedOn(uint64)
}

func (k *KeySpec) GetLastUsedOn() uint64 {
	return k.LastUsedOn
}

func (k *KeySpec) SetLastUsedOn(timestamp uint64) {
	k.LastUsedOn = timestamp
}

func (l *LiteTokenAccount) GetLastUsedOn() uint64 {
	return l.LastUsedOn
}

func (l *LiteTokenAccount) SetLastUsedOn(timestamp uint64) {
	l.LastUsedOn = timestamp
}
