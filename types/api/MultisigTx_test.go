package api

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestMultiSigTx(t *testing.T) {
	msTx := &MultiSigTx{}
	msTx.TxHash = sha256.Sum256([]byte("testTxId"))

	data, err := msTx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	msTx2 := &MultiSigTx{}
	err = msTx2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(msTx.TxHash[:], msTx2.TxHash[:]) != 0 {
		t.Fatalf("umarshalling failed, txhash doesn't match")
	}
}
