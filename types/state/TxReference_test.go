package state

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestTxIdRef(t *testing.T) {

	txId := sha256.Sum256([]byte("test txhash"))
	txr := NewTxReference("acme-xxxxx", txId[:])

	data, err := txr.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	txr2 := TxReference{}
	err = txr2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(txr.TxId[:], txr2.TxId[:]) {
		t.Fatal(err)
	}
}
