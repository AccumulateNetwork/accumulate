package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func (k *KeySpecParams) IsEmpty() bool {
	return len(k.PublicKey) == 0 && k.Owner == nil
}

func (ms *KeyPage) FindKey(pubKey []byte) *KeySpec {
	_, ks, _ := ms.EntryByKey(pubKey)
	return ks
}

// EntryByKey finds the entry that matches the hash of the key.
func (p *KeyPage) EntryByKey(key []byte) (int, *KeySpec, bool) {
	keyHash := sha256.Sum256(key)
	return p.EntryByKeyHash(keyHash[:])
}

// EntryByKeyHash finds the entry with a matching key hash.
func (p *KeyPage) EntryByKeyHash(keyHash []byte) (int, *KeySpec, bool) {
	for i, entry := range p.Keys {
		if bytes.Equal(entry.PublicKey, keyHash) {
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
