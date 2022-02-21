package protocol_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

func TestTransactionState(t *testing.T) {
	nts1 := protocol.SendTokens{}
	nts1.AddRecipient(&url.URL{Authority: "BlueWagon", Path: "/account"}, big.NewInt(int64(100*100000000)))

	_, key, _ := ed25519.GenerateKey(rand.Reader)
	addr := acctesting.AcmeLiteAddressStdPriv(key)

	env := acctesting.NewTransaction().
		WithOrigin(addr).
		WithBody(&nts1).
		SignLegacyED25519(key)

	txPendingState := state.NewPendingTransaction(env)
	txPendingState.Url = &url.URL{Authority: "RedWagon", Path: "/myAccount"}
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
	if !txBodyEqual(txPendingState2.TransactionState.Transaction, txPendingState.TransactionState.Transaction) {
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
	if !txBodyEqual(txState2.Transaction, txState.Transaction) {
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

func txBodyEqual(v, u protocol.TransactionPayload) bool {
	switch v := v.(type) {
	case *protocol.CreateIdentity:
		u, ok := u.(*protocol.CreateIdentity)
		return ok && v.Equal(u)
	case *protocol.CreateTokenAccount:
		u, ok := u.(*protocol.CreateTokenAccount)
		return ok && v.Equal(u)
	case *protocol.SendTokens:
		u, ok := u.(*protocol.SendTokens)
		return ok && v.Equal(u)
	case *protocol.CreateDataAccount:
		u, ok := u.(*protocol.CreateDataAccount)
		return ok && v.Equal(u)
	case *protocol.WriteData:
		u, ok := u.(*protocol.WriteData)
		return ok && v.Equal(u)
	case *protocol.WriteDataTo:
		u, ok := u.(*protocol.WriteDataTo)
		return ok && v.Equal(u)
	case *protocol.AcmeFaucet:
		u, ok := u.(*protocol.AcmeFaucet)
		return ok && v.Equal(u)
	case *protocol.CreateToken:
		u, ok := u.(*protocol.CreateToken)
		return ok && v.Equal(u)
	case *protocol.IssueTokens:
		u, ok := u.(*protocol.IssueTokens)
		return ok && v.Equal(u)
	case *protocol.BurnTokens:
		u, ok := u.(*protocol.BurnTokens)
		return ok && v.Equal(u)
	case *protocol.CreateKeyPage:
		u, ok := u.(*protocol.CreateKeyPage)
		return ok && v.Equal(u)
	case *protocol.CreateKeyBook:
		u, ok := u.(*protocol.CreateKeyBook)
		return ok && v.Equal(u)
	case *protocol.AddCredits:
		u, ok := u.(*protocol.AddCredits)
		return ok && v.Equal(u)
	case *protocol.UpdateKeyPage:
		u, ok := u.(*protocol.UpdateKeyPage)
		return ok && v.Equal(u)
	case *protocol.SignPending:
		u, ok := u.(*protocol.SignPending)
		return ok && v.Equal(u)

	case *protocol.SyntheticCreateChain:
		u, ok := u.(*protocol.SyntheticCreateChain)
		return ok && v.Equal(u)
	case *protocol.SyntheticWriteData:
		u, ok := u.(*protocol.SyntheticWriteData)
		return ok && v.Equal(u)
	case *protocol.SyntheticDepositTokens:
		u, ok := u.(*protocol.SyntheticDepositTokens)
		return ok && v.Equal(u)
	case *protocol.SyntheticAnchor:
		u, ok := u.(*protocol.SyntheticAnchor)
		return ok && v.Equal(u)
	case *protocol.SyntheticDepositCredits:
		u, ok := u.(*protocol.SyntheticDepositCredits)
		return ok && v.Equal(u)
	case *protocol.SyntheticBurnTokens:
		u, ok := u.(*protocol.SyntheticBurnTokens)
		return ok && v.Equal(u)
	case *protocol.SyntheticMirror:
		u, ok := u.(*protocol.SyntheticMirror)
		return ok && v.Equal(u)
	case *protocol.SegWitDataEntry:
		u, ok := u.(*protocol.SegWitDataEntry)
		return ok && v.Equal(u)

	case *protocol.InternalGenesis:
		u, ok := u.(*protocol.InternalGenesis)
		return ok && v.Equal(u)
	case *protocol.InternalTransactionsSigned:
		u, ok := u.(*protocol.InternalTransactionsSigned)
		return ok && v.Equal(u)
	case *protocol.InternalTransactionsSent:
		u, ok := u.(*protocol.InternalTransactionsSent)
		return ok && v.Equal(u)

	default:
		panic(fmt.Errorf("unknown transaction type %T", v))
	}
}
