// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// FakeNode is a compatibility shim for tests based on abci_test.FakeNode.
type FakeNode struct {
	H       *harness.Sim
	onErr   func(err error)
	assert  *assert.Assertions
	require *require.Assertions
}

func NewFakeNode(t testing.TB, errorHandler func(err error)) *FakeNode {
	sim := harness.NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	if errorHandler == nil {
		errorHandler = func(err error) {
			t.Helper()

			var err2 *errors.Error
			if errors.As(err, &err2) && err2.Code == errors.Delivered {
				return
			}
			assert.NoError(t, err)
		}
	}
	return &FakeNode{H: sim, onErr: errorHandler}
}

func (n *FakeNode) T() testing.TB { return n.H.TB }

func (n *FakeNode) Assert() *assert.Assertions {
	if n.assert == nil {
		n.assert = assert.New(n.T())
	}
	return n.assert
}

func (n *FakeNode) Require() *require.Assertions {
	if n.require == nil {
		n.require = require.New(n.T())
	}
	return n.require
}

func (n *FakeNode) View(fn func(batch *database.Batch)) {
	helpers.View(n.T(), n.H.Database("BVN0"), fn)
}

func (n *FakeNode) Update(fn func(batch *database.Batch)) {
	helpers.Update(n.T(), n.H.Database("BVN0"), fn)
}

func (n *FakeNode) QueryTx(txid []byte, wait time.Duration, ignorePending bool) *api.TransactionRecord {
	n.T().Helper()
	return n.H.QueryTransaction(protocol.PartitionUrl("BVN0").WithTxID(*(*[32]byte)(txid)), nil)
}

func (n *FakeNode) GetOraclePrice() uint64 {
	n.T().Helper()
	return n.H.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory}).Oracle.Price
}

func (n *FakeNode) parseUrl(s string) *url.URL {
	u := url.MustParse(s)
	if _, err := protocol.ParseLiteAddress(u); err == nil {
		return u
	}
	if _, err := protocol.ParseLiteDataAddress(u); err == nil {
		return u
	}

	return protocol.AccountUrl(u.Authority, u.Path)
}

func (n *FakeNode) QueryAccountAs(s string, result any) {
	n.T().Helper()
	n.H.QueryAccountAs(n.parseUrl(s), nil, result)
}

func (n *FakeNode) GetDirectory(s string) []string {
	n.T().Helper()
	dir := n.H.QueryDirectoryUrls(n.parseUrl(s), nil).Records
	ss := make([]string, len(dir))
	for i, u := range dir {
		ss[i] = u.Value.String()
	}
	return ss
}

func (n *FakeNode) GetDataEntry(s string, q *api.DataQuery) protocol.DataEntry {
	r := n.H.QueryDataEntry(n.parseUrl(s), q)
	switch body := r.Value.Transaction.Body.(type) {
	case *protocol.WriteData:
		return body.Entry
	case *protocol.WriteDataTo:
		return body.Entry
	case *protocol.SyntheticWriteData:
		return body.Entry
	case *protocol.SystemWriteData:
		return body.Entry
	default:
		n.T().Fatalf("Wanted data transaction, got %T", body)
		panic("not reached")
	}
}

func (n *FakeNode) GetDataAccount(s string) *protocol.DataAccount {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.DataAccount](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetTokenAccount(s string) *protocol.TokenAccount {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.TokenAccount](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetLiteIdentity(s string) *protocol.LiteIdentity {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.LiteIdentity](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetLiteTokenAccount(s string) *protocol.LiteTokenAccount {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.LiteTokenAccount](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetLiteDataAccount(s string) *protocol.LiteDataAccount {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.LiteDataAccount](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetADI(s string) *protocol.ADI {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.ADI](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetKeyBook(s string) *protocol.KeyBook {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.KeyBook](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetKeyPage(s string) *protocol.KeyPage {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.KeyPage](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) GetTokenIssuer(s string) *protocol.TokenIssuer {
	n.T().Helper()
	return harness.QueryAccountAs[*protocol.TokenIssuer](&n.H.Harness, n.parseUrl(s))
}

func (n *FakeNode) Execute(inBlock func(func(*protocol.Envelope))) (sigHashes, txnHashes [][32]byte, err error) {
	n.T().Helper()

	var deliveries []*chain.Delivery
	inBlock(func(env *protocol.Envelope) {
		delivery, err := chain.NormalizeEnvelope(env)
		n.Require().NoError(err)
		deliveries = append(deliveries, delivery...)
	})

	for _, delivery := range deliveries {
		for _, sig := range delivery.Signatures {
			sigHashes = append(sigHashes, *(*[32]byte)(sig.Hash()))
		}
	}
	for _, delivery := range deliveries {
		remote, ok := delivery.Transaction.Body.(*protocol.RemoteTransaction)
		switch {
		case !ok:
			txnHashes = append(txnHashes, *(*[32]byte)(delivery.Transaction.GetHash()))
		case remote.Hash != [32]byte{}:
			txnHashes = append(txnHashes, remote.Hash)
		}
	}

	var cond []harness.Condition
	for _, delivery := range deliveries {
		status := n.H.Submit(&protocol.Envelope{
			Transaction: []*protocol.Transaction{delivery.Transaction},
			Signatures:  delivery.Signatures,
		})
		if status.Error == nil {
			cond = append(cond, n.conditionFor(status.TxID))
		} else {
			return sigHashes, txnHashes, status.Error
		}
	}

	// Step until the transactions are done
	n.H.StepUntil(cond...)
	return sigHashes, txnHashes, nil
}

func (n *FakeNode) MustExecute(inBlock func(func(*protocol.Envelope))) (sigHashes, txnHashes [][32]byte) {
	n.T().Helper()
	sigHashes, txnHashes, err := n.Execute(inBlock)
	n.Require().NoError(err)
	return sigHashes, txnHashes
}

func (n *FakeNode) MustExecuteAndWait(inBlock func(func(*protocol.Envelope))) [][32]byte {
	// Execute already waits
	_, txnHashes := n.MustExecute(inBlock)
	return txnHashes
}

// func (n *FakeNode) MustWaitForTxns(ids ...[]byte)
// func (n *FakeNode) WaitForTxns(ids ...[]byte) error

func (n *FakeNode) conditionFor(txid *url.TxID) harness.Condition {
	var produced []harness.Condition
	return func(h *harness.Harness) bool {
		if produced == nil {
			h.TB.Helper()
			r, err := h.Query().QueryTransaction(context.Background(), txid, nil)
			switch {
			case errors.Is(err, errors.NotFound):
				// Wait
				return false

			case err != nil:
				n.Require().NoError(err)
			}

			// Wait if it hasn't been delivered
			if !r.Status.Delivered() {
				return false
			}

			// If there are no produced transactions, we're done
			if r.Produced == nil || r.Produced.Total == 0 {
				return true
			}

			// Add conditions for produced transactions
			for _, txid := range r.Produced.Records {
				produced = append(produced, n.conditionFor(txid.Value))
			}
		}

		// If all produced transactions are done, we're done
		for _, produced := range produced {
			if !produced(h) {
				return false
			}
		}
		return true
	}
}
