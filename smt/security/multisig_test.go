package security

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/crypto/ed25519"
)

// BuildMultiVectors
// Build nCount test multiSignatures.  All choices of m for all multiSignatures
// of n Signatures where n <= nCount
func BuildMultiVectors(nCount int) (mSigs []*MultiSig, messages [][32]byte) {

	for n := 1; n <= nCount; n += 1 { //                   Create MultiSigs with n upto nCount
		mSig := new(MultiSig)              //              Working structure for building the mSigs
		var Private [][]byte               //              Place for private keys
		s := []byte{byte(n), byte(n >> 8), //              Use n as a seed
			byte(n >> 16), byte(n >> 32)} //                 which will work as long as n < ~4 GB
		mSig.sig, _, _, Private, _, _ = buildSigs(s, n) // Get n Sigs.  Get the Private keys, because we need to
		var message [1234]byte                          // Every Sig is going to sign this message
		rand.Read(message[:])                           // Set the message to some random stuff
		msgHash := sha256.Sum256(message[:])            // Get the hash of the message
		for i, sig := range mSig.sig {                  // For all sig in mSig.sig
			s := ed25519.Sign(Private[i], msgHash[:]) //   but repurpose each signature to sign the message
			sig.(*SigEd25519).sig = s                 //   by updating the signature
		}
		messages = append(messages, msgHash) //            Add the message to the messages list
		mSigs = append(mSigs, mSig)          //              Add the updated m2Sig to the set of mSigs
	}
	return mSigs, messages
}

func TestMultiSig_Verify(t *testing.T) {
	t.Skip("ignoring")
	mSigs, messages := BuildMultiVectors(50)               //           Get a pool of test vectors
	fmt.Printf("number of test cases %d\n", len(messages)) //           For grins, print how many tests
	for i, m := range messages {                           //           For each of these vectors
		if !mSigs[i].Verify(m) { //                                     Make sure they validate their message
			t.Error(fmt.Sprintf("Failed signature %d", i)) //           Bonk if they don't.
		} //
		m[1] = m[1] ^ 1         //                                      Flip a bit in the message
		if mSigs[i].Verify(m) { //                                      Make sure the multisig fails the message
			t.Error(fmt.Sprintf("Accepted a bad message %d", i)) //
		} //
		m[1] = m[1] ^ 1                    //                           Flip the bit back, so message stays good
		for _, sig := range mSigs[i].sig { //                           Now go through the sigs of the multi-sig
			sig.(*SigEd25519).sig[0] = sig.Signature()[0] ^ 1 //        Flip a bit in the signature
			if mSigs[i].Verify(m) {                           //        Verify we don't accept the message
				t.Error(fmt.Sprintf("found a bad signature %d", i)) //  Blow if we accept a bad sig
			} //
			sig.(*SigEd25519).sig[0] = sig.Signature()[0] ^ 1 //        Repair the bit we flipped
		}
	} //
	var data []byte             //                 Now let's make sure we can marshal and come back.
	for _, sig := range mSigs { //                 Marshal all the multi signatures
		data = append(data, sig.Marshal()...) //   Stack them all together
	} //
	var mSig = new(MultiSig)     //                Get a multi sig structure we can reuse
	for i, m := range messages { //                Go through all multi sigs and the messages they signed
		data = mSig.Unmarshal(data) //             Make sure we can reconstitute the multi-sig
		if !mSig.Verify(m) {        //             And validate all the messages
			t.Errorf("failure to marshal() and Unmarshal() %d", i) // Blow if it fails.
		} //
	}
}
