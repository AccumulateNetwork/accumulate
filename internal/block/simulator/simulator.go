package simulator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
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

	routingOverrides map[[32]byte]string
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
	sim.routingOverrides = map[[32]byte]string{}
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

func (s *Simulator) SetRouteFor(account *url.URL, subnet string) {
	// Make sure the account is a root identity
	if !account.RootIdentity().Equal(account) {
		s.Fatalf("Cannot set the route for a non-root: %v", account)
	}

	// Make sure the subnet exists
	s.Subnet(subnet)

	// Add/remove the override
	if subnet == "" {
		delete(s.routingOverrides, account.AccountID32())
	} else {
		s.routingOverrides[account.AccountID32()] = subnet
	}
}

func (s *Simulator) Router() routing.Router {
	return router{s}
}

func (s *Simulator) Subnet(id string) *ExecEntry {
	e, ok := s.Executors[id]
	require.Truef(s, ok, "Unknown subnet %q", id)
	return e
}

func (s *Simulator) SubnetFor(url *url.URL) *ExecEntry {
	s.Helper()

	subnet, err := s.Router().RouteAccount(url)
	require.NoError(s, err)
	return s.Subnet(subnet)
}

func (s *Simulator) QueryUrl(url *url.URL, req queryRequest, prove bool) interface{} {
	x := s.SubnetFor(url)
	return Query(s, x.Database, x.Executor, req, prove)
}

func (s *Simulator) Query(url *url.URL, req queryRequest, prove bool) interface{} {
	s.Helper()

	x := s.SubnetFor(url)
	return Query(s, x.Database, x.Executor, req, prove)
}

func (s *Simulator) InitChain() {
	s.Helper()

	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.ID)
		InitChain(s, x.Database, x.Executor)
	}

	// Wait for the governor
	for _, subnet := range s.Subnets {
		s.Subnet(subnet.ID).Executor.PingGovernor_TESTONLY()
	}
}

// ExecuteBlock executes a block after submitting envelopes. If a status channel
// is provided, statuses will be sent to the channel as transactions are
// executed. Once the block is complete, the status channel will be closed (if
// provided).
func (s *Simulator) ExecuteBlock(statusChan chan<- *protocol.TransactionStatus) {
	s.Helper()

	wg := new(sync.WaitGroup)
	wg.Add(len(s.Subnets))

	for _, subnet := range s.Subnets {
		go func(x *ExecEntry) {
			defer wg.Done()

			submitted := x.TakeSubmitted()
			status := ExecuteBlock(s, x.Database, x.Executor, nil, submitted...)
			if statusChan == nil {
				return
			}

			for i, status := range status {
				status.For = *(*[32]byte)(submitted[i].GetTxHash())
				statusChan <- status
			}
		}(s.Subnet(subnet.ID))
	}

	// Wait for all subnets to complete
	wg.Wait()

	// Wait for the governor
	for _, subnet := range s.Subnets {
		s.Subnet(subnet.ID).Executor.PingGovernor_TESTONLY()
	}

	if statusChan != nil {
		close(statusChan)
	}
}

// ExecuteBlocks executes a number of blocks. This is useful for things like
// waiting for a block to be anchored.
func (s *Simulator) ExecuteBlocks(n int) {
	for ; n > 0; n-- {
		s.ExecuteBlock(nil)
	}
}

func (s *Simulator) Submit(envelopes ...*protocol.Envelope) ([]*protocol.Envelope, error) {
	s.Helper()

	for _, envelope := range envelopes {
		// Route
		subnet, err := s.Router().Route(envelope)
		require.NoError(s, err)
		x := s.Subnet(subnet)

		// Enqueue
		x.Submit(envelope)
	}

	return envelopes, nil
}

// MustSubmitAndExecuteBlock executes a block with the envelopes and fails the test if
// any envelope fails.
func (s *Simulator) MustSubmitAndExecuteBlock(envelopes ...*protocol.Envelope) []*protocol.Envelope {
	s.Helper()

	ch := make(chan *protocol.TransactionStatus)
	ids := make(map[[32]byte]bool, len(envelopes))
	for _, env := range envelopes {
		ids[*(*[32]byte)(env.GetTxHash())] = true
	}

	_, err := s.Submit(envelopes...)
	require.NoError(s, err)

	go s.ExecuteBlock(ch)

	var didFail bool
	for status := range ch {
		if status.Code == 0 || !ids[status.For] {
			continue
		}

		assert.Zero(s, status.Code, status.Message)
		didFail = true
	}
	if didFail {
		s.FailNow()
	}

	return envelopes
}

func (s *Simulator) findTxn(hash []byte) *ExecEntry {
	s.Helper()

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

func (s *Simulator) WaitForTransactions(envelopes ...*protocol.Envelope) {
	s.Helper()

	for _, envelope := range envelopes {
		s.WaitForTransaction(envelope.GetTxHash())
	}
}

func (s *Simulator) WaitForTransaction(txnHash []byte) {
	s.Helper()

	var x *ExecEntry
	for i := 0; i < 50; i++ {
		x = s.findTxn(txnHash)
		if x != nil {
			break
		}

		s.ExecuteBlock(nil)
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
	mu                      sync.Mutex
	nextBlock, currentBlock []*protocol.Envelope

	Database *database.Database
	Executor *block.Executor
}

// Submit adds the envelopes to the next block's queue.
//
// By adding transactions to the next block and swaping queues when a block is
// executed, we roughly simulate the process Tendermint uses to build blocks.
func (s *ExecEntry) Submit(envelopes ...*protocol.Envelope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextBlock = append(s.nextBlock, envelopes...)
}

// TakeSubmitted returns the envelopes for the current block.
func (s *ExecEntry) TakeSubmitted() []*protocol.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	submitted := s.currentBlock
	s.currentBlock = s.nextBlock
	s.nextBlock = nil
	return submitted
}
