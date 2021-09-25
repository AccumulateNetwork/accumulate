package security

import (
	"bytes"
	"testing"
)

func TryTheWalletLimit(count int) (errorThrown bool) {
	defer func() {
		errorThrown = recover() != nil
	}()
	TestWallet.GetKeys(count)
	return false
}

func TestTheWallet(t *testing.T) {
	if TryTheWalletLimit(1) {
		t.Error("should not throw an error")
	}
	if !TryTheWalletLimit(101) {
		t.Error("should thrown an error")
	}
	pks := TestWallet.GetKeys(100)
	for i := 1; i < 100; i++ {
		pkMap := make(map[[64]byte]int)
		for i, k := range pks {
			var pk [64]byte
			copy(pk[:], k)
			pkMap[pk] = i
		}
		if len(pkMap) != len(pks) {
			t.Error("should have no duplicates")
		}
		for _, k := range pks {
			pub := k.PublicKey()
			if !bytes.Equal(k[32:], pub[:]) {
				t.Error("function PublicKey() does not return the public key")
			}
		}
	}
}
