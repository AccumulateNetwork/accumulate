package build

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func SignatureForHash(hash []byte) SignatureBuilder {
	txn := new(protocol.Transaction)
	txn.Body = &protocol.RemoteTransaction{Hash: *(*[32]byte)(hash)}
	return SignatureBuilder{transaction: txn}
}

func SignatureForTransaction(txn *protocol.Transaction) SignatureBuilder {
	return SignatureBuilder{transaction: txn}
}

func Transaction(principal any, path ...string) TransactionBuilder {
	return TransactionBuilder{}.Principal(principal, path...)
}

// UnixTimeNow returns the current time as a number of milliseconds since the
// Unix epoch. This is the recommended timestamp value.
func UnixTimeNow() uint64 {
	return uint64(time.Now().UTC().UnixMilli())
}
