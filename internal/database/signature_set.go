// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"fmt"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SignatureSet struct {
	txn      *Transaction
	signer   protocol.Signer2
	writable bool
	value    values.Value[*sigSetData]
	entries  *sigSetData
	other    []*SignatureSetEntry
}

// newSigSet creates a new SignatureSet.
func newSigSet(txn *Transaction, signer protocol.Signer2, writable bool) (*SignatureSet, error) {
	s := new(SignatureSet)
	s.txn = txn
	s.signer = signer
	s.writable = writable
	s.value = txn.getSignatures(signer.GetUrl())

	var err error
	s.entries, err = s.value.Get()
	if err != nil {
		return nil, err
	}

	s.other, err = txn.parent.
		Account(signer.GetUrl()).
		Transaction(txn.hash32()).
		Signatures().
		Get()
	if err != nil {
		return nil, err
	}

	// Reset if the set is writable and the version is different
	if writable && s.entries.Version != signer.GetVersion() {
		s.entries.Reset(signer.GetVersion())
	}
	return s, nil
}

func (s *SignatureSet) Count() int {
	return len(s.entries.Entries) + len(s.other)
}

func (s *SignatureSet) Version() uint64 {
	return s.entries.Version
}

func (s *SignatureSet) Entries() []SigSetEntry {
	entries := make([]SigSetEntry, len(s.entries.Entries)+len(s.other))
	n := copy(entries, s.entries.Entries)
	for i, v := range s.other {
		entries[i+n] = SigSetEntry{
			KeyEntryIndex: v.KeyIndex,
			SignatureHash: v.Hash,
		}
	}
	return entries
}

func (s *sigSetData) Reset(version uint64) {
	// Retain retain signature entries
	retain := make([]SigSetEntry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e.Type.IsSystem() || e.ValidatorKeyHash != nil {
			retain = append(retain, e)
		}
	}

	// Remove all other entries and update the version
	s.Version = version
	s.Entries = retain
}

func (s *SigSetEntry) Compare(t *SigSetEntry) int {
	// If both are system, compare by signature hash, otherwise system sorts
	// first
	switch {
	case s.Type.IsSystem() && t.Type.IsSystem():
		return bytes.Compare(s.SignatureHash[:], t.SignatureHash[:])
	case s.Type.IsSystem():
		return -1
	case t.Type.IsSystem():
		return +1
	}

	// If both are validators, compare by key hash, otherwise validators sort
	// first
	switch {
	case s.ValidatorKeyHash != nil && t.ValidatorKeyHash != nil:
		return bytes.Compare((*s.ValidatorKeyHash)[:], (*t.ValidatorKeyHash)[:])
	case s.ValidatorKeyHash != nil:
		return -1
	case t.ValidatorKeyHash != nil:
		return +1
	}

	// Compare by key entry index
	return int(s.KeyEntryIndex) - int(t.KeyEntryIndex)
}

func (s *sigSetData) Add(newEntry SigSetEntry, newSignature protocol.Signature) bool {
	// Check the signer version
	if keysig, ok := newSignature.(protocol.KeySignature); ok && newEntry.ValidatorKeyHash == nil && s.Version != keysig.GetSignerVersion() {
		return false
	}

	// Find based on the key keyHash
	ptr, new := sortutil.BinaryInsert(&s.Entries, func(entry SigSetEntry) int {
		return entry.Compare(&newEntry)
	})

	*ptr = newEntry
	return new
}

// Add adds a signature to the signature set. Add does nothing if the signature
// set already includes the signer's public key. The entry hash must refer to a
// signature chain entry.
func (s *SignatureSet) Add(keyEntryIndex uint64, newSignature protocol.Signature) (int, error) {
	if !s.writable {
		return 0, fmt.Errorf("signature set opened as read-only")
	}

	var newEntry SigSetEntry
	newEntry.Type = newSignature.Type()
	newEntry.KeyEntryIndex = keyEntryIndex
	newEntry.SignatureHash = *(*[32]byte)(newSignature.Hash())
	if sig, ok := newSignature.(protocol.KeySignature); ok && protocol.DnUrl().JoinPath(protocol.Network).Equal(newSignature.GetSigner()) {
		newEntry.ValidatorKeyHash = (*[32]byte)(sig.GetPublicKeyHash())
	}
	if !s.entries.Add(newEntry, newSignature) {
		return len(s.entries.Entries), nil
	}

	err := s.txn.ensureSigner(s.signer)
	if err != nil {
		return 0, err
	}
	return len(s.entries.Entries), s.value.Put(s.entries)
}
