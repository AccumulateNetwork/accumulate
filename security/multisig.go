package security

import (
	"bytes"
	"sort"

	"github.com/AccumulateNetwork/SMT/common"

	"golang.org/x/crypto/ed25519"
)

// MultiSig
// A slice of Sig structs where each Sig signs a message.  It gives
// the m signatures of a possible n specified by the MultiSigSpec
type MultiSig struct {
	msgHash *[32]byte // Hash of the message
	sig     []Sig     // Candidate Signatures.  Must be exactly equal to n
}

// Marshal
// Marshals the MultiSig into a byte slice
func (s *MultiSig) Marshal() (data []byte) {
	s.Check()                                                    // Sort the signatures
	data = append(data, s.msgHash[:]...)                         // Store the message hash
	data = append(data, common.Int64Bytes(int64(len(s.sig)))...) // Write out the sig count
	for _, aSig := range s.sig {                                 // For each sig
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
	for _, sig := range s.sig { //                                               Run through the signatures
		if !ed25519.Verify(sig.PublicKey(), s.msgHash[:], sig.Signature()) { //  and make sure they all sign the msg
			panic("Holds a signature that does not sign the message hash") //    hash, and blow up if one doesn't.
		} //
	}
}

// Unmarshal
// Get the values from a data slice to set up a MultiSig
// Note anything off, this function panics
func (s *MultiSig) Unmarshal(data []byte) []byte { //
	var m int64                       //           Number of provided signatures (m of n for multi-sig)
	var os Sig                        //           One Signature temp space
	copy(s.msgHash[:], data[:32])     //           Pull out the message hash
	data = data[32:]                  //           step to the next field
	m, data = common.BytesInt64(data) //           Pull m
	s.sig = s.sig[:0]                 //           Reset the Sig array
	for i := 0; int64(i) < m; i++ {   //           for m times
		os, data = Unmarshal(data) //               Pull a Sig out of the data
		s.sig = append(s.sig, os)  //               Append to the Sig List
	} //
	s.Check()   //                                  Check that all is well, everything is sorted
	return data //                                  Return the data
}

// AddSig
// Add a Sig to a MultiSig.
// panics if the signature doesn't match the message Hash
func (s *MultiSig) AddSig(msgHash [32]byte, sig Sig) {
	if s.msgHash == nil {
		s.msgHash = &msgHash
	} else {
		if !bytes.Equal(s.msgHash[:], msgHash[:]) {
			panic("attempt to add a signature that doesn't match the msgHash of the MultiSig")
		}
	}
	if !ed25519.Verify(sig.PublicKey(), msgHash[:], sig.Signature()) {
		panic("signature to be added to a MultiSig does not verify")
	}
	s.sig = append(s.sig, sig) //                   Add a signature to the multisig
}

// Verify
// Check that the signatures of the MultiSig all validly sign a given
// message.
func (s *MultiSig) Verify(message [32]byte) bool { // Verify a message with the signature
	for i, sig := range s.sig { // For every signatures
		if !sig.Verify(s, i, message) { //          The message must match
			return false //                         If it does not, then return false
		} //
	} //
	return true //                                  If all the signatures check out, return true
}
