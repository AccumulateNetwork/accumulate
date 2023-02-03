// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var startTime = uint64(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

type bldTxn struct {
	transaction *protocol.Transaction
	signatures  []protocol.Signature
	s           *Session
	b           *signing.Builder
}

func (s *Session) WaitForSubmitted() []completedFlow {
	completed := make([]completedFlow, len(s.submitted))
	for i, sub := range s.submitted {
		completed[i] = sub.Wait()
	}
	s.submitted = s.submitted[0:]
	return completed
}

func (s *Session) SetStartTime(time time.Time) {
	s.timestamp = uint64(time.Unix())
}

func (s *Session) Transaction(principal Urlish) bldTxn {
	// Start the timestamp at 2022-1-1 00:00:00
	for atomic.LoadUint64(&s.timestamp) == 0 {
		if atomic.CompareAndSwapUint64(&s.timestamp, 0, startTime) {
			break
		}
	}

	var b bldTxn
	b.transaction = new(protocol.Transaction)
	b.transaction.Header.Principal = s.url(principal)
	b.s = s
	b.b = new(signing.Builder)
	b.b.InitMode = signing.InitWithSimpleHash
	b.b.Type = protocol.SignatureTypeED25519
	b.b.SetTimestampWithVar(&s.timestamp)
	return b
}

func (b bldTxn) WithMemo(memo string) bldTxn {
	b.transaction.Header.Memo = memo
	return b
}

func (b bldTxn) WithHash(hash []byte) bldTxn {
	return b.WithBody(&protocol.RemoteTransaction{Hash: *(*[32]byte)(hash)})
}

func (b bldTxn) WithRemote(url Urlish, hash []byte) bldTxn {
	b.transaction.Header.Principal = b.s.url(url)
	return b.WithBody(&protocol.RemoteTransaction{Hash: *(*[32]byte)(hash)})
}

func (b bldTxn) WithPending(txn interface{ submitted() *protocol.Transaction }) bldTxn {
	b.transaction = txn.submitted()
	return b
}

func (b bldTxn) WithSigner(url Urlish, version ...uint64) bldTxn {
	b.b.Url = b.s.url(url)
	if len(version) > 0 {
		b.b.Version = version[0]
		return b
	}

	signerUrl := b.b.Url
	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	var signer protocol.Signer
	b.s.GetAccountAs(signerUrl, &signer)
	b.b.Version = signer.GetVersion()
	return b
}

func (b bldTxn) WithDelegator(url Urlish) bldTxn {
	b.b.AddDelegator(b.s.url(url))
	return b
}

func (b bldTxn) WithBody(body protocol.TransactionBody) bldTxn {
	b.transaction.Body = body
	return b
}

func (b bldTxn) readyToSign(key interface{}, init bool) {
	b.b.SetPrivateKey(b.s.privkey(key))

	if b.transaction.Body == nil {
		b.s.Abort("Missing transaction body")
	}
}

func (b bldTxn) Sign(key interface{}) bldTxn {
	b.readyToSign(key, false)
	b.b.Timestamp = nil
	sig, err := b.b.Sign(b.transaction.GetHash())
	if err != nil {
		b.s.Abortf("Failed to sign: %v", err)
	}
	b.signatures = append(b.signatures, sig)
	return b
}

func (b bldTxn) Initiate(key interface{}) bldTxn {
	b.readyToSign(key, true)
	sig, err := b.b.Initiate(b.transaction)
	if err != nil {
		b.s.Abortf("Failed to initiate: %v", err)
	}
	b.signatures = append(b.signatures, sig)
	return b
}

func (b bldTxn) Submit() *submittedTxn {
	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{b.transaction}
	env.Signatures = b.signatures
	status, err := b.s.Engine.Submit(env)
	if err != nil {
		b.s.Abortf("Failed to submit transaction: %v", err)
	}

	hash := *(*[32]byte)(b.transaction.GetHash())
	sub := new(submittedTxn)
	sub.s = b.s
	sub.Transaction = b.transaction
	sub.Hash = hash
	sub.Status = status
	b.s.submitted = append(b.s.submitted, sub)
	return sub
}

func (s *Session) Faucet(account Urlish) *submittedTxn {
	f := protocol.Faucet.Signer()
	b := s.Transaction(protocol.FaucetUrl)
	b.transaction.Body = &protocol.AcmeFaucet{Url: s.url(account)}
	b.b.Signer = f
	b.b.Url = protocol.FaucetUrl
	b.b.Version = f.Version()

	sig, err := b.b.Initiate(b.transaction)
	if err != nil {
		b.s.Abortf("Failed to initiate: %v", err)
	}
	b.signatures = append(b.signatures, sig)
	return b.Submit()
}

type submittedTxn struct {
	s           *Session
	Transaction *protocol.Transaction
	Hash        [32]byte
	Status      *protocol.TransactionStatus
}

func (s *submittedTxn) submitted() *protocol.Transaction {
	return s.Transaction
}

func (s *submittedTxn) Ok() {
	if s.Status.Code.Success() {
		return
	}

	s.s.Abortf("Transaction %X failed:\n%+v\n", s.Hash, s.Status.Error)
}

func (s *submittedTxn) NotOk(message string) *submittedTxn {
	if s.Status.Failed() {
		return s
	}

	s.s.Abortf("Transaction %X succeeded, %s", s.Status.TxID.Hash(), message)
	panic("unreachable")
}

func (s *submittedTxn) Wait() completedFlow {
	s.Ok()

	status, txn, err := s.s.Engine.WaitFor(s.Hash, false)
	if err != nil {
		s.s.Abortf("Failed to get transaction %X: %v\n", s.Hash, err)
	}
	return s.asCompleted(status, txn)
}

func (s *submittedTxn) Delivered() completedFlow {
	s.Ok()

	status, txn, err := s.s.Engine.WaitFor(s.Hash, true)
	if err == nil {
		return s.asCompleted(status, txn)
	}

	c := s.Wait()
	for _, c := range c {
		if c.Status.Pending() {
			s.s.Abortf("Transaction %v is pending\n", c.Status.TxID)
		}
	}
	return c
}

func (s *submittedTxn) asCompleted(status []*protocol.TransactionStatus, txn []*protocol.Transaction) completedFlow {
	if len(status) != len(txn) {
		panic("wrong number of statuses")
	}
	c := make(completedFlow, len(status))
	for i := range c {
		c[i] = &completedTxn{
			s:           s.s,
			Transaction: txn[i],
			Status:      status[i],
		}
	}
	return c
}

type completedFlow []*completedTxn

func (c completedFlow) Ok() completedFlow {
	for _, c := range c {
		c.Ok()
	}

	return c
}

type completedTxn struct {
	s *Session
	*protocol.Transaction
	Status *protocol.TransactionStatus
}

func (c *completedTxn) Ok() *completedTxn {
	if c.Status.Code.Success() {
		return c
	}

	c.s.Abortf("Transaction %X failed:\n%+v\n", c.Status.TxID.Hash(), c.Status.Error)
	panic("unreachable")
}

func (c *completedTxn) NotOk(message string) *completedTxn {
	if c.Status.Failed() {
		return c
	}

	c.s.Abortf("Transaction %X succeeded, %s", c.Status.TxID.Hash(), message)
	panic("unreachable")
}
