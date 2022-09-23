package testing

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TransactionBuilder struct {
	transaction build.TransactionBuilder
	signer      signing.Builder
	signatures  []protocol.Signature
	SkipChecks  bool
}

func NewTransaction() TransactionBuilder {
	var tb TransactionBuilder
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
	return tb.WithHeader(&txn.Header).WithBody(txn.Body)
}

func (tb TransactionBuilder) WithHeader(hdr *protocol.TransactionHeader) TransactionBuilder {
	tb.transaction = tb.transaction.
		Principal(hdr.Principal).
		Initiator(hdr.Initiator).
		Memo(hdr.Memo).
		Metadata(hdr.Metadata)
	return tb
}

func (tb TransactionBuilder) WithPrincipal(origin *url.URL) TransactionBuilder {
	tb.transaction = tb.transaction.Principal(origin)
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
	tb.transaction = tb.transaction.Body(body)
	return tb
}

func (tb TransactionBuilder) WithTxnHash(hash []byte) TransactionBuilder {
	body := new(protocol.RemoteTransaction)
	body.Hash = *(*[32]byte)(hash)
	return tb.WithBody(body)
}

func (tb *TransactionBuilder) buildTxn() *protocol.Transaction {
	txn, err := tb.transaction.Build()
	if err != nil {
		panic(err)
	}
	return txn
}

func (tb TransactionBuilder) Sign(typ protocol.SignatureType, privateKey []byte) TransactionBuilder {
	txn := tb.buildTxn()
	switch {
	case tb.SkipChecks:
		// Skip checks
	case txn.Body == nil:
		panic("cannot sign a transaction without the transaction body or transaction hash")
	case txn.Header.Initiator == ([32]byte{}) && txn.Body.Type() != protocol.TransactionTypeRemote:
		panic("cannot sign a transaction before setting the initiator")
	}

	tb.signer.SetPrivateKey(privateKey)
	sig, err := tb.signer.Sign(txn.GetHash())
	if err != nil {
		panic(err)
	}

	tb.signatures = append(tb.signatures, sig)
	return tb
}

func (tb TransactionBuilder) SignFunc(fn func(txn *protocol.Transaction) protocol.Signature) TransactionBuilder {
	tb.signatures = append(tb.signatures, fn(tb.buildTxn()))
	return tb
}

func (tb TransactionBuilder) Initiate(typ protocol.SignatureType, privateKey []byte) TransactionBuilder {
	txn := tb.buildTxn()
	switch {
	case tb.SkipChecks:
		// Skip checks
	case txn.Body == nil:
		panic("cannot initiate transaction without a body")
	case txn.Body.Type() == protocol.TransactionTypeRemote:
		panic("cannot initiate transaction: have hash instead of body")
	case txn.Header.Initiator != ([32]byte{}):
		panic("cannot initiate transaction: already initiated")
	}

	tb.signer.Type = typ
	tb.signer.SetPrivateKey(privateKey)
	sig, err := tb.signer.Initiate(txn)
	if err != nil {
		panic(err)
	}

	tb.transaction = tb.transaction.Initiator(txn.Header.Initiator)
	tb.signatures = append(tb.signatures, sig)
	return tb
}

func (tb TransactionBuilder) Build() *protocol.Envelope {
	env := new(protocol.Envelope)
	env.Transaction = []*protocol.Transaction{tb.buildTxn()}
	env.Signatures = tb.signatures
	sort.Slice(env.Transaction, func(i, j int) bool {
		return bytes.Compare(env.Transaction[i].GetHash(), env.Transaction[j].GetHash()) < 0
	})
	return env
}

func (tb TransactionBuilder) BuildDelivery() *chain.Delivery {
	delivery, err := chain.NormalizeEnvelope(tb.Build())
	if err != nil {
		panic(err)
	}
	if len(delivery) != 1 {
		panic(fmt.Errorf("expected 1 delivery, got %d", len(delivery)))
	}
	return delivery[0]
}

func (tb TransactionBuilder) InitiateSynthetic(destPartitionUrl *url.URL) TransactionBuilder {
	txn := tb.buildTxn()
	if txn.Header.Initiator != ([32]byte{}) {
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

	txn.Header.Initiator = *(*[32]byte)(initSig.Metadata().Hash())
	initSig.TransactionHash = *(*[32]byte)(txn.GetHash())
	tb.signatures = append(tb.signatures, initSig)
	return tb
}

func (tb TransactionBuilder) Faucet() *protocol.Envelope {
	sig, err := new(signing.Builder).UseFaucet().Initiate(tb.buildTxn())
	if err != nil {
		panic(err)
	}

	tb.signatures = append(tb.signatures, sig)
	return tb.Build()
}
