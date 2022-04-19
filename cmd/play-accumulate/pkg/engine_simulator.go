package pkg

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type SimEngine struct {
	*simulator.Simulator
}

type sessionTB struct {
	*Session
	lastLog string
}

func (s *sessionTB) Name() string {
	return s.Filename
}

func (s *sessionTB) Log(v ...interface{}) {
	str := fmt.Sprintln(v...)
	s.lastLog = str
	s.Output(Output{"stderr", []byte(str)})
}

func (s *sessionTB) Fail() {
	// What should we do here?
}

func (s *sessionTB) FailNow() {
	if s.lastLog == "" {
		s.Abort("Failed")
	} else {
		s.Abort(s.lastLog)
	}
}

func (s *sessionTB) Helper() {
	// Anything to do here?
}

func (s *Session) UseSimulator(bvnCount int) {
	sim := simulator.New(&sessionTB{Session: s}, bvnCount)
	sim.InitChain()
	s.Engine = &SimEngine{sim}
}

func (s SimEngine) GetAccount(url *URL) (protocol.Account, error) {
	subnet, err := s.Router().RouteAccount(url)
	if err != nil {
		return nil, err
	}

	batch := s.Subnet(subnet).Database.Begin(false)
	defer batch.Discard()
	return batch.Account(url).GetState()
}

func (s SimEngine) GetDirectory(account *URL) ([]*URL, error) {
	subnet, err := s.Router().RouteAccount(account)
	if err != nil {
		return nil, err
	}

	batch := s.Subnet(subnet).Database.Begin(false)
	defer batch.Discard()
	dir := indexing.Directory(batch, account)
	n, err := dir.Count()
	if err != nil {
		return nil, err
	}
	urls := make([]*URL, n)
	for i := range urls {
		urls[i], err = dir.Get(uint64(i))
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (s SimEngine) GetTransaction(hash [32]byte) (*protocol.Transaction, error) {
	for _, subnet := range s.Subnets {
		batch := s.Subnet(subnet.ID).Database.Begin(false)
		defer batch.Discard()
		txn, err := batch.Transaction(hash[:]).GetState()
		switch {
		case err == nil:
			return txn.Transaction, nil
		case !errors.Is(err, storage.ErrNotFound):
			return nil, err
		}
	}

	return nil, errors.NotFound("transaction %X not found", hash[:4])
}

func (s SimEngine) Submit(envelope *protocol.Envelope) (*protocol.TransactionStatus, error) {
	envelope = envelope.Copy()
	subnet, err := s.Router().Route(envelope)
	if err != nil {
		return nil, err
	}

	resp, err := s.Router().Submit(context.Background(), subnet, envelope, false, false)
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

func (s SimEngine) WaitFor(hash [32]byte) ([]*protocol.TransactionStatus, []*protocol.Transaction, error) {
	status, txn := s.WaitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
		return status.Delivered || status.Pending
	}, hash[:])
	return status, txn, nil
}
