package protocol_test

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

func TestTransactionState(t *testing.T) {
	nts1 := protocol.SendTokens{}
	nts1.AddRecipient(&url.URL{Authority: "BlueWagon", Path: "/account"}, big.NewInt(int64(100*100000000)))

	we := acctesting.NewWalletEntry()
	u, err := url.Parse(we.Addr)
	require.NoError(t, err)
	trans := new(Envelope)
	trans.Transaction = new(Transaction)
	trans.Transaction.Origin = u

	trans.Transaction.Body, err = nts1.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	eSig := new(ED25519Sig)
	transHash := trans.GetTxHash()

	if err := eSig.Sign(we.Nonce, we.PrivateKey, transHash); err != nil {
		t.Errorf("error signing tx %v", err)
	}

	trans.Signatures = append(trans.Signatures, eSig)

	txPendingState := state.NewPendingTransaction(trans)
	txPendingState.Url = "RedWagon/myAccount"
	data, err := txPendingState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling pending tx state %v", err)
	}

	txPendingState2 := PendingTransactionState{}

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
	if !bytes.Equal(txPendingState2.TransactionState.Transaction, txPendingState.TransactionState.Transaction) {
		t.Fatalf("error unmarshalling transaction")
	}
	//if bytes.Compare(txPendingState2.Type[:], txPendingState.Type[:]) != 0 {
	//	t.Fatalf("error unmarshaling header")
	//}
	if !txPendingState.TransactionState.SigInfo.Equal(txPendingState2.TransactionState.SigInfo) {
		t.Fatalf("sig info doesn't match")
	}

	txState, txPendingState := state.NewTransaction(txPendingState)

	if txPendingState.TransactionState.Transaction != nil {
		t.Fatal("expected NO transaction in pending state")
	}

	data, err = txPendingState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling pending tx state %v", err)
	}

	txPendingState2 = PendingTransactionState{}
	err = txPendingState2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("error unmarshaling pending transaction, %v", err)
	}

	data, err = txState.MarshalBinary()
	if err != nil {
		t.Fatalf("error marshaling txState %v", err)
	}

	txState2 := TransactionState{}
	err = txState2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("error unmarshaling transaction state")
	}

	if txState2.Transaction == nil {
		t.Fatalf("error unmarshaling transaction, it is nil")
	}
	if !bytes.Equal(txState2.Transaction, txState.Transaction) {
		t.Fatalf("error unmarshalling transaction")
	}

	if !txState2.SigInfo.Equal(txState.SigInfo) {
		t.Fatalf("sig info doesn't match")
	}

	if !txState2.SigInfo.Equal(txPendingState2.TransactionState.SigInfo) {
		t.Fatalf("sig info doesn't match against pending")
	}

	txPendingStateObject := Object{}
	root := sha256.Sum256([]byte("some mdroot"))
	txPendingStateObject.Roots = [][]byte{root[:]}
	txPendingStateObject.Height = 1
	txPendingStateObject.Entry = data
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
