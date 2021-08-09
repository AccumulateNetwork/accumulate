package security

import (
	"bytes"
	"sort"

	"github.com/AccumulateNetwork/SMT/storage"
)

// MultiSig
// A slice of Sig structs where each Sig signs a message.  It gives
// the m signatures of a possible n specified by the MultiSigSpec
type MultiSig struct {
	sig []Sig // Candidate Signatures.  Must be exactly equal to n
}

// Marshal
// Marshals the MultiSig into a byte slice
func (s *MultiSig) Marshal() (data []byte) {
	s.Check()                                                     // Sort the signatures
	data = append(data, storage.Int64Bytes(int64(len(s.sig)))...) // Write out the sig count
	for _, aSig := range s.sig {                                  // For each sig
		data = append(data, aSig.Marshal()...) //                    Append its data representation
	}
	return data //                                                   Return the bytes
}

// Check
// Throws a panic if anything is wrong about the MultiSig.  Mostly that the
// signatures given have no duplicates.
func (s *MultiSig) Check() {
	sort.Slice(s.sig, func(i, j int) bool { //                                   Make sure all signatures are in order
		return bytes.Compare(s.sig[i].PublicKey(), s.sig[j].PublicKey()) < 0 //  Less then test comparing public keys
	}) //
	for i := 0; i < len(s.sig)-1; i++ { //                                         Look through the signatures
		if bytes.Equal(s.sig[i].PublicKey(), s.sig[i+1].PublicKey()) { //        If any are duplicates, blow
			panic("no duplicate signatures allowed") //                            with an error saying why
		}
	}
}

// Unmarshal
// Get the values from a data slice to set up a MultiSig
// Note anything off, this function panics
func (s *MultiSig) Unmarshal(data []byte) []byte { //
	var m int64                        //           Number of provided signatures (m of n for multi-sig)
	var os Sig                         //           One Signature temp space
	m, data = storage.BytesInt64(data) //           Pull m
	s.sig = s.sig[:0]                  //           Reset the Sig array
	for i := 0; int64(i) < m; i++ {    //           for m times
		os, data = Unmarshal(data) //               Pull a Sig out of the data
		s.sig = append(s.sig, os)  //               Append to the Sig List
	} //
	s.Check()   //                                  Check that all is well, everything is sorted
	return data //                                  Return the data
}

// AddSig
// Add a Sig to a MultiSig
func (s *MultiSig) AddSig(sig Sig) {
	s.sig = append(s.sig, sig) //                   Add a signature to the multisig
}

// Verify
// Check that the signatures of the MultiSig all validly sign a given
// message.
func (s *MultiSig) Verify(message []byte) bool { // Verify a message with the signature
	for i, sig := range s.sig { // For every signatures
		if !sig.Verify(s, i, message) { //          The message must match
			return false //                         If it does not, then return false
		} //
	} //
	return true //                                  If all the signatures check out, return true
}
