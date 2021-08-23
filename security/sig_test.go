package security

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	"github.com/AccumulateNetwork/SMT/common"

	"golang.org/x/crypto/ed25519"
)

// buildSigs
// Build some signed messages, along with with private and public keys
func buildSigs(Seed []byte, NumTests int) ( //                     Generate NumTests number of test vectors
	sigStruct []Sig, //                                            Create the Sig struct
	msgHashes [][32]byte, //                                         Return a set of message hashes
	sig, privateKey, publicKey [][]byte, //                        A Test Vector  (msg/sig/private/public)
	data []byte, //                                                And a marshaled data representation of the vectors
) { //                                                               a public/private key pair.
	seed := sha256.Sum256(Seed)
	var buff [1234]byte             //                             Going to hold random messages here
	for i := 0; i < NumTests; i++ { //                             Collect this tests
		rand.Read(buff[:])                                             // Get a random selection of bytes
		private := ed25519.NewKeyFromSeed(seed[:])                     // Compute the public and private key
		public := private[32:]                                         // Public key is the last 32 bytes
		seed = sha256.Sum256(seed[:])                                  // Increment the seed
		msgHash := sha256.Sum256(buff[:])                              // Sign the msgHashes
		s := ed25519.Sign(private, msgHash[:])                         // Sign the msgHashes
		aSigStruct := new(SigEd25519)                                  // Create Sig instance
		aSigStruct.sig = s                                             // Populate it with the signature
		aSigStruct.publicKey = append(aSigStruct.publicKey, public...) //   and the public key

		// At this point we have the msgHashes, signature, private key,
		// and public key.  Now build the test vector

		msgHashes = append(msgHashes, msgHash)    //        Add msgHashes (make copy because we reuse buff)
		sig = append(sig, s)                      //        Add signature
		privateKey = append(privateKey, private)  //        Add private Key
		publicKey = append(publicKey, public)     //        Add public key
		sigStruct = append(sigStruct, aSigStruct) //        Add a signature

		data = append(data, common.Int64Bytes(int64(SEd25519))...) //    Encode signatures type
		data = append(data, public...)                             //    Encode public key
		data = append(data, s...)                                  //    Encode signature

	}
	return sigStruct, msgHashes, sig, privateKey, publicKey, data
}

func TestUnmarshal(t *testing.T) {
	s := []byte{1, 2, 3, 4, 5}
	sigs, msg, signatures, private, public, data := buildSigs(s, 100) // Generate a set of test vectors
	_ = signatures
	_ = private
	_ = public
	for i, m := range msg { // for all the data in Data
		sig, newData := Unmarshal(data) //                             Unmarshal one of the test vectors (don't advance)
		if !sig.Equal(sigs[i]) {        //                             Make sure the constructed sig == Unmarshal()
			t.Error(fmt.Sprintf("Sig built != Unmarshal()")) //
		} //
		if !sig.Verify(nil, 0, m) { //                                 If the message doesn't verify, something failed
			t.Error(fmt.Sprintf("Sig %d failed to verify", i)) //
		} //
		testData := sig.Marshal()                         //           Now marshal my sig.
		if !bytes.Equal(testData, data[:len(testData)]) { //           Did that match what we pulled from data?
			t.Error(fmt.Sprintf("Marshal failed %d", i)) //            Bad if it doesn't
		} //
		data = newData //                                              NOW we can advance data
	}
}
