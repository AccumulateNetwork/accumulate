package security

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/AccumulateNetwork/SMT/smt/storage"
)

// SigSpec
// Defines the public keys that can be used in a multiSignature
type SigSpec struct {
	PublicKeys [][32]byte // Candidate Signatures.  Must be exactly equal to n
	m, n       int64
}

// Marshal
// Marshals the SigSpec into a byte slice
func (s *SigSpec) Marshal() (data []byte) {
	s.Check()                                       // Sort the signatures
	data = append(data, storage.Int64Bytes(s.m)...) // Write out the sig count
	data = append(data, storage.Int64Bytes(s.n)...) // Write out the sig count
	for _, publicKey := range s.PublicKeys {        // For each sig
		data = append(data, publicKey[:]...) //           Append its data representation
	}
	return data //                                     Return the bytes
}

// Check
// Panics if m of n and list of public keys are not consistent.
// As a side effect, Check sorts the Public Keys
func (s *SigSpec) Check() {
	if s.m < 1 || s.m > s.n || int(s.n) != len(s.PublicKeys) { //                1 <= m <= n && n == Len(s.PublicKeys)
		panic(fmt.Sprintf("sigSpec not valid: m=%d n=%d len(public keys)=%d", // If not, panic
			s.m, s.n, len(s.PublicKeys))) //
	}
	sort.Slice(s.PublicKeys, func(i, j int) bool { //                            Make sure all signatures are in order
		return bytes.Compare(s.PublicKeys[i][:], s.PublicKeys[j][:]) < 0 //            Less then test comparing public keys
	})
	for i := 0; i < int(s.n)-1; i++ { //                                         Loop n-1 times (compare i with i+1 )
		if bytes.Equal(s.PublicKeys[i][:], s.PublicKeys[i+1][:]) { //                  If any are equal, blow.  We are sorted
			panic("Allow no duplicate keys") //                                  so duplicates must be right next to
		} //                                                                     each other.
	}
}

// Unmarshal
// Get the values from a data slice to set up a SigSpec
// Note anything off, this function panics
func (s *SigSpec) Unmarshal(data []byte) []byte { //
	s.m, data = storage.BytesInt64(data) //          Number of required signatures
	s.n, data = storage.BytesInt64(data) //          Number of candidate signatures
	for i := 0; int64(i) < s.n; i++ {    //          unmarshal n public keys
		var p [32]byte                         //    Copy the data into an array
		copy(p[:], data[:32])                  //
		s.PublicKeys = append(s.PublicKeys, p) //    Get a key
		data = data[32:]                       //    Move the data pointer
	} //
	s.Check()   //
	return data //
}

// AddSig
// Add a Sig to a SigSpec
func (s *SigSpec) AddSig(publicKey [32]byte) {
	s.PublicKeys = append(s.PublicKeys, publicKey)
	s.n = int64(len(s.PublicKeys))
}

// Verify
// SigSpec verifies a MultiSig instance meets the SigSpec.  Note that the
// sigs in a MultiSig likely come from different parties, so we can tolerate
// some not matching, or more matching than we need.
func (s *SigSpec) Verify(mSig *MultiSig) (r bool) {
	defer func() { //            for any error
		if recover() != nil { // that's an indication this doesn't work
			r = false //           so return false
		}
	}()
	mSig.Check()                  // Make sure the mSig itself is good, and sorted
	s.Check()                     // Make sure the SigSpec is good and sorted
	if len(mSig.sig) < int(s.m) { // If there are not enough signatures
		return false //              we just don't care.  It can't be right
	}
	idx := 0                                 //                          Now check that all the public keys in mSig
	good := 0                                //                          Count of good signatures found in mSig
	for _, publicKey := range s.PublicKeys { //                          in the mSig match public keys in the SigSpec
	goodloop:
		for good < int(s.m) { //   Look for enough good sigs
			switch bytes.Compare(publicKey[:], mSig.sig[idx].PublicKey()) { // Check SigSpec against mSig. Both are sorted
			case 0: //             If the key matches the mSig, great!
				idx++  //          Go to the next public key in mSig and SigSpec
				good++ //          And count this one as good
			case 1: //             If the mSig is greater than Sig, remember both are sorted, so mSig.sig[idx] is bad
				idx++    //           go to next mSig.sig[idx]
				continue //        But continue this loop; don't progress the SigSpec publicKey
			default: //            If the public is less, then go to the next public key
				break goodloop //
			}
		}
		if good >= int(s.m) { //   Are we done?  Let's not go further if so.
			break //
		} //
	}
	if good < int(s.m) { //        After everything is done, make sure we didn't just fall out of the loop
		return false //            If we don't have enough good matches, return false
	} //
	return true //                 Ah! all turned out well!
}
