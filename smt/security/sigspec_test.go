package security

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/common"

	"golang.org/x/crypto/ed25519"
)

func Sign(m int, sigSpec *SigSpec) (msgh [32]byte, mSig *MultiSig) { // m of the n sigSpec keys. Note m <= n
	//                                                                        is not enforced here, so we can test
	//                                                                        bad sigs easy.
	msg := make([]byte, 1000, 1000)                                       // The message will be 1000 bytes
	rand.Read(msg)                                                        // Fill it with random bytes
	var pks [][32]byte                                                    // This will be the slice of private keys
	pks = append(pks, sigSpec.PublicKeys...)                              // and let's randomize the keys in the sigSpec
	sort.Slice(pks, func(_, _ int) bool { return rand.Int31n(100) < 50 }) //  by sorting with random swaps
	mSig = new(MultiSig)                                                  // Now create a MultiSig that will sign msg
	for i := 0; i < m; i++ {                                              // Now for all the signatures that is desired
		var sig = new(SigEd25519) //                                      Create each signature
		if i < len(pks) {         //                                      If we still have valid public keys
			sig.publicKey = pks[i][:] //                                        in the sig spec use that.
		} else { //                                                          if we don't then...
			pk := TestWallet.GetKeys(1)[0].PublicKey() //                       Just get a key and use that which isn't
			sig.publicKey = pk[:]                      //                       a member of the SigSpec necessarily
		} //
		msgh = sha256.Sum256(msg)
		sig.sig = ed25519.Sign(TestWallet[pks[i]], msgh[:]) //                   Sign the message
		mSig.AddSig(msgh, sig)                              //                   Add it to the list in the mSig
	}
	return msgh, mSig //                                                      Return the msg signed, and the mSig signer
}

