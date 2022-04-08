package database

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type SignatureSet struct {
	txn      *Transaction
	signer   protocol.Signer
	writable bool
	hashes   *sigSetData
}

// newSigSet creates a new SignatureSet.
func newSigSet(txn *Transaction, signer protocol.Signer, writable bool) (*SignatureSet, error) {
	s := new(SignatureSet)
	s.txn = txn
	s.signer = signer
	s.writable = writable
	s.hashes = new(sigSetData)

	err := txn.batch.getValuePtr(s.key(), s.hashes, &s.hashes, true)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	// Reset if the set is writable and the version is different
	if writable && s.hashes.Version != signer.GetVersion() {
		s.hashes.Reset(signer.GetVersion())
	}
	return s, nil
}

func (s *SignatureSet) key() storage.Key {
	return s.txn.key.Signatures(s.signer.GetUrl())
}

func (s *SignatureSet) Count() int {
	return len(s.hashes.Entries)
}

func (s *SignatureSet) EntryHashes() [][32]byte {
	h := make([][32]byte, len(s.hashes.Entries))
	for i, e := range s.hashes.Entries {
		h[i] = e.EntryHash
	}
	return h
}

func (s *sigSetData) Reset(version uint64) {
	// Retain system signature entries
	system := make([]sigSetKeyData, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e.System {
			system = append(system, e)
		}
	}

	// Remove all other entries and update the version
	s.Version = version
	s.Entries = system
}

func (s *sigSetData) Add(entryHash [32]byte, newSignature protocol.Signature) bool {
	var newEntry sigSetKeyData
	newEntry.KeyHash = signatureKeyHash(newSignature)
	newEntry.EntryHash = entryHash

	// The signature is a system signature if it's one of the system types or if
	// the signer is a node.
	switch {
	case newSignature.Type().IsSystem():
		newEntry.System = true
	case protocol.IsDnUrl(newSignature.GetSigner()):
		newEntry.System = true
	default:
		_, ok := protocol.ParseBvnUrl(newSignature.GetSigner())
		newEntry.System = ok
	}

	// Check the signer version
	if !newEntry.System && s.Version != newSignature.GetSignerVersion() {
		return false
	}

	// Find based on the key keyHash
	i := sort.Search(len(s.Entries), func(i int) bool {
		return bytes.Compare(s.Entries[i].KeyHash[:], newEntry.KeyHash[:]) >= 0
	})

	// Append
	if i >= len(s.Entries) {
		s.Entries = append(s.Entries, newEntry)
		return true
	}

	// An entry exists for the key, so ignore this one
	if s.Entries[i].KeyHash == newEntry.KeyHash {
		return false
	}

	// Insert
	s.Entries = append(s.Entries, sigSetKeyData{})
	copy(s.Entries[i+1:], s.Entries[i:])
	s.Entries[i] = newEntry
	return true
}

// Add adds a signature to the signature set. Add does nothing if the signature
// set already includes the signer's public key. The entry hash must refer to a
// signature chain entry.
func (s *SignatureSet) Add(newSignature protocol.Signature) (int, error) {
	if !s.writable {
		return 0, fmt.Errorf("signature set opened as read-only")
	}

	data, err := newSignature.MarshalBinary()
	if err != nil {
		return 0, fmt.Errorf("marshal signature: %w", err)
	}

	entryHash := sha256.Sum256(data)
	if !s.hashes.Add(entryHash, newSignature) {
		return len(s.hashes.Entries), nil
	}

	err = s.txn.ensureSigner(s.signer)
	if err != nil {
		return 0, err
	}

	s.txn.batch.putValue(s.key(), s.hashes)
	return len(s.hashes.Entries), nil
}

// signatureKeyHash returns a hash that is used to prevent two signatures from
// the same key.
func signatureKeyHash(sig protocol.Signature) [32]byte {
	switch sig := sig.(type) {
	case *protocol.SyntheticSignature:
		// Multiple synthetic signatures doesn't make sense, but if they're
		// unique... ok
		hasher := make(hash.Hasher, 0, 3)
		hasher.AddUrl(sig.SourceNetwork)
		hasher.AddUrl(sig.DestinationNetwork)
		hasher.AddUint(sig.SequenceNumber)
		return *(*[32]byte)(hasher.MerkleHash())

	case *protocol.ReceiptSignature:
		// Multiple receipts doesn't make sense, but if they anchor to a unique
		// root... ok
		return *(*[32]byte)(sig.Result)

	case *protocol.InternalSignature:
		// Internal signatures only make any kind of sense if they're coming
		// from the local network, so they should never be different
		return sha256.Sum256([]byte("Accumulate Internal Signature"))

	default:
		// Normal signatures must come from a unique key
		hash := sig.GetPublicKeyHash()
		if hash == nil {
			panic("attempted to add a signature that doesn't have a key!")
		}
		return *(*[32]byte)(hash)
	}
}
