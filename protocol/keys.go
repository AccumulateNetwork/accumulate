package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

type KeyEntry interface {
	GetLastUsedOn() uint64
	SetLastUsedOn(uint64)
}

// GetLastUsedOn returns LastUsedOn.
func (li *LiteIdentity) GetLastUsedOn() uint64 { return li.LastUsedOn }

// SetLastUsedOn sets LastUsedOn.
func (li *LiteIdentity) SetLastUsedOn(timestamp uint64) { li.LastUsedOn = timestamp }

// GetLastUsedOn returns LastUsedOn.
func (k *KeySpec) GetLastUsedOn() uint64 { return k.LastUsedOn }

// SetLastUsedOn sets LastUsedOn.
func (k *KeySpec) SetLastUsedOn(timestamp uint64) { k.LastUsedOn = timestamp }

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

// GetMofN
// return the signature requirements of the Key Page.  Each Key Page requires
// m of n signatures, where m <= n, and n is the number of keys on the key page.
// m is the Threshold number of signatures required to validate a transaction
func (ms *KeyPage) GetMofN() (m, n uint64) {
	m = ms.AcceptThreshold
	n = uint64(len(ms.Keys))
	return m, n
}

// SetThreshold
// set the signature threshold to M.  Returns an error if m > n
func (ms *KeyPage) SetThreshold(m uint64) error {
	if m <= uint64(len(ms.Keys)) {
		ms.AcceptThreshold = m
	} else {
		return fmt.Errorf("cannot require %d signatures on a key page with %d keys", m, len(ms.Keys))
	}
	return nil
}
