package simulator

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
	"golang.org/x/sync/errgroup"
)

var genesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

type Simulator struct {
	tb
	Logger    log.Logger
	Subnets   []config.Subnet
	Executors map[string]*ExecEntry

	LogLevels string

	router           routing.Router
	routingOverrides map[[32]byte]string
}

func (s *Simulator) newLogger() log.Logger {
	levels := s.LogLevels
	if levels == "" {
		levels = acctesting.DefaultLogLevels
	}

	if !acctesting.LogConsole {
		return logging.NewTestLogger(s, "plain", levels, false)
	}

	w, err := logging.NewConsoleWriter("plain")
	require.NoError(s, err)
	level, writer, err := logging.ParseLogLevel(levels, w)
	require.NoError(s, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(s, err)
	return logger
}

func New(t TB, bvnCount int) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.Setup(bvnCount)
	return sim
}

func NewWithLogLevels(t TB, bvnCount int, logLevels config.LogLevel) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.LogLevels = logLevels.String()
	sim.Setup(bvnCount)
	return sim
}

func (sim *Simulator) Setup(bvnCount int) {
	sim.Helper()

	// Initialize the simulartor and network
	sim.routingOverrides = map[[32]byte]string{}
	sim.Logger = sim.newLogger().With("module", "simulator")
	sim.Executors = map[string]*ExecEntry{}
	sim.Subnets = make([]config.Subnet, bvnCount+1)
	sim.Subnets[0] = config.Subnet{Type: config.Directory, Id: protocol.Directory, BasePort: 30000}
	for i := 0; i < bvnCount; i++ {
		sim.Subnets[i+1] = config.Subnet{Type: config.BlockValidator, Id: fmt.Sprintf("BVN%d", i), BasePort: 30000}
	}

	mainEventBus := events.NewBus(sim.Logger.With("subnet", protocol.Directory))
	sim.router = routing.NewRouter(mainEventBus, nil)

	// Initialize each executor
	for i := range sim.Subnets {
		subnet := &sim.Subnets[i]
		subnet.Nodes = []config.Node{{Type: config.Validator, Address: subnet.Id}}

		logger := sim.newLogger().With("subnet", subnet.Id)
		key := acctesting.GenerateKey(sim.Name(), subnet.Id)
		db := database.OpenInMemory(logger)

		network := config.Describe{
			NetworkType:  subnet.Type,
			SubnetId:     subnet.Id,
			LocalAddress: subnet.Id,
			Network:      config.Network{Id: "simulator", Subnets: sim.Subnets},
		}

		eventBus := mainEventBus
		if subnet.Type != config.Directory {
			eventBus = events.NewBus(logger)
		}

		execOpts := ExecutorOptions{
			Logger:   logger,
			Key:      key,
			Describe: network,
			Router:   sim.Router(),
			EventBus: eventBus,
		}
		if execOpts.Describe.NetworkType == config.Directory {
			execOpts.MajorBlockScheduler = InitFakeMajorBlockScheduler(genesisTime)
		}
		exec, err := NewNodeExecutor(execOpts, db)
		require.NoError(sim, err)

		jrpc, err := api.NewJrpc(api.Options{
			Logger:        logger,
			Describe:      &network,
			Router:        sim.Router(),
			TxMaxWaitTime: time.Hour,
		})
		require.NoError(sim, err)

		sim.Executors[subnet.Id] = &ExecEntry{
			Database:   db,
			Executor:   exec,
			Subnet:     subnet,
			API:        acctesting.DirectJrpcClient(jrpc),
			tb:         sim.tb,
			blockTime:  genesisTime,
			Validators: [][]byte{key[32:]},
		}
	}
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
	return router{s, s.router}
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

func (s *Simulator) Query(url *url.URL, req query.Request, prove bool) interface{} {
	s.Helper()

	x := s.SubnetFor(url)
	return Query(s, x.Database, x.Executor, req, prove)
}

func (s *Simulator) InitFromGenesis() {
	s.InitFromGenesisWith(nil)
}

func (s *Simulator) InitFromGenesisWith(values *core.GlobalValues) {
	s.Helper()

	netValMap := make(genesis.NetworkValidatorMap)
	for _, x := range s.Executors {
		x.bootstrap = InitGenesis(s, x.Executor, genesisTime, values, netValMap)
	}

	// Execute bootstrap after the entire network is known
	for _, x := range s.Executors {
		err := x.bootstrap.Bootstrap()
		if err != nil {
			panic(fmt.Errorf("could not execute bootstrap: %v", err))
		}
		state, err := x.bootstrap.GetDBState()
		require.NoError(tb{s}, err)

		func() {
			batch := x.Database.Begin(true)
			defer batch.Discard()
			require.NoError(tb{s}, x.Executor.InitFromGenesis(batch, state))
			require.NoError(tb{s}, batch.Commit())
		}()
	}
}

func (s *Simulator) InitFromSnapshot(filename func(string) string) {
	s.Helper()

	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.Id)
		InitFromSnapshot(s, x.Database, x.Executor, filename(subnet.Id))
	}
}

