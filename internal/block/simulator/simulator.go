package simulator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Simulator struct {
	testing.TB
	Logger    log.Logger
	Subnets   []config.Subnet
	Executors map[string]*ExecEntry
}

func newLogger(t testing.TB) log.Logger {
	if !acctesting.LogConsole {
		return logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)
	}

	w, err := logging.NewConsoleWriter("plain")
	require.NoError(t, err)
	level, writer, err := logging.ParseLogLevel(acctesting.DefaultLogLevels, w)
	require.NoError(t, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(t, err)
	return logger
}

func New(t testing.TB, bvnCount int) *Simulator {
	t.Helper()

	// Initialize the simulartor and network
	sim := new(Simulator)
	sim.TB = t
	sim.Logger = newLogger(t).With("module", "simulator")
	sim.Executors = map[string]*ExecEntry{}
	sim.Subnets = make([]config.Subnet, bvnCount+1)
	sim.Subnets[0] = config.Subnet{Type: config.Directory, ID: protocol.Directory}
	for i := 0; i < bvnCount; i++ {
		sim.Subnets[i+1] = config.Subnet{Type: config.BlockValidator, ID: fmt.Sprintf("BVN%d", i)}
	}

	// Initialize each executor
	for i := range sim.Subnets {
		subnet := &sim.Subnets[i]
		subnet.Nodes = []config.Node{{Type: config.Validator, Address: subnet.ID}}

		logger := newLogger(t).With("subnet", subnet.ID)
		key := acctesting.GenerateKey(t.Name(), subnet.ID)
		db := database.OpenInMemory(logger)

		network := config.Network{
			Type:          subnet.Type,
			LocalSubnetID: subnet.ID,
			LocalAddress:  subnet.ID,
			Subnets:       sim.Subnets,
		}

		exec, err := NewNodeExecutor(ExecutorOptions{
			Logger:  logger,
			Key:     key,
			Network: network,
			Router:  sim.Router(),
		}, db)
		require.NoError(t, err)

		sim.Executors[subnet.ID] = &ExecEntry{
			Database: db,
			Executor: exec,
		}
	}

	for _, subnet := range sim.Subnets {
		x := sim.Subnet(subnet.ID)
		require.NoError(t, x.Executor.Start())
	}

	return sim
}

func (s *Simulator) Router() routing.Router {
	return router{s}
}

func (s *Simulator) Subnet(id string) *ExecEntry {
	e, ok := s.Executors[id]
	require.Truef(s, ok, "Unknown subnet %q", id)
	return e
}

func (s *Simulator) Query(url *url.URL, req queryRequest, prove bool) interface{} {
	s.Helper()

	subnet, err := s.Router().RouteAccount(url)
	require.NoError(s, err)
	x := s.Subnet(subnet)
	return Query(s, x.Database, x.Executor, req, prove)
}

func (s *Simulator) InitChain() {
	s.Helper()

	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.ID)
		InitChain(s, x.Database, x.Executor)
	}
	s.WaitForGovernor()
}

func (s *Simulator) WaitForGovernor() {
	for _, subnet := range s.Subnets {
		s.Subnet(subnet.ID).Executor.PingGovernor_TESTONLY()
	}
}

func (s *Simulator) ExecuteBlock(envelopes ...*protocol.Envelope) {
	s.Helper()

	for _, envelope := range envelopes {
		subnet, err := s.Router().Route(envelope)
		require.NoError(s, err)
		x := s.Subnet(subnet)
		x.Submit(envelope)
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(s.Subnets))

	for _, subnet := range s.Subnets {
		go func(x *ExecEntry) {
			defer wg.Done()
			submitted := x.TakeSubmitted()
			ExecuteBlock(s, x.Database, x.Executor, nil, submitted...)
		}(s.Subnet(subnet.ID))
	}

	wg.Wait()
}

func (s *Simulator) findTxn(hash []byte) *ExecEntry {
	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.ID)

		batch := x.Database.Begin(false)
		defer batch.Discard()
		obj, err := batch.Transaction(hash).GetStatus()
		require.NoError(s, err)

		if !obj.Delivered {
			continue
		}

		return x
	}

	return nil
}

func (s *Simulator) WaitForTransaction(txnHash []byte) {
	var x *ExecEntry
	for i := 0; i < 50; i++ {
		x = s.findTxn(txnHash)
		if x != nil {
			break
		}

		s.ExecuteBlock()
		s.WaitForGovernor()
	}
	if x == nil {
		s.Fatalf("Transaction %X has not been delivered after 50 blocks", txnHash[:4])
		panic("unreachable")
	}

	batch := x.Database.Begin(false)
	synth, err := batch.Transaction(txnHash).GetSyntheticTxns()
	batch.Discard()
	require.NoError(s, err)

	for _, id := range synth.Hashes {
		s.WaitForTransaction(id[:])
	}
}

type ExecEntry struct {
	mu    sync.Mutex
	queue []*protocol.Envelope

	Database *database.Database
	Executor *block.Executor
}

func (s *ExecEntry) Submit(envelopes ...*protocol.Envelope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, envelopes...)
}

func (s *ExecEntry) TakeSubmitted() []*protocol.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	submitted := s.queue
	s.queue = nil
	return submitted
}
