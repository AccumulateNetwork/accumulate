// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

type SimEngine struct {
	session *Session
	*simulator.Simulator
}

func (s *Session) UseSimulator(bvnCount int) {
	w, err := logging.NewConsoleWriter("plain")
	if err != nil {
		s.Abort(err)
	}
	level, writer, err := logging.ParseLogLevel(config.DefaultLogLevels, w)
	if err != nil {
		s.Abort(err)
	}
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	if err != nil {
		s.Abort(err)
	}

	sim, err := simulator.New(
		logger,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork("Play", bvnCount, 1),
		simulator.Genesis(time.Now()),
	)
	if err != nil {
		s.Abort(err)
	}
	s.Engine = &SimEngine{s, sim}
}

func (s *SimEngine) GetAccount(url *URL) (protocol.Account, error) {
	var account protocol.Account
	err := s.DatabaseFor(url).View(func(batch *database.Batch) error {
		var err error
		account, err = batch.Account(url).Main().Get()
		return err
	})
	return account, err
}

func (s *SimEngine) GetDirectory(account *URL) ([]*URL, error) {
	var urls []*URL
	err := s.DatabaseFor(account).View(func(batch *database.Batch) error {
		dir := indexing.Directory(batch, account)
		n, err := dir.Count()
		if err != nil {
			return err
		}
		urls := make([]*URL, n)
		for i := range urls {
			urls[i], err = dir.Get(uint64(i))
			if err != nil {
				return err
			}
		}
		return nil
	})
	return urls, err
}

func (s *SimEngine) GetTransaction(hash [32]byte) (*protocol.Transaction, error) {
	for _, partition := range s.Partitions() {
		var state *database.SigOrTxn
		err := s.Database(partition.ID).View(func(batch *database.Batch) error {
			var err error
			state, err = batch.Transaction(hash[:]).GetState()
			return err
		})
		switch {
		case err == nil:
			return state.Transaction, nil
		case !errors.Is(err, storage.ErrNotFound):
			return nil, err
		}
	}

	return nil, errors.NotFound.WithFormat("transaction %X not found", hash[:4])
}

func (s *SimEngine) Submit(envelope *messaging.Envelope) (*protocol.TransactionStatus, error) {
	envelope = envelope.Copy()
	partition, err := s.Router().Route(envelope)
	if err != nil {
		return nil, err
	}

	resp, err := s.Router().Submit(context.Background(), partition, envelope, false, false)
	if err != nil {
		return nil, err
	}

	rset := new(protocol.TransactionResultSet)
	err = rset.UnmarshalBinary(resp.Data)
	if err != nil {
		return nil, err
	}

	return rset.Results[0], nil
}

func (s *SimEngine) WaitFor(hash [32]byte, delivered bool) ([]*protocol.TransactionStatus, []*protocol.Transaction, error) {
	status, txn := s.waitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
		if delivered {
			return status.Delivered()
		}
		return status.Pending() || status.Delivered()
	}, hash[:])
	return status, txn, nil
}

func (s *SimEngine) findTxn(status func(*protocol.TransactionStatus) bool, hash []byte) *url.TxID {
	for _, partition := range s.Partitions() {
		var txid *url.TxID
		err := s.Database(partition.ID).View(func(batch *database.Batch) error {
			obj, err := batch.Transaction(hash).GetStatus()
			if err != nil {
				s.session.Abort(err)
			}
			if !status(obj) {
				return nil
			}
			state, err := batch.Transaction(hash).Main().Get()
			if err != nil {
				s.session.Abort(err)
			}
			txid = state.Transaction.ID()
			return nil
		})
		if err != nil {
			s.session.Abort(err)
		}
		if txid != nil {
			return txid
		}
	}

	return nil
}

func (s *SimEngine) waitForTransaction(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte, n int) (*protocol.Transaction, *protocol.TransactionStatus, []*url.TxID) {
	var x *url.TxID
	for i := 0; i < n; i++ {
		x = s.findTxn(statusCheck, txnHash)
		if x != nil {
			break
		}

		err := s.Step()
		if err != nil {
			s.session.Abort(err)
		}
	}
	if x == nil {
		return nil, nil, nil
	}

	var synth []*url.TxID
	var state *database.SigOrTxn
	var status *protocol.TransactionStatus
	err := s.DatabaseFor(x.AsUrl()).View(func(batch *database.Batch) error {
		var err error
		synth, err = batch.Transaction(txnHash).Produced().Get()
		if err != nil {
			s.session.Abort(err)
		}
		state, err = batch.Transaction(txnHash).Main().Get()
		if err != nil {
			s.session.Abort(err)
		}
		status, err = batch.Transaction(txnHash).Status().Get()
		if err != nil {
			s.session.Abort(err)
		}
		return nil
	})
	if err != nil {
		s.session.Abort(err)
	}
	return state.Transaction, status, synth
}

func (s *SimEngine) waitForTransactionFlow(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	txn, status, synth := s.waitForTransaction(statusCheck, txnHash, 50)
	if txn == nil {
		s.session.Abortf("Transaction %X has not been delivered after 50 blocks", txnHash[:4])
		panic("unreachable")
	}

	status.TxID = txn.ID()
	statuses := []*protocol.TransactionStatus{status}
	transactions := []*protocol.Transaction{txn}
	for _, id := range synth {
		// Wait for synthetic transactions to be delivered
		id := id.Hash()
		st, txn := s.waitForTransactionFlow((*protocol.TransactionStatus).Delivered, id[:]) //nolint:rangevarref
		statuses = append(statuses, st...)
		transactions = append(transactions, txn...)
	}

	return statuses, transactions
}