// ExecuteBlock executes a block after submitting envelopes. If a status channel
// is provided, statuses will be sent to the channel as transactions are
// executed. Once the block is complete, the status channel will be closed (if
// provided).
func (s *Simulator) ExecuteBlock(statusChan chan<- *protocol.TransactionStatus) {
	s.Helper()

	if statusChan != nil {
		defer close(statusChan)
	}

	errg := new(errgroup.Group)
	for _, subnet := range s.Subnets {
		s.Subnet(subnet.Id).executeBlock(errg, statusChan)
	}

	// Wait for all subnets to complete
	err := errg.Wait()
	require.NoError(tb{s}, err)
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

	status, err := s.SubmitAndExecuteBlock(envelopes...)
	require.NoError(tb{s}, err)

	var didFail bool
	for _, status := range status {
		if status.Code == 0 {
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

// SubmitAndExecuteBlock executes a block with the envelopes.
func (s *Simulator) SubmitAndExecuteBlock(envelopes ...*protocol.Envelope) ([]*protocol.TransactionStatus, error) {
	s.Helper()

	_, err := s.Submit(envelopes...)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	ids := map[[32]byte]bool{}
	for _, env := range envelopes {
		for _, d := range NormalizeEnvelope(s, env) {
			ids[*(*[32]byte)(d.Transaction.GetHash())] = true
		}
	}

	ch1 := make(chan *protocol.TransactionStatus)
	ch2 := make(chan *protocol.TransactionStatus)
	go func() {
		s.ExecuteBlock(ch1)
		s.ExecuteBlock(ch2)
	}()

	status := make([]*protocol.TransactionStatus, 0, len(envelopes))
	for s := range ch1 {
		if ids[s.For] {
			status = append(status, s)
		}
	}
	for s := range ch2 {
		if ids[s.For] {
			status = append(status, s)
		}
	}

	return status, nil
}

func (s *Simulator) findTxn(status func(*protocol.TransactionStatus) bool, hash []byte) *ExecEntry {
	s.Helper()

	for _, subnet := range s.Subnets {
		x := s.Subnet(subnet.Id)

		batch := x.Database.Begin(false)
		defer batch.Discard()
		obj, err := batch.Transaction(hash).GetStatus()
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

func (s *Simulator) WaitForTransaction(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte, n int) (*protocol.Transaction, *protocol.TransactionStatus, []*url.TxID) {
	var x *ExecEntry
	for i := 0; i < n; i++ {
		x = s.findTxn(statusCheck, txnHash)
		if x != nil {
			break
		}

		s.ExecuteBlock(nil)
	}
	if x == nil {
		return nil, nil, nil
	}

	batch := x.Database.Begin(false)
	synth, err1 := batch.Transaction(txnHash).GetSyntheticTxns()
	state, err2 := batch.Transaction(txnHash).GetState()
	status, err3 := batch.Transaction(txnHash).GetStatus()
	batch.Discard()
	require.NoError(s, err1)
	require.NoError(s, err2)
	require.NoError(s, err3)
	return state.Transaction, status, synth.Entries
}

func (s *Simulator) WaitForTransactionFlow(statusCheck func(*protocol.TransactionStatus) bool, txnHash []byte) ([]*protocol.TransactionStatus, []*protocol.Transaction) {
	s.Helper()

	txn, status, synth := s.WaitForTransaction(statusCheck, txnHash, 50)
	if txn == nil {
		require.FailNow(s, fmt.Sprintf("Transaction %X has not been delivered after 50 blocks", txnHash[:4]))
		panic("unreachable")
	}

	status.For = *(*[32]byte)(txnHash)
	statuses := []*protocol.TransactionStatus{status}
	transactions := []*protocol.Transaction{txn}
	for _, id := range synth {
		// Wait for synthetic transactions to be delivered
		id := id.Hash()
		st, txn := s.WaitForTransactionFlow(func(status *protocol.TransactionStatus) bool {
			return status.Delivered
		}, id[:]) //nolint:rangevarref
		statuses = append(statuses, st...)
		transactions = append(transactions, txn...)
	}

	return statuses, transactions
}

type ExecEntry struct {
	tb
	mu                      sync.Mutex
	blockIndex              uint64
	blockTime               time.Time
	nextBlock, currentBlock []*protocol.Envelope

	Subnet     *config.Subnet
	Database   *database.Database
	Executor   *block.Executor
	bootstrap  genesis.Bootstrap
	API        *client.Client
	Validators [][]byte

	// SubmitHook can be used to control how envelopes are submitted to the
	// subnet. It is not safe to change SubmitHook concurrently with calls to
	// Submit.
	SubmitHook func([]*protocol.Envelope) []*protocol.Envelope
}

// Submit adds the envelopes to the next block's queue.
//
// By adding transactions to the next block and swaping queues when a block is
// executed, we roughly simulate the process Tendermint uses to build blocks.
func (x *ExecEntry) Submit(envelopes ...*protocol.Envelope) {
	// Capturing the field in a variable is more concurrency safe than using the
	// field directly
	if h := x.SubmitHook; h != nil {
		envelopes = h(envelopes)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.nextBlock = append(x.nextBlock, envelopes...)
}

// takeSubmitted returns the envelopes for the current block.
func (x *ExecEntry) takeSubmitted() []*protocol.Envelope {
	x.mu.Lock()
	defer x.mu.Unlock()
	submitted := x.currentBlock
	x.currentBlock = x.nextBlock
	x.nextBlock = nil
	return submitted
}

func (x *ExecEntry) executeBlock(errg *errgroup.Group, statusChan chan<- *protocol.TransactionStatus) {
	if x.blockIndex > 0 {
		x.blockIndex++
	} else {
		_ = x.Database.View(func(batch *database.Batch) error {
			var ledger *protocol.SystemLedger
			err := batch.Account(x.Executor.Describe.Ledger()).GetStateAs(&ledger)
			switch {
			case err == nil:
				x.blockIndex = ledger.Index + 1
			case errors.Is(err, errors.StatusNotFound):
				x.blockIndex = protocol.GenesisBlock + 1
			default:
				require.NoError(tb{x.tb}, err)
			}
			return nil
		})
	}
	x.blockTime = x.blockTime.Add(time.Second)
	block := new(Block)
	block.Index = x.blockIndex
	block.Time = x.blockTime
	block.IsLeader = true
	block.Batch = x.Database.Begin(true)

	// fmt.Printf("Executing %d\n", x.blockIndex)

	var deliveries []*chain.Delivery
	for _, envelope := range x.takeSubmitted() {
		d, err := chain.NormalizeEnvelope(envelope)
		require.NoErrorf(x, err, "Normalizing envelopes for %s", x.Executor.Describe.SubnetId)
		deliveries = append(deliveries, d...)
	}

	errg.Go(func() error {
		defer block.Batch.Discard()

		err := x.Executor.BeginBlock(block)
		require.NoError(x, err)

		for _, delivery := range deliveries {
			status, err := delivery.LoadTransaction(block.Batch)
			if errors.Is(err, errors.StatusDelivered) {
				if statusChan != nil {
					status.For = *(*[32]byte)(delivery.Transaction.GetHash())
					statusChan <- status
				}
				continue
			}
			if err != nil {
				return errors.Wrap(errors.StatusUnknown, err)
			}

			status, err = x.Executor.ExecuteEnvelope(block, delivery)
			if err != nil {
				return errors.Wrap(errors.StatusUnknown, err)
			}

			if statusChan != nil {
				status.For = *(*[32]byte)(delivery.Transaction.GetHash())
				statusChan <- status
			}
		}

		require.NoError(x, x.Executor.EndBlock(block))

		// Is the block empty?
		if !block.State.Empty() {
			// Commit the batch
			require.NoError(x, block.Batch.Commit())
		}

		// Apply validator updates
		for _, update := range block.State.ValidatorsUpdates {
			cmp := func(entry []byte) int {
				return bytes.Compare(entry, update.PubKey.Bytes())
			}
			if update.Enabled {
				ptr, new := sortutil.BinaryInsert(&x.Validators, cmp)
				if new {
					key := update.PubKey.Bytes()
					*ptr = make([]byte, len(key))
					copy(*ptr, key)
				}
			} else if i, found := sortutil.Search(x.Validators, cmp); found {
				copy(x.Validators[i:], x.Validators[i+1:])
				x.Validators = x.Validators[:len(x.Validators)-1]
			}
		}
		return nil
	})
}
