package security

import (
	"bytes"
	"sort"

	"github.com/AccumulateNetwork/SMT/storage"
)

// MSigs
// Signature where multiple signatures are provided at once.  This provides
// the m signatures of a possible n specified by the MultiSigSpec
type MSigs struct {
	sig []Sig // Candidate Signatures.  Must be exactly equal to n
}

// Marshal
// Marshals the MSigs into a byte slice
func (s *MSigs) Marshal() (data []byte) {
	s.Sort()                                                      // Sort the signatures
	data = append(data, storage.Int64Bytes(int64(len(s.sig)))...) // Write out the sig count
	for _, aSig := range s.sig {                                  // For each sig
		data = append(data, aSig.Marshal()...) //                    Append its data representation
	}
	return data //                                                               Return the bytes
}

// Sort
// Return true if the given MSigs atleast appears to be valid
func (s *MSigs) Sort() {
	sort.Slice(s.sig, func(i, j int) bool { //                                   Make sure all signatures are in order
		return bytes.Compare(s.sig[i].PublicKey(), s.sig[j].PublicKey()) < 0 //  Less then test comparing public keys
	}) //
}

// Unmarshal
// Get the values from a data slice to set up a MSigs
// Note anything off, this function panics
func (s *MSigs) Unmarshal(data []byte) []byte {
	var m int64
	var os Sig
	m, data = storage.BytesInt64(data)
	s.sig = s.sig[:0]
	for i := 0; int64(i) < m; i++ {
		os, data = Unmarshal(data)
		s.sig = append(s.sig, os)
	}
	s.Sort()
	return data
}

// AddSig
// Add a Sig to a MSigs
func (s *MSigs) AddSig(sig Sig) {
	s.sig = append(s.sig, sig)
}

func (s *MSigs) Verify(message []byte) bool {
	for i, sig := range s.sig {
		if !sig.Verify(s, i, message) {
			return false
		}
	}
	return true
}
