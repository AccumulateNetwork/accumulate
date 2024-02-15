// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"io"
	"math/big"
	"runtime"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/services"
)

type Simulator struct {
	deterministic bool
	logger        log.Logger
	router        *Router
	services      *services.Network
	tasks         *taskQueue
	partIDs       []string
	partitions    map[string]*Partition
}

func New(opts ...Option) (*Simulator, error) {
	// Process options
	o := &simFactory{
		network: NewSimpleNetwork("Sim", 3, 3),
		storeOpt: func(_ *protocol.PartitionInfo, _ int, logger log.Logger) keyvalue.Beginner {
			return memory.New(nil)
		},
		snapshot: func(string, *accumulated.NetworkInit, log.Logger) (ioutil2.SectionReader, error) {
			return new(ioutil2.Buffer), nil
		},
		abci: noABCI,

		// Tests like to materialize tokens, which can cause problems with the
		// supply calculations. So start with a non-zero supply to provide a
		// buffer.
		initialSupply: big.NewInt(1e6 * protocol.AcmePrecision),
	}
	for _, opt := range opts {
		err := opt(o)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Build the simulator
	s := o.Build()

	// Initialize the network
	for _, id := range s.partIDs {
		snapshot, err := o.snapshot(id, o.network, s.logger)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("open snapshot: %w", err)
		}
		err = s.partitions[id].initChain(snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("init %s: %w", id, err)
		}
	}

	// Set the initial supply
	if o.initialSupply != nil {
		err := s.partitions[protocol.Directory].Update(func(b *database.Batch) error {
			var acme *protocol.TokenIssuer
			account := b.Account(protocol.AcmeUrl())
			err := account.Main().GetAs(&acme)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}

			acme.Issued.Add(&acme.Issued, o.initialSupply)

			err = account.Main().Put(acme)
			return errors.UnknownError.Wrap(err)
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return s, nil
}

func (s *Simulator) SetRoute(account *url.URL, partition string) {
	s.router.SetRoute(account, partition)
}

func (s *Simulator) Router() *Router { return s.router }

func (s *Simulator) EventBus(partition string) *events.Bus {
	return s.partitions[partition].nodes[0].eventBus
}

func (s *Simulator) BlockIndex(partition string) uint64 {
	res, err := s.partitions[partition].nodes[0].consensus.Status(&consensus.StatusRequest{})
	if err != nil {
		panic(err)
	}
	return res.BlockIndex
}

func (s *Simulator) BlockIndexFor(account *url.URL) uint64 {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	return s.BlockIndex(partition)
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	if s.deterministic {
		for _, id := range s.partIDs {
			err := s.partitions[id].execute()
			if err != nil {
				return err
			}
		}
	} else {
		for _, p := range s.partitions {
			p := p // Don't capture loop variables
			s.tasks.Go(p.execute)
		}
	}

	// Wait for execution to complete
	err := s.tasks.Flush()

	// Give any parallel processes a chance to run
	runtime.Gosched()

	return err
}

func (s *Simulator) SetSubmitHookFor(account *url.URL, fn SubmitHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetSubmitHook(fn)
}

func (s *Simulator) SetSubmitHook(partition string, fn SubmitHookFunc) {
	s.partitions[partition].SetSubmitHook(fn)
}

func (s *Simulator) SetBlockHookFor(account *url.URL, fn BlockHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetBlockHook(fn)
}

func (s *Simulator) SetBlockHook(partition string, fn BlockHookFunc) {
	s.partitions[partition].SetBlockHook(fn)
}

func (s *Simulator) SetNodeBlockHookFor(account *url.URL, fn NodeBlockHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetNodeBlockHook(fn)
}

func (s *Simulator) SetNodeBlockHook(partition string, fn NodeBlockHookFunc) {
	s.partitions[partition].SetNodeBlockHook(fn)
}

func (s *Simulator) Submit(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	partition, err := s.router.Route(envelope)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return s.SubmitTo(partition, envelope)
}

func (s *Simulator) SubmitTo(partition string, envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.BadRequest.WithFormat("%s is not a partition", partition)
	}

	return p.Submit(envelope, false)
}

func (s *Simulator) Partitions() []*protocol.PartitionInfo {
	var partitions []*protocol.PartitionInfo
	for _, p := range s.partitions {
		partitions = append(partitions, &p.PartitionInfo)
	}
	return partitions
}

func (s *Simulator) Partition(partition string) *Partition {
	return s.partitions[partition]
}

type errDb struct{ err error }

func (e errDb) View(func(*database.Batch) error) error   { return e.err }
func (e errDb) Update(func(*database.Batch) error) error { return e.err }
func (e errDb) Begin(bool) *database.Batch               { panic(e.err) }
func (e errDb) SetObserver(observer database.Observer)   { panic(e.err) }

func (s *Simulator) Database(partition string) database.Beginner {
	p, ok := s.partitions[partition]
	if !ok {
		return errDb{errors.BadRequest.WithFormat("%s is not a partition", partition)}
	}
	return p
}

func (s *Simulator) DatabaseFor(account *url.URL) database.Updater {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		return errDb{errors.UnknownError.Wrap(err)}
	}
	return s.Database(partition)
}

func (s *Simulator) ViewAll(fn func(batch *database.Batch) error) error {
	for _, p := range s.partitions {
		err := p.View(fn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Simulator) Collect(partition string, file io.WriteSeeker, opts *database.CollectOptions) error {
	return s.partitions[partition].nodes[0].database.Collect(file, protocol.PartitionUrl(partition), opts)
}
