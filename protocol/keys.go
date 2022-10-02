// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
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
	return len(k.KeyHash) == 0 && k.Delegate == nil
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
	if m <= uint64(len(ms.Keys)) && m > 0 {
		ms.AcceptThreshold = m
	} else if m == 0 {
		return fmt.Errorf("cannot require 0 signatures on a key page")
	} else {
		return fmt.Errorf("cannot require %d signatures on a key page with %d keys", m, len(ms.Keys))
	}
	return nil
}

// EntryByKeyHash finds the entry with a matching key hash.
func (p *KeyPage) EntryByKeyHash(keyHash []byte) (int, KeyEntry, bool) {
	i, found := sortutil.Search(p.Keys, func(ks *KeySpec) int {
		return bytes.Compare(ks.PublicKeyHash, keyHash)
	})
	if !found {
		return -1, nil, false
	}
	return i, p.Keys[i], true
}

// AddKeySpec adds a key spec to the page.
func (p *KeyPage) AddKeySpec(k *KeySpec) {
	ptr, _ := sortutil.BinaryInsert(&p.Keys, func(l *KeySpec) int {
		v := bytes.Compare(l.PublicKeyHash, k.PublicKeyHash)
		switch {
		case v != 0:
			return v
		case l.Delegate == nil:
			return -1
		case k.Delegate == nil:
			return +1
		default:
			return l.Delegate.Compare(k.Delegate)
		}
	})
	*ptr = k
}

// RemoveKeySpecAt removes the I'th key spec.
func (p *KeyPage) RemoveKeySpecAt(i int) {
	copy(p.Keys[i:], p.Keys[i+1:])
	p.Keys = p.Keys[:len(p.Keys)-1]
}
