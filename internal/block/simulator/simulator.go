package simulator

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type Simulator struct {
	tb
	Logger    log.Logger
	Subnets   []config.Subnet
	Executors map[string]*ExecEntry

	routingOverrides map[[32]byte]string
}

func (s *Simulator) newLogger() log.Logger {
	if !acctesting.LogConsole {
		return logging.NewTestLogger(s, "plain", acctesting.DefaultLogLevels, false)
	}

	w, err := logging.NewConsoleWriter("plain")
	require.NoError(s, err)
	level, writer, err := logging.ParseLogLevel(acctesting.DefaultLogLevels, w)
	require.NoError(s, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(s, err)
	return logger
}

func New(t TB, bvnCount int) *Simulator {
	t.Helper()

	// Initialize the simulartor and network
	sim := new(Simulator)
	sim.routingOverrides = map[[32]byte]string{}
	sim.TB = t
	sim.Logger = sim.newLogger().With("module", "simulator")
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

		logger := sim.newLogger().With("subnet", subnet.ID)
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
		require.NoError(sim, err)

		sim.Executors[subnet.ID] = &ExecEntry{
			Database: db,
			Executor: exec,
		}
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
}

// ExecuteBlock executes a block after submitting envelopes. If a status channel
// is provided, statuses will be sent to the channel as transactions are
// executed. Once the block is complete, the status channel will be closed (if
// provided).
func (s *Simulator) ExecuteBlock(statusChan chan<- *protocol.TransactionStatus) {
	s.Helper()

	errg := new(errgroup.Group)
	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.ID)
		submitted := x.TakeSubmitted()
		errg.Go(func() error {
			status, err := ExecuteBlock(s, x.Database, x.Executor, nil, submitted...)
			if err != nil {
				return err
			}
			if statusChan == nil {
				return nil
			}

			for _, status := range status {
				statusChan <- status
			}
			return nil
		})
	}

	// Wait for all subnets to complete
	err := errg.Wait()
	require.NoError(tb{s}, err)

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

		// Normalize - use a copy to avoid weird issues caused by modifying values
		deliveries, err := chain.NormalizeEnvelope(envelope.Copy())
		if err != nil {
			return nil, err
		}

		// Check
		for _, delivery := range deliveries {
			_, err = CheckTx(s, x.Database, x.Executor, delivery)
			if err != nil {
				return nil, err
			}
		}

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
	ids := map[[32]byte]bool{}
	for _, env := range envelopes {
		for _, d := range NormalizeEnvelope(s, env) {
			ids[*(*[32]byte)(d.Transaction.GetHash())] = true
		}
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

func (s *Simulator) findTxn(status func(*protocol.TransactionStatus) bool, hash []byte) *ExecEntry {
	s.Helper()

	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.ID)

		batch := x.Database.Begin(false)
		defer batch.Discard()
		accurl, err := batch.Transaction(hash).GetOriginUrl()
		require.NoError(s, err)
		acc := batch.Account(accurl)
		obj, err := acc.GetStatus(hash)
		require.NoError(s, err)
		if status(obj) {
			return x
		}
	}

	return nil
}

func (s *Simulator) WaitForTransactions(status func(*protocol.TransactionStatus) bool, envelopes ...*protocol.Envelope) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	s.Helper()

	var statuses []*protocol.TransactionStatus
	var transactions []*protocol.Transaction
	for _, envelope := range envelopes {
		for _, delivery := range NormalizeEnvelope(s, envelope) {
			st, txn := s.WaitForTransactionFlow(status, delivery.Transaction.GetHash())
			statuses = append(statuses, st...)
			transactions = append(transactions, txn...)
		}
	}
	return statuses, transactions
}

func (s *Simulator) WaitForTransactionFlow(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	s.Helper()

	var x *ExecEntry
	for i := 0; i < 50; i++ {
		x = s.findTxn(statusCheck, txnHash)
		if x != nil {
			break
		}

		s.ExecuteBlock(nil)
	}
	if x == nil {
		s.Errorf("Transaction %X has not been delivered after 50 blocks", txnHash[:4])
		s.FailNow()
		panic("unreachable")
	}

	batch := x.Database.Begin(false)
	synth, err1 := batch.Transaction(txnHash).GetSyntheticTxns()
	state, err2 := batch.Transaction(txnHash).GetState()
	accurl, _ := batch.Transaction(txnHash).GetOriginUrl()
	acc := batch.Account(accurl)
	status, err3 := acc.GetStatus(txnHash)
	batch.Discard()
	require.NoError(s, err1)
	require.NoError(s, err2)
	require.NoError(s, err3)
	status.For = *(*[32]byte)(txnHash)
	statuses := []*protocol.TransactionStatus{status}
	transactions := []*protocol.Transaction{state.Transaction}
	for _, id := range synth.Hashes {
		// Wait for synthetic transactions to be delivered
		st, txn := s.WaitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
			return status.Delivered
		}, id[:])
		statuses = append(statuses, st...)
		transactions = append(transactions, txn...)
	}

	return statuses, transactions
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
