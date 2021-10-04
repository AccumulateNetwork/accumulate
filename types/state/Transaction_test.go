package state

import (
	"bytes"
	"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/types"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
)

func TestTransactionState(t *testing.T) {
	nts1 := transactions.NewTokenSend("RedWagon/myAccount", transactions.Output{100 * 100000000, "BlueWagon"})

	var nonce uint64 = 1

	we := transactions.NewWalletEntry()
	trans := new(transactions.GenTransaction)
	trans.SigInfo = new(transactions.SignatureInfo)
	trans.SigInfo.URL = we.Addr
	trans.SigInfo.Nonce = nonce
	if err := trans.SetRoutingChainID(); err != nil {
		t.Fatal("could not create the Routing value")
	}

	trans.Transaction = nts1.Marshal()

	eSig := new(transactions.ED25519Sig)
	transHash := trans.TransactionHash()

	if err := eSig.Sign(we.Nonce, we.PrivateKey, transHash); err != nil {
		t.Errorf("error signing tx %v", err)
	}

	trans.Signature = append(trans.Signature, eSig)

	txPendingState := NewPendingTransaction(trans)
	data, err := txPendingState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling pending tx state %v", err)
	}

	txPendingState2 := PendingTransaction{}
	err = txPendingState2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("error unmarshaling pending transaction, %v", err)
	}

	if txPendingState.TransactionState.SigInfo == nil {
		t.Fatal("no Sig info")
	}
	if txPendingState.TransactionState.Transaction == nil {
		t.Fatal("no transaction when expecting a transaction")
	}
	if txPendingState.TransactionState.Transaction == nil {
		t.Fatalf("error unmarshaling transaction, it is nil")
	}
	if !bytes.Equal(txPendingState2.TransactionState.Transaction.Bytes(), txPendingState.TransactionState.Transaction.Bytes()) {
		t.Fatalf("error unmarshalling transaction")
	}
	//if bytes.Compare(txPendingState2.Type[:], txPendingState.Type[:]) != 0 {
	//	t.Fatalf("error unmarshaling header")
	//}
	if !txPendingState.TransactionState.SigInfo.Equal(txPendingState2.TransactionState.SigInfo) {
		t.Fatalf("sig info doesn't match")
	}

	txState, txPendingState := NewTransaction(txPendingState)

	if txPendingState.TransactionState.Transaction != nil {
		t.Fatal("expected NO transaction in pending state")
	}

	data, err = txPendingState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling pending tx state %v", err)
	}

	txPendingState2 = PendingTransaction{}
	err = txPendingState2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("error unmarshaling pending transaction, %v", err)
	}

	data, err = txState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling txState %v", err)
	}

	txState2 := Transaction{}
	err = txState2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("error unmarshaling transaction state")
	}

	if txState2.Transaction == nil {
		t.Fatalf("error unmarshaling transaction, it is nil")
	}
	if !bytes.Equal(txState2.Transaction.Bytes(), txState.Transaction.Bytes()) {
		t.Fatalf("error unmarshalling transaction")
	}

	if txState.Type != txState2.Type {
		t.Fatalf("error unmarshaling header")
	}

	if !txState2.SigInfo.Equal(txState.SigInfo) {
		t.Fatalf("sig info doesn't match")
	}

	if !txState2.SigInfo.Equal(txPendingState2.TransactionState.SigInfo) {
		t.Fatalf("sig info doesn't match against pending")
	}

	txPendingStateObject := Object{}
	mdRoot := sha256.Sum256([]byte("some mdroot"))
	txPendingStateObject.MDRoot = mdRoot[:]
	txPendingStateObject.Entry = data
	txPendingStateObject.ChainHeader.SetHeader(types.String("RedWagon/myAccount"), types.ChainTypePendingTransaction)
	pendingStateData, err := txPendingStateObject.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	txPendingStateObject2 := Object{}
	err = txPendingStateObject2.UnmarshalBinary(pendingStateData)
	if err != nil {
		t.Fatal(err)
	}

}
