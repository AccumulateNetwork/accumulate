package database

import (
	"bytes"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type SignatureSet struct {
	txn      *Transaction
	signer   protocol.Signer
	writable bool
	entries  *sigSetData
}

// newSigSet creates a new SignatureSet.
func newSigSet(txn *Transaction, signer protocol.Signer, writable bool) (*SignatureSet, error) {
	s := new(SignatureSet)
	s.txn = txn
	s.signer = signer
	s.writable = writable
	s.entries = new(sigSetData)

	err := txn.batch.getValuePtr(s.key(), s.entries, &s.entries, true)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	// Reset if the set is writable and the version is different
	if writable && s.entries.Version != signer.GetVersion() {
		s.entries.Reset(signer.GetVersion())
	}
	return s, nil
}

func (s *SignatureSet) key() storage.Key {
	return s.txn.key.Signatures(s.signer.GetUrl())
}

func (s *SignatureSet) Count() int {
	return len(s.entries.Entries)
}

func (s *SignatureSet) Entries() []SigSetEntry {
	entries := make([]SigSetEntry, len(s.entries.Entries))
	copy(entries, s.entries.Entries)
	return entries
}

func (s *sigSetData) Reset(version uint64) {
	// Retain system signature entries
	system := make([]SigSetEntry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e.System {
			system = append(system, e)
		}
	}

	// Remove all other entries and update the version
	s.Version = version
	s.Entries = system
}

func (s *SigSetEntry) Compare(t *SigSetEntry) int {
	switch {
	case !s.System && !t.System:
		return int(s.KeyEntryIndex) - int(t.KeyEntryIndex)
	case !s.System:
		return -1
	case !t.System:
		return +1
	}

	return bytes.Compare(s.SignatureHash[:], t.SignatureHash[:])
}

func (s *sigSetData) Add(newEntry SigSetEntry, newSignature protocol.Signature) bool {
	// The signature is a system signature if it's one of the system types or if
	// the signer is a node.
	switch {
	case newSignature.Type().IsSystem():
		newEntry.System = true
	case protocol.IsDnUrl(newSignature.GetSigner()):
		newEntry.System = true
	default:
		_, ok := protocol.ParseSubnetUrl(newSignature.GetSigner())
		newEntry.System = ok
	}

	// Check the signer version
	if keysig, ok := newSignature.(protocol.KeySignature); ok && !newEntry.System && s.Version != keysig.GetSignerVersion() {
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
	if !s.entries.Add(newEntry, newSignature) {
		return len(s.entries.Entries), nil
	}

	err := s.txn.ensureSigner(s.signer)
	if err != nil {
		return 0, err
	}
	s.txn.batch.putValue(s.key(), s.entries)
	return len(s.entries.Entries), nil
}
