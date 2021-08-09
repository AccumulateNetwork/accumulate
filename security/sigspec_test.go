package security

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/AccumulateNetwork/SMT/managed"

	"golang.org/x/crypto/ed25519"
)

func Sign(m int, sigSpec *SigSpec) (msg []byte, mSig *MultiSig) {
	msg = make([]byte, 1000, 1000)
	rand.Read(msg)
	var pks [][32]byte
	pks = append(pks, sigSpec.PublicKeys...)
	sort.Slice(pks, func(_, _ int) bool { return rand.Int31n(100) < 50 })
	mSig = new(MultiSig)
	for i := 0; i < m; i++ {
		var sig = new(SigEd25519)
		sig.publicKey = pks[i][:]
		sig.sig = ed25519.Sign(TestWallet[pks[i]], msg)
		mSig.AddSig(sig)
	}
	return msg, mSig
}

func TestSigSpec_Verify(t *testing.T) {
	totalTests := 0
	var data []byte
	for i := 0; i < 50; i++ {
		for cnt := 1; cnt <= 10; cnt++ {
			pks := TestWallet.GetKeys(cnt)
			SigSpec := new(SigSpec)
			for _, k := range pks {
				SigSpec.AddSig(k.PublicKey())
			}
			for j := 1; j <= cnt; j++ {
				SigSpec.m = int64(j)
				msg, mSig := Sign(j, SigSpec)

				data = append(data, managed.SliceBytes(msg)...)
				data = append(data, SigSpec.Marshal()...)
				data = append(data, mSig.Marshal()...)

				SigSpecValid := SigSpec.Verify(mSig)
				mSigValid := mSig.Verify(msg)

				if !mSigValid {
					t.Fatal(fmt.Sprintf("failed to validate mSig "+
						" cnt=%d j=%d", cnt, j))
				}
				if !SigSpecValid {
					t.Fatal(fmt.Sprintf("failed to validate SigSpec "+
						" cnt=%d j=%d", cnt, j))
				}
				totalTests++
			}
		}
	}
	for len(data) > 0 {
		SigSpec := new(SigSpec)
		mSig := new(MultiSig)
		msg, d := managed.BytesSlice(data)
		d = SigSpec.Unmarshal(d)
		data = mSig.Unmarshal(d)

		SigSpecValid := SigSpec.Verify(mSig)
		mSigValid := mSig.Verify(msg)

		if !mSigValid {
			t.Fatal(fmt.Sprintf("failed to validate mSig"))
		}
		if !SigSpecValid {
			t.Fatal(fmt.Sprintf("failed to validate SigSpec"))
		}
	}
}