// Create a set of tests and validate them.
func TestSigSpec_Verify(t *testing.T) {
	t.Skip("ignore")
	totalTests := 0
	var data []byte           //                                    data holds the marshaled tests
	for i := 0; i < 10; i++ { //                                    Create 50 test sets
		for cnt := 1; cnt <= 10; cnt++ { //                         cnt will be the number of sigs in each sigSpec
			sigSpec := new(SigSpec)        //                       Create a sigSpec
			pks := TestWallet.GetKeys(cnt) //                       Get the number of keys needed by the sigSpec
			for _, k := range pks {        //                       For each of the keys
				sigSpec.AddSig(k.PublicKey()) //                      add them to the sigSpec
			} //                                                    Now we need to build the set of required m
			for j := 1; j <= cnt; j++ { //                          values (how many of the sigs we need to validate)
				sigSpec.m = int64(j)          //                    Change m
				msg, mSig := Sign(j, sigSpec) //                    Create a signature that meets the requirement
				//                                                    of the sigSpec
				data = append(data, common.SliceBytes(msg[:])...) //  Marshal the message
				data = append(data, sigSpec.Marshal()...)         //  Marshal the sigSpec
				data = append(data, mSig.Marshal()...)            //  Marshal the MultiSig
				//
				SigSpecValid := sigSpec.Verify(mSig) //             Attempt to verify the mSig against the sigSpec
				mSigValid := mSig.Verify(msg)        //             Attempt to verify the msg against the mSig
				//
				if !mSigValid { //                                        The mSig should verify the msg
					t.Fatal(fmt.Sprintf("failed to validate mSig "+ //    And if not, fail! And fail big
						" cnt=%d j=%d", cnt, j)) //
				}
				if !SigSpecValid { //                                     The SigSpec should verify the mSig
					t.Fatal(fmt.Sprintf("failed to validate sigSpec "+ // And if not, fail!
						" cnt=%d j=%d", cnt, j)) //
				} //
				totalTests++ //                                           Count the tests for fun.
			} //
		} //
	} //

	d := data                         //                          Save data as a starting point
	for i := 0; i < totalTests; i++ { //                          Run through the tests
		SigSpec := new(SigSpec)         //                       UnMarshal
		mSig := new(MultiSig)           //
		msg, d_ := common.BytesSlice(d) //
		var msgh [32]byte               //
		copy(msgh[:], msg)
		d = SigSpec.Unmarshal(d_) //
		d = mSig.Unmarshal(d)     //
		//
		SigSpecValid := SigSpec.Verify(mSig) //                   Verify the mSig using the SigSpec
		mSigValid := mSig.Verify(msgh)       //                   Verify the msg using the mSig
		//
		if !mSigValid { //                                        Report if the msg fails to validate
			t.Fatal(fmt.Sprintf("failed to validate mSig")) //
		} //
		if !SigSpecValid { //                                     Report if the MultiSig fails to validate
			t.Fatal(fmt.Sprintf("failed to validate SigSpec")) //
		} //
	} //
	if len(d) != 0 {
		t.Error("all tests should be processed")
	}

	d = data                          //                          Now add unused signatures
	for i := 0; i < totalTests; i++ { //
		SigSpec := new(SigSpec)         //
		mSig := new(MultiSig)           //
		msg, d_ := common.BytesSlice(d) //
		var msgh [32]byte               //
		copy(msgh[:], msg)
		d = SigSpec.Unmarshal(d_) //
		d = mSig.Unmarshal(d)     //
		//
		pks := TestWallet.GetKeys(rand.Intn(4) + 1)
		for _, pub := range pks { //                                               Signatures get some invalid sigs
			var sig = new(SigEd25519)                                           // The mSig STILL has enough
			sig.publicKey = pub[:]                                              // valid signatures, so these
			sig.sig = ed25519.Sign(ed25519.PrivateKey(pub), []byte{1, 2, 3, 4}) // should still validate.
		}
		//
		SigSpecValid := SigSpec.Verify(mSig) //
		mSigValid := mSig.Verify(msgh)       //
		//
		if !mSigValid { //
			t.Fatal(fmt.Sprintf("failed to validate mSig")) //
		} //
		if !SigSpecValid { //
			t.Fatal(fmt.Sprintf("failed to validate SigSpec")) //
		} //
	} //
	if len(d) != 0 {
		t.Error("all tests should be processed")
	}

	d = data                          //                          Remove some valid signatures. Replace 0 or more
	for i := 0; i < totalTests; i++ { //                          with bad signatures.
		SigSpec := new(SigSpec)         //
		mSig := new(MultiSig)           //
		msg, d_ := common.BytesSlice(d) //
		var msgh [32]byte               //
		copy(msgh[:], msg)
		d = SigSpec.Unmarshal(d_) //
		d = mSig.Unmarshal(d)     //
		//
		remove := rand.Intn(int(SigSpec.m) + 1)                                  // Remove 1 or more signatures
		sort.Slice(mSig.sig, func(_, _ int) bool { return rand.Intn(100) < 50 }) // Shuffle signatures to make removal
		mSig.sig = mSig.sig[remove:]                                             // random, and remove them.
		//
		pks := TestWallet.GetKeys(rand.Intn(int(SigSpec.m))) //                    Add some number of bad signatures
		for _, pub := range pks {                            //                    Signatures get some invalid sigs
			var sig = new(SigEd25519)                                           // The mSig STILL has enough
			sig.publicKey = pub[:]                                              // valid signatures, so these
			sig.sig = ed25519.Sign(ed25519.PrivateKey(pub), []byte{1, 2, 3, 4}) // should still validate.
		}
		//
		SigSpecValid := SigSpec.Verify(mSig) //
		mSigValid := mSig.Verify(msgh)       //
		//
		if mSigValid { //
			t.Fatal(fmt.Sprintf("failed to detect bad mSig")) //
		} //
		if SigSpecValid { //
			t.Fatal(fmt.Sprintf("failed to  SigSpec")) //
		} //
	} //
	if len(d) != 0 {
		t.Error("all tests should be processed")
	}
}
