// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type BatchTest struct {
	*testing.T
	*database.Batch
}

func NewBatchTest(t *testing.T, beginner database.Beginner) BatchTest {
	return BatchTest{t, beginner.Begin(true)}
}

func (t BatchTest) Run(name string, run func(t BatchTest)) {
	t.T.Run(name, func(s *testing.T) {
		t := BatchTest{s, t.Begin(true)}
		defer t.Discard()
		run(t)
	})
}

func (t *BatchTest) PutAccount(account protocol.Account) {
	require.NoError(t, t.Account(account.GetUrl()).Main().Put(account))
}

func (t *BatchTest) PutAccountCopy(account protocol.Account) protocol.Account {
	copy := account.CopyAsInterface().(protocol.Account)
	t.PutAccount(copy)
	return copy
}

func (t *BatchTest) AddSignature(txnHash []byte, keyEntryIndex uint64, sig protocol.Signature) {
	_, err := t.Transaction(txnHash).AddSignature(keyEntryIndex, sig)
	require.NoError(t, err)
}

func (t *BatchTest) GetTxnStatus(txnHash []byte) *protocol.TransactionStatus {
	status, err := t.Transaction(txnHash).Status().Get()
	require.NoError(t, err)
	return status
}

func (t *BatchTest) GetSignatures(txnHash []byte, signer *url.URL) *database.SignatureSet {
	sigs, err := t.Transaction(txnHash).ReadSignatures(signer)
	require.NoError(t, err)
	return sigs
}
