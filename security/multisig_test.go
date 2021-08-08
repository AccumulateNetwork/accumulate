package security

import (
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/crypto/ed25519"
)

// BuildMultiVectors
// Build nCount test multiSignatures.  All choices of m for all multiSignatures
// of n Signatures where n <= nCount
func BuildMultiVectors(nCount int) (mSigs []*MSigs, messages [][]byte) {

	for n := 1; n <= nCount; n += 1 { //                   Create MultiSigs with n upto nCount
		mSig := new(MSigs)                 //              Working structure for building the mSigs
		var Private [][]byte               //              Place for private keys
		s := []byte{byte(n), byte(n >> 8), //              Use n as a seed
			byte(n >> 16), byte(n >> 32)} //                 which will work as long as n < ~4 GB
		mSig.sig, _, _, Private, _, _ = buildSigs(s, n) // Get n Sigs.  Get the Private keys, because we need to
		var message [1234]byte                          // Every Sig is going to sign this message
		rand.Read(message[:])                           // Set the message to some random stuff
		for i, sig := range mSig.sig {                  // For all sig in mSig.sig
			s := ed25519.Sign(Private[i], message[:]) //   but repurpose each signature to sign the message
			sig.(*SigEd25519).sig = s                 //   by updating the signature
		}
		messages = append(messages, append([]byte{}, message[:]...)) // Add the message to the messages list
		mSigs = append(mSigs, mSig)                                  //              Add the updated m2Sig to the set of mSigs
	}
	return mSigs, messages
}

func TestMultiSig_Verify(t *testing.T) {
	mSigs, messages := BuildMultiVectors(300)
	fmt.Printf("number of test cases %d\n", len(messages))
	for i, m := range messages {
		if !mSigs[i].Verify(m) {
			t.Error(fmt.Sprintf("Failed signature %d", i))
		}
		m[1] = m[1] ^ 1
		if mSigs[i].Verify(m) {
			t.Error(fmt.Sprintf("Accepted a bad message %d", i))
		}
		m[1] = m[1] ^ 1
	}
	var data []byte
	for _, sig := range mSigs {
		data = append(data, sig.Marshal()...)
	}
	var mSig = new(MSigs)
	for i, m := range messages {
		data = mSig.Unmarshal(data)
		if !mSig.Verify(m) {
			t.Errorf("failure to marshal() and Unmarshal() %d", i)
		}
	}
}
