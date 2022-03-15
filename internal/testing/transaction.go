package testing

import (
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TransactionBuilder struct {
	*protocol.Envelope
}

func NewTransaction() TransactionBuilder {
	var tb TransactionBuilder
	tb.Envelope = new(protocol.Envelope)
	tb.Transaction = new(protocol.Transaction)
	return tb
}

func (tb TransactionBuilder) WithHeader(hdr *protocol.TransactionHeader) TransactionBuilder {
	tb.Transaction.TransactionHeader = *hdr
	return tb
}

func (tb TransactionBuilder) WithOrigin(origin *url.URL) TransactionBuilder {
	tb.Transaction.Origin = origin
	return tb
}

func (tb TransactionBuilder) WithOriginStr(origin string) TransactionBuilder {
	originURL, err := url.Parse(origin)
	if err != nil {
		panic(err)
	}
	tb.Transaction.Origin = originURL
	return tb
}

func (tb TransactionBuilder) WithKeyPage(index, height uint64) TransactionBuilder {
	tb.Transaction.KeyPageIndex = index
	tb.Transaction.KeyPageHeight = height
	return tb
}

func (tb TransactionBuilder) WithTimestamp(timestamp uint64) TransactionBuilder {
	tb.Transaction.Timestamp = timestamp
	return tb
}

func (tb TransactionBuilder) WithNonceVar(timestamp *uint64) TransactionBuilder {
	tb.Transaction.Timestamp = atomic.AddUint64(timestamp, 1)
	return tb
}

func (tb TransactionBuilder) WithCurrentTimestamp() TransactionBuilder {
	tb.Transaction.Timestamp = uint64(time.Now().UTC().UnixNano())
	return tb
}

func (tb TransactionBuilder) WithBody(body protocol.TransactionBody) TransactionBuilder {
	tb.Transaction.Body = body
	return tb
}

func (tb TransactionBuilder) WithTxnHash(hash []byte) TransactionBuilder {
	tb.TxHash = hash
	return tb
}

func (tb TransactionBuilder) Sign(signer func(timestamp uint64, hash []byte) (protocol.Signature, error)) *protocol.Envelope {
	sig, err := signer(tb.Transaction.Timestamp, tb.GetTxHash())
	if err != nil {
		panic(err)
	}

	tb.Signatures = append(tb.Signatures, sig)
	return tb.Envelope
}

func (tb TransactionBuilder) SignLegacyED25519(key []byte) *protocol.Envelope {
	return tb.Sign(func(nonce uint64, hash []byte) (protocol.Signature, error) {
		sig := new(protocol.LegacyED25519Signature)
		return sig, sig.Sign(nonce, key, hash)
	})
}
