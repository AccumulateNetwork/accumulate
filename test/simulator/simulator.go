package simulator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

type Simulator struct {
	logger     logging.OptionalLogger
	init       *accumulated.NetworkInit
	database   func(partition string, node int, logger log.Logger) database.Beginner
	partitions map[string]*Partition
	router     *Router
}

func New(logger log.Logger, init *accumulated.NetworkInit, database func(partition string, node int, logger log.Logger) database.Beginner) (*Simulator, error) {
	s := new(Simulator)
	s.logger.Set(logger)
	s.init = init
	s.database = database
	s.partitions = make(map[string]*Partition, len(init.Bvns)+1)
	s.router = newRouter(logger, s.partitions)

	var err error
	s.partitions[protocol.Directory], err = newDn(s, init)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	for _, init := range init.Bvns {
		s.partitions[init.Id], err = newBvn(s, init)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	events.SubscribeSync(s.partitions[protocol.Directory].nodes[0].eventBus, s.router.willChangeGlobals)
	return s, nil
}

func MemoryDatabase(_ string, _ int, logger log.Logger) database.Beginner {
	return database.OpenInMemory(logger)
}

func SimpleNetwork(name string, bvnCount, nodeCount int) *accumulated.NetworkInit {
	net := new(accumulated.NetworkInit)
	net.Id = name
	for i := 0; i < bvnCount; i++ {
		bvnInit := new(accumulated.BvnInit)
		bvnInit.Id = fmt.Sprintf("BVN%d", i)
		for j := 0; j < nodeCount; j++ {
			bvnInit.Nodes = append(bvnInit.Nodes, &accumulated.NodeInit{
				DnnType:    config.Validator,
				BvnnType:   config.Validator,
				PrivValKey: testing.GenerateKey(name, bvnInit.Id, j),
			})
		}
		net.Bvns = append(net.Bvns, bvnInit)
	}
	return net
}

func (s *Simulator) SetRoute(account *url.URL, partition string) {
	s.router.SetRoute(account, partition)
}

func (s *Simulator) InitFromGenesis() error {
	return s.InitFromGenesisWith(nil)
}

func (s *Simulator) InitFromGenesisWith(values *core.GlobalValues) error {
	if values == nil {
		values = new(core.GlobalValues)
	}

	genDocs, err := accumulated.BuildGenesisDocs(s.init, values, GenesisTime, s.logger, nil)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "build genesis docs: %w", err)
	}

	for _, p := range s.partitions {
		var snapshot []byte
		err = json.Unmarshal(genDocs[p.ID].AppState, &snapshot)
		if err != nil {
			panic(err)
		}

		err = p.initChain(ioutil2.NewBuffer(snapshot))
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}
	return nil
}

func (s *Simulator) InitFromSnapshot(snapshot func(string) (ioutil2.SectionReader, error)) error {
	for _, p := range s.partitions {
		snapshot, err := snapshot(p.ID)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "open snapshot: %w", err)
		}
		err = p.initChain(snapshot)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "init %s: %w", p.ID, err)
		}
	}
	return nil
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	errg := new(errgroup.Group)
	for _, p := range s.partitions {
		p := p // Don't capture loop variables
		errg.Go(func() error { return p.execute(errg) })
	}
	return errg.Wait()
}

func (s *Simulator) Submit(delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	partition, err := s.router.Route(&protocol.Envelope{
		Transaction: []*protocol.Transaction{delivery.Transaction},
		Signatures:  delivery.Signatures,
	})
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.Format(errors.StatusBadRequest, "%s is not a partition", partition)
	}

	return p.Submit(delivery, false)
}

func (s *Simulator) partitionFor(account *url.URL) (*Partition, error) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.Format(errors.StatusBadRequest, "%s is not a partition", partition)
	}

	return p, nil
}

func (s *Simulator) View(account *url.URL, fn func(batch *database.Batch) error) error {
	p, err := s.partitionFor(account)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return p.View(fn)
}

func (s *Simulator) Update(account *url.URL, fn func(batch *database.Batch) error) error {
	p, err := s.partitionFor(account)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return p.Update(fn)
}
