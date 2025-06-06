// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"os"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

type Simulator struct {
	S *simulator.Simulator
	H *harness.Sim

	// setup
	TB   testing.TB
	opts SimulatorOptions
}

type SimulatorOptions struct {
	BvnCount  int
	LogLevels string
	OpenDB    simulator.OpenDatabaseFunc
	Snapshots []func() (ioutil2.SectionReader, error)
}

func New(t testing.TB, bvnCount int) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.Setup(SimulatorOptions{BvnCount: bvnCount})
	return sim
}

func NewWith(t testing.TB, opts SimulatorOptions) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.Setup(opts)
	return sim
}

func (sim *Simulator) Setup(opts SimulatorOptions) {
	sim.TB.Helper()

	if opts.BvnCount == 0 {
		opts.BvnCount = 3
	}
	if opts.LogLevels == "" {
		opts.LogLevels = acctesting.DefaultLogLevels
	}
	sim.opts = opts
}

func (s *Simulator) Init(opts ...simulator.Option) {
	if s.opts.OpenDB != nil {
		opts = append(opts, simulator.WithDatabase(s.opts.OpenDB))
	}
	opts = append(opts,
		simulator.SimpleNetwork(s.TB.Name(), s.opts.BvnCount, 1),
		simulator.WithRecordings(harness.Recordings(s.TB)),
		simulator.WithLogger(acctesting.NewTestLogger(s.TB)),
	)

	var err error
	s.S, err = simulator.New(opts...)
	require.NoError(s.TB, err)
	s.H = harness.NewSimWith(s.TB, s.S)
}

func (s *Simulator) InitFromGenesis() {
	s.Init(simulator.Genesis(GenesisTime))
}

func (s *Simulator) InitFromGenesisWith(values *core.GlobalValues) {
	s.Init(simulator.GenesisWith(GenesisTime, values))
}

func (s *Simulator) InitFromSnapshot(filename func(string) string) {
	s.Init(simulator.WithSnapshot(func(partition string, _ *accumulated.NetworkInit, _ log.Logger) (ioutil2.SectionReader, error) {
		return os.Open(filename(partition))
	}))
}

func (s *Simulator) Router() routing.Router {
	return s.S.Router()
}

func (s *Simulator) SetRouteFor(u *url.URL, p string) {
	s.S.SetRoute(u, p)
}

func (s *Simulator) ExecuteBlock(interface{ x() }) {
	s.H.Step()
}

func (s *Simulator) ExecuteBlocks(n int) {
	s.H.StepN(n)
}

func (s *Simulator) Submit(envelopes ...*messaging.Envelope) ([]*messaging.Envelope, error) {
	for _, env := range envelopes {
		st, err := s.S.Submit(env)
		require.NoError(s.TB, err)
		for _, st := range st {
			if st.Error != nil {
				return nil, st.Error
			}
		}
	}
	return envelopes, nil
}

func (s *Simulator) MustSubmitAndExecuteBlock(envelopes ...*messaging.Envelope) []*messaging.Envelope {
	_, err := s.Submit(envelopes...)
	require.NoError(s.TB, err)
	s.H.Step()
	s.H.Step()
	return envelopes
}

func (s *Simulator) SubmitAndExecuteBlock(envelopes ...*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	s.TB.Helper()

	_, err := s.Submit(envelopes...)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	ids := map[[32]byte]bool{}
	for _, env := range envelopes {
		deliveries, err := chain.NormalizeEnvelope(env)
		require.NoError(s.TB, err)
		for _, d := range deliveries {
			ids[*(*[32]byte)(d.Transaction.GetHash())] = true
		}
	}

	s.ExecuteBlock(nil)
	s.ExecuteBlock(nil)

	status := make([]*protocol.TransactionStatus, 0, len(envelopes))
	for _, env := range envelopes {
		deliveries, err := chain.NormalizeEnvelope(env)
		require.NoError(s.TB, err)
		for _, d := range deliveries {
			helpers.View(s.TB, s.S.DatabaseFor(d.Transaction.Header.Principal), func(batch *database.Batch) {
				st, err := batch.Transaction(d.Transaction.GetHash()).Status().Get()
				require.NoError(s.TB, err)
				st.TxID = d.Transaction.ID()
				status = append(status, st)
			})
		}
	}

	return status, nil
}

func (s *Simulator) findTxn(status func(*protocol.TransactionStatus) bool, hash []byte) *url.TxID {
	s.TB.Helper()

	for _, partition := range s.S.Partitions() {
		var txid *url.TxID
		err := s.S.Database(partition.ID).View(func(batch *database.Batch) error {
			obj, err := batch.Transaction(hash).Status().Get()
			require.NoError(s.TB, err)
			if !status(obj) {
				return nil
			}
			var msg messaging.MessageWithTransaction
			err = batch.Message2(hash).Main().GetAs(&msg)
			require.NoError(s.TB, err)
			txid = msg.GetTransaction().ID()
			return nil
		})
		require.NoError(s.TB, err)
		if txid != nil {
			return txid
		}
	}

	return nil
}

func (s *Simulator) WaitForTransaction(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte, n int) (*protocol.Transaction, *protocol.TransactionStatus, []*url.TxID) {
	s.TB.Helper()

	var x *url.TxID
	for i := 0; i < n; i++ {
		x = s.findTxn(statusCheck, txnHash)
		if x != nil {
			break
		}

		s.H.Step()
	}
	if x == nil {
		return nil, nil, nil
	}

	var synth []*url.TxID
	var state messaging.MessageWithTransaction
	var status *protocol.TransactionStatus
	err := s.S.DatabaseFor(x.AsUrl()).View(func(batch *database.Batch) error {
		var err error
		synth, err = batch.Transaction(txnHash).Produced().Get()
		require.NoError(s.TB, err)
		err = batch.Message2(txnHash).Main().GetAs(&state)
		require.NoError(s.TB, err)
		status, err = batch.Transaction(txnHash).Status().Get()
		require.NoError(s.TB, err)
		return nil
	})
	require.NoError(s.TB, err)
	return state.GetTransaction(), status, synth
}

func (s *Simulator) WaitForTransactionFlow(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	s.TB.Helper()

	txn, status, synth := s.WaitForTransaction(statusCheck, txnHash, 50)
	if txn == nil {
		s.TB.Fatalf("Transaction %X has not been delivered after 50 blocks", txnHash[:4])
		panic("unreachable")
	}

	status.TxID = txn.ID()
	statuses := []*protocol.TransactionStatus{status}
	transactions := []*protocol.Transaction{txn}
	for _, id := range synth {
		// Wait for synthetic transactions to be delivered
		id := id.Hash()
		st, txn := s.WaitForTransactionFlow((*protocol.TransactionStatus).Delivered, id[:]) //nolint:rangevarref
		statuses = append(statuses, st...)
		transactions = append(transactions, txn...)
	}

	return statuses, transactions
}

func (s *Simulator) WaitForTransactions(status func(*protocol.TransactionStatus) bool, envelopes ...*messaging.Envelope) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	s.TB.Helper()

	var statuses []*protocol.TransactionStatus
	var transactions []*protocol.Transaction
	for _, envelope := range envelopes {
		deliveries, err := chain.NormalizeEnvelope(envelope)
		require.NoError(s.TB, err)
		for _, delivery := range deliveries {
			st, txn := s.WaitForTransactionFlow(status, delivery.Transaction.GetHash())
			statuses = append(statuses, st...)
			transactions = append(transactions, txn...)
		}
	}
	return statuses, transactions
}
