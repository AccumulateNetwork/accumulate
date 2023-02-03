// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TransactionBuilder struct {
	*protocol.Envelope
	signer     signing.Builder
	SkipChecks bool
}

func NewTransaction() TransactionBuilder {
	var tb TransactionBuilder
	tb.Envelope = new(protocol.Envelope)
	tb.Transaction = []*protocol.Transaction{new(protocol.Transaction)}
	return tb
}

// Unsafe allows the caller to generate invalid signatures
func (tb TransactionBuilder) Unsafe() TransactionBuilder {
	tb.SkipChecks = true
	return tb
}

func (tb TransactionBuilder) SetSigner(s *signing.Builder) TransactionBuilder {
	tb.signer = *s
	return tb
}

func (tb TransactionBuilder) UseSimpleHash() TransactionBuilder {
	tb.signer.UseSimpleHash()
	return tb
}

func (tb TransactionBuilder) WithTransaction(txn *protocol.Transaction) TransactionBuilder {
	tb.Transaction[0] = txn
	return tb
}

func (tb TransactionBuilder) WithHeader(hdr *protocol.TransactionHeader) TransactionBuilder {
	tb.Transaction[0].Header = *hdr
	return tb
}

func (tb TransactionBuilder) WithPrincipal(origin *url.URL) TransactionBuilder {
	tb.Transaction[0].Header.Principal = origin
	return tb
}

func (tb TransactionBuilder) WithDelegator(delegator *url.URL) TransactionBuilder {
	tb.signer.AddDelegator(delegator)
	return tb
}

func (tb TransactionBuilder) WithSigner(signer *url.URL, height uint64) TransactionBuilder {
	tb.signer.SetUrl(signer)
	tb.signer.SetVersion(height)
	return tb
}

func (tb TransactionBuilder) WithTimestamp(nonce uint64) TransactionBuilder {
	tb.signer.SetTimestamp(nonce)
	return tb
}

func (tb TransactionBuilder) WithTimestampVar(nonce *uint64) TransactionBuilder {
	tb.signer.SetTimestampWithVar(nonce)
	return tb
}

func (tb TransactionBuilder) WithCurrentTimestamp() TransactionBuilder {
	tb.signer.SetTimestamp(uint64(time.Now().UTC().UnixMilli()))
	return tb
}

func (tb TransactionBuilder) WithBody(body protocol.TransactionBody) TransactionBuilder {
	tb.Transaction[0].Body = body
	return tb
}

func (tb TransactionBuilder) WithTxnHash(hash []byte) TransactionBuilder {
	body := new(protocol.RemoteTransaction)
	body.Hash = *(*[32]byte)(hash)
	return tb.WithBody(body)
}

func (tb TransactionBuilder) Sign(typ protocol.SignatureType, privateKey []byte) TransactionBuilder {
	switch {
	case tb.SkipChecks:
		// Skip checks
	case tb.Transaction[0].Body == nil:
		panic("cannot sign a transaction without the transaction body or transaction hash")
	case tb.Transaction[0].Header.Initiator == ([32]byte{}) && tb.Transaction[0].Body.Type() != protocol.TransactionTypeRemote:
		panic("cannot sign a transaction before setting the initiator")
	}

	tb.signer.SetPrivateKey(privateKey)
	sig, err := tb.signer.Sign(tb.Transaction[0].GetHash())
	if err != nil {
		panic(err)
	}

	tb.Signatures = append(tb.Signatures, sig)
	return tb
}

func (tb TransactionBuilder) SignFunc(fn func(txn *protocol.Transaction) protocol.Signature) TransactionBuilder {
	tb.Signatures = append(tb.Signatures, fn(tb.Transaction[0]))
	return tb
}

func (tb TransactionBuilder) Initiate(typ protocol.SignatureType, privateKey []byte) TransactionBuilder {
	switch {
	case tb.SkipChecks:
		// Skip checks
	case tb.Transaction[0].Body == nil:
		panic("cannot initiate transaction without a body")
	case tb.Transaction[0].Body.Type() == protocol.TransactionTypeRemote:
		panic("cannot initiate transaction: have hash instead of body")
	case tb.Transaction[0].Header.Initiator != ([32]byte{}):
		panic("cannot initiate transaction: already initiated")
	}

	tb.signer.Type = typ
	tb.signer.SetPrivateKey(privateKey)
	sig, err := tb.signer.Initiate(tb.Transaction[0])
	if err != nil {
		panic(err)
	}

	tb.Signatures = append(tb.Signatures, sig)
	return tb
}

func (tb TransactionBuilder) Build() *protocol.Envelope {
	return tb.Envelope
}

func (tb TransactionBuilder) BuildDelivery() *chain.Delivery {
	delivery, err := chain.NormalizeEnvelope(tb.Envelope)
	if err != nil {
		panic(err)
	}
	if len(delivery) != 1 {
		panic(fmt.Errorf("expected 1 delivery, got %d", len(delivery)))
	}
	return delivery[0]
}

func (tb TransactionBuilder) InitiateSynthetic(destPartitionUrl *url.URL) TransactionBuilder {
	if tb.TxHash != nil {
		panic("cannot initiate transaction: have hash instead of body")
	}
	if tb.Transaction[0].Header.Initiator != ([32]byte{}) {
		panic("cannot initiate transaction: already initiated")
	}
	if tb.signer.Url == nil {
		panic("missing signer")
	}
	if tb.signer.Version == 0 {
		panic("missing version")
	}

	initSig := new(protocol.PartitionSignature)
	initSig.SourceNetwork = tb.signer.Url
	initSig.DestinationNetwork = destPartitionUrl
	initSig.SequenceNumber = tb.signer.Version

	tb.Transaction[0].Header.Initiator = *(*[32]byte)(initSig.Metadata().Hash())
	initSig.TransactionHash = *(*[32]byte)(tb.Transaction[0].GetHash())
	tb.Signatures = append(tb.Signatures, initSig)
	return tb
}

func (tb TransactionBuilder) Faucet() *protocol.Envelope {
	sig, err := new(signing.Builder).UseFaucet().Initiate(tb.Transaction[0])
	if err != nil {
		panic(err)
	}

	tb.Signatures = append(tb.Signatures, sig)
	return tb.Envelope
}
