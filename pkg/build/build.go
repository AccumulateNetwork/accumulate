// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func SignatureForHash(hash []byte) SignatureBuilder {
	txn := new(protocol.Transaction)
	txn.Body = &protocol.RemoteTransaction{Hash: *(*[32]byte)(hash)}
	return SignatureBuilder{transaction: txn}
}

func SignatureForTxID(txid *url.TxID) SignatureBuilder {
	txn := new(protocol.Transaction)
	txn.Header.Principal = txid.Account()
	txn.Body = &protocol.RemoteTransaction{Hash: txid.Hash()}
	return SignatureBuilder{transaction: txn}
}

func SignatureForTransaction(txn *protocol.Transaction) SignatureBuilder {
	return SignatureBuilder{transaction: txn}
}

func SignatureForMessage(msg messaging.Message) SignatureBuilder {
	return SignatureBuilder{message: msg}
}

func Transaction() TransactionBuilder {
	return TransactionBuilder{}
}

// Load loads the transaction using the querier. Load panics if the existing
// transaction is not remote.
func (b SignatureBuilder) Load(q api.Querier) SignatureBuilder {
	if !b.ok() {
		return b
	}
	if _, ok := b.transaction.Body.(*protocol.RemoteTransaction); !ok {
		panic("not a remote transaction")
	}
	r, err := api.Querier2{Querier: q}.QueryTransaction(context.Background(), b.transaction.ID(), nil)
	if err != nil {
		b.errs = append(b.errs, err)
		return b
	}
	b.transaction = r.Message.Transaction
	return b
}

// UnixTimeNow returns the current time as a number of milliseconds since the
// Unix epoch. This is the recommended timestamp value.
func UnixTimeNow() uint64 {
	return uint64(time.Now().UTC().UnixMilli())
}

func Faucet(recipient any, path ...string) SignatureBuilder {
	f := protocol.Faucet.Signer()
	b := Transaction().For(protocol.FaucetUrl)
	u := b.parseUrl(recipient, path...)
	b = b.Body(&protocol.AcmeFaucet{Url: u})
	return b.SignWith(protocol.FaucetUrl).
		Version(f.Version()).
		Timestamp(f.Timestamp()).
		Signer(f)
}
