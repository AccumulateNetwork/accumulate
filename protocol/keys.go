// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"fmt"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type KeyEntry interface {
	GetLastUsedOn() uint64
	SetLastUsedOn(uint64)

	// CanUseTimestamp reports whether a signature timestamp passes replay
	// protection, and UseTimestamp records it as spent. Together they
	// implement a bounded reordering window (#4132): DAG-BFT commits batches
	// in DAG order, not submission order, so a signer's transactions reach
	// the executor shuffled. The strict lastUsedOn >= timestamp rule
	// silently discarded everything but an increasing subsequence — 4 of a
	// 100-transaction burst in production.
	CanUseTimestamp(timestamp uint64) error
	UseTimestamp(timestamp uint64)
}

// ReplayWindowSize is how many spent timestamps a key entry retains:
// LastUsedOn plus up to ReplayWindowSize-1 prior entries. A burst of up to
// ReplayWindowSize transactions from one signer survives arbitrary
// reordering; a timestamp below the oldest retained entry once the window is
// full is rejected as too old, because the entry can no longer prove it is
// not a replay. The state cost is bounded at ReplayWindowSize uvarints per
// key entry — every spent timestamp inside the window must be retained,
// because accepting an unseen timestamp below LastUsedOn is only safe if
// spent and unspent can be told apart.
const ReplayWindowSize = 128

// canUseTimestamp is the windowed replay rule. The spent set is
// prior ∪ {last}; prior is sorted ascending and every element is < last.
func canUseTimestamp(last uint64, prior []uint64, timestamp uint64) error {
	switch {
	case timestamp == 0:
		// Zero opts out of replay protection entirely
		return nil
	case timestamp > last:
		return nil
	case timestamp == last:
		return errors.BadTimestamp.WithFormat("timestamp %d has already been used", timestamp)
	}
	if _, found := sortutil.Search(prior, func(v uint64) int {
		switch {
		case v < timestamp:
			return -1
		case v > timestamp:
			return +1
		}
		return 0
	}); found {
		return errors.BadTimestamp.WithFormat("timestamp %d has already been used", timestamp)
	}
	if len(prior) >= ReplayWindowSize-1 && timestamp < prior[0] {
		return errors.BadTimestamp.WithFormat("timestamp %d is below the replay window floor %d", timestamp, prior[0])
	}
	return nil
}

// useTimestamp records a timestamp as spent, keeping last = max(spent) and
// pruning the retained set to ReplayWindowSize from the low end.
func useTimestamp(last *uint64, prior *[]uint64, timestamp uint64) {
	if timestamp == 0 {
		return
	}
	insert := func(v uint64) {
		ptr, added := sortutil.BinaryInsert(prior, func(entry uint64) int {
			switch {
			case entry < v:
				return -1
			case entry > v:
				return +1
			}
			return 0
		})
		if added {
			*ptr = v
		}
	}
	if timestamp > *last {
		if *last != 0 {
			insert(*last)
		}
		*last = timestamp
	} else {
		insert(timestamp)
	}
	if n := len(*prior); n > ReplayWindowSize-1 {
		*prior = (*prior)[n-(ReplayWindowSize-1):]
	}
}

// GetLastUsedOn returns LastUsedOn.
func (li *LiteIdentity) GetLastUsedOn() uint64 { return li.LastUsedOn }

// SetLastUsedOn sets LastUsedOn.
func (li *LiteIdentity) SetLastUsedOn(timestamp uint64) { li.LastUsedOn = timestamp }

// CanUseTimestamp implements the windowed replay rule for a lite identity.
func (li *LiteIdentity) CanUseTimestamp(timestamp uint64) error {
	return canUseTimestamp(li.LastUsedOn, li.PriorUsedOn, timestamp)
}

// UseTimestamp records a spent timestamp on a lite identity.
func (li *LiteIdentity) UseTimestamp(timestamp uint64) {
	useTimestamp(&li.LastUsedOn, &li.PriorUsedOn, timestamp)
}

// GetLastUsedOn returns LastUsedOn.
func (k *KeySpec) GetLastUsedOn() uint64 { return k.LastUsedOn }

// SetLastUsedOn sets LastUsedOn.
func (k *KeySpec) SetLastUsedOn(timestamp uint64) { k.LastUsedOn = timestamp }

// CanUseTimestamp implements the windowed replay rule for a key page entry.
func (k *KeySpec) CanUseTimestamp(timestamp uint64) error {
	return canUseTimestamp(k.LastUsedOn, k.PriorUsedOn, timestamp)
}

// UseTimestamp records a spent timestamp on a key page entry.
func (k *KeySpec) UseTimestamp(timestamp uint64) {
	useTimestamp(&k.LastUsedOn, &k.PriorUsedOn, timestamp)
}

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
