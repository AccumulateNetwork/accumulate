package simulator

//lint:file-ignore ST1001 Don't care

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

type SimulatorOptions struct {
	BvnCount        int
	LogLevels       string
	OpenDB          func(partition string, nodeIndex int, logger log.Logger) *database.Database
	FactomAddresses func() (io.Reader, error)
}

type Simulator struct {
	tb
	Logger     log.Logger
	Partitions []config.Partition
	Executors  map[string]*ExecEntry

	opts             SimulatorOptions
	netInit          *accumulated.NetworkInit
	router           routing.Router
	routingOverrides map[[32]byte]string
}

func (s *Simulator) newLogger(opts SimulatorOptions) log.Logger {
	if !acctesting.LogConsole {
		return logging.NewTestLogger(s, "plain", opts.LogLevels, false)
	}

	w, err := logging.NewConsoleWriter("plain")
	require.NoError(s, err)
	level, writer, err := logging.ParseLogLevel(opts.LogLevels, w)
	require.NoError(s, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(s, err)
	return logger
}

func New(t TB, bvnCount int) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.Setup(SimulatorOptions{BvnCount: bvnCount})
	return sim
}

func NewWith(t TB, opts SimulatorOptions) *Simulator {
	t.Helper()
	sim := new(Simulator)
	sim.TB = t
	sim.Setup(opts)
	return sim
}

func (sim *Simulator) Setup(opts SimulatorOptions) {
	sim.Helper()

	if opts.BvnCount == 0 {
		opts.BvnCount = 3
	}
	if opts.LogLevels == "" {
		opts.LogLevels = acctesting.DefaultLogLevels
	}
	if opts.OpenDB == nil {
		opts.OpenDB = func(_ string, _ int, logger log.Logger) *database.Database {
			return database.OpenInMemory(logger)
		}
	}
	sim.opts = opts

	// Initialize the simulartor and network
	sim.routingOverrides = map[[32]byte]string{}
	sim.Logger = sim.newLogger(opts).With("module", "simulator")
	sim.Executors = map[string]*ExecEntry{}

	sim.netInit = new(accumulated.NetworkInit)
	sim.netInit.Id = sim.Name()
	for i := 0; i < opts.BvnCount; i++ {
		bvnInit := new(accumulated.BvnInit)
		bvnInit.Id = fmt.Sprintf("BVN%d", i)
		bvnInit.Nodes = []*accumulated.NodeInit{{
			DnnType:    config.Validator,
			BvnnType:   config.Validator,
			PrivValKey: acctesting.GenerateKey(sim.Name(), bvnInit.Id),
		}}
		sim.netInit.Bvns = append(sim.netInit.Bvns, bvnInit)
	}

	sim.Partitions = make([]config.Partition, 1)
	sim.Partitions[0] = config.Partition{Type: config.Directory, Id: protocol.Directory, BasePort: 30000}
	for _, bvn := range sim.netInit.Bvns {
		partition := config.Partition{Type: config.BlockValidator, Id: bvn.Id, BasePort: 30000}
		sim.Partitions = append(sim.Partitions, partition)
	}

	mainEventBus := events.NewBus(sim.Logger.With("partition", protocol.Directory))
	events.SubscribeSync(mainEventBus, sim.willChangeGlobals)
	sim.router = routing.NewRouter(mainEventBus, nil)

	// Initialize each executor
	for i, bvn := range sim.netInit.Bvns[:1] {
		// TODO Initialize multiple executors for the DN
		dn := &sim.Partitions[0]
		dn.Nodes = append(dn.Nodes, config.Node{Type: config.Validator, Address: protocol.Directory})

		x := new(ExecEntry)
		x.Partition = dn
		x.tb = sim.tb
		x.blockTime = GenesisTime
		x.Validators = [][]byte{bvn.Nodes[0].PrivValKey[32:]}
		sim.Executors[protocol.Directory] = x

		logger := sim.newLogger(opts).With("partition", protocol.Directory)
		x.Database = opts.OpenDB(protocol.Directory, i, logger)

		network := config.Describe{
			NetworkType:  config.Directory,
			PartitionId:  protocol.Directory,
			LocalAddress: protocol.Directory,
			Network:      config.Network{Id: "simulator", Partitions: sim.Partitions},
		}

		execOpts := block.ExecutorOptions{
			Logger:   logger,
			Key:      bvn.Nodes[0].PrivValKey,
			Describe: network,
			Router:   sim.Router(),
			EventBus: mainEventBus,
		}
		if execOpts.Describe.NetworkType == config.Directory {
			execOpts.MajorBlockScheduler = blockscheduler.Init(mainEventBus)
		}
		var err error
		x.Executor, err = block.NewNodeExecutor(execOpts, x)
		require.NoError(sim, err)

		jrpc, err := api.NewJrpc(api.Options{
			Logger:        logger,
			Describe:      &network,
			Router:        sim.Router(),
			TxMaxWaitTime: time.Hour,
			Database:      x,
			Key:           execOpts.Key,
		})
		require.NoError(sim, err)
		x.API = acctesting.DirectJrpcClient(jrpc)
	}

	for i, bvnInit := range sim.netInit.Bvns {
		bvn := &sim.Partitions[i+1]
		bvn.Nodes = []config.Node{{Type: config.Validator, Address: bvn.Id}}

		x := new(ExecEntry)
		x.Partition = bvn
		x.tb = sim.tb
		x.blockTime = GenesisTime
		x.Validators = [][]byte{bvnInit.Nodes[0].PrivValKey[32:]}
		sim.Executors[bvn.Id] = x

		logger := sim.newLogger(opts).With("partition", bvn.Id)
		x.Database = opts.OpenDB(bvn.Id, 0, logger)

		network := config.Describe{
			NetworkType:  bvn.Type,
			PartitionId:  bvn.Id,
			LocalAddress: bvn.Id,
			Network:      config.Network{Id: "simulator", Partitions: sim.Partitions},
		}

		execOpts := block.ExecutorOptions{
			Logger:   logger,
			Key:      bvnInit.Nodes[0].PrivValKey,
			Describe: network,
			Router:   sim.Router(),
			EventBus: events.NewBus(logger),
		}
		var err error
		x.Executor, err = block.NewNodeExecutor(execOpts, x)
		require.NoError(sim, err)

		jrpc, err := api.NewJrpc(api.Options{
			Logger:        logger,
			Describe:      &network,
			Router:        sim.Router(),
			TxMaxWaitTime: time.Hour,
			Database:      x,
			Key:           execOpts.Key,
		})
		require.NoError(sim, err)
		x.API = acctesting.DirectJrpcClient(jrpc)
	}
}

// willChangeGlobals is called when global values are about to change.
// willChangeGlobals is responsible for updating the validator list.
func (s *Simulator) willChangeGlobals(e events.WillChangeGlobals) error {
	for id, x := range s.Executors {
		updates, err := e.Old.DiffValidators(e.New, id)
		if err != nil {
			return err
		}

		for key, typ := range updates {
			key := key // See docs/developer/rangevarref.md
			cmp := func(entry []byte) int {
				return bytes.Compare(entry, key[:])
			}
			switch typ {
			case core.ValidatorUpdateAdd:
				ptr, new := sortutil.BinaryInsert(&x.Validators, cmp)
				if !new {
					break
				}

				*ptr = key[:]

			case core.ValidatorUpdateRemove:
				i, found := sortutil.Search(x.Validators, cmp)
				if !found {
					break
				}

				copy(x.Validators[i:], x.Validators[i+1:])
				x.Validators = x.Validators[:len(x.Validators)-1]
			}
		}
	}
	return nil
}

func (s *Simulator) SetRouteFor(account *url.URL, partition string) {
	// Make sure the account is a root identity
	if !account.RootIdentity().Equal(account) {
		s.Fatalf("Cannot set the route for a non-root: %v", account)
	}

	// Make sure the partition exists
	s.Partition(partition)

	// Add/remove the override
	if partition == "" {
		delete(s.routingOverrides, account.AccountID32())
	} else {
		s.routingOverrides[account.AccountID32()] = partition
	}
}

func (s *Simulator) Router() routing.Router {
	return router{s, s.router}
}

func (s *Simulator) Partition(id string) *ExecEntry {
	e, ok := s.Executors[id]
	require.Truef(s, ok, "Unknown partition %q", id)
	return e
}

func (s *Simulator) PartitionFor(url *url.URL) *ExecEntry {
	s.Helper()

	partition, err := s.Router().RouteAccount(url)
	require.NoError(s, err)
	return s.Partition(partition)
}

func QueryUrl[T any](s *Simulator, url *url.URL, prove bool) T {
	s.Helper()
	req := new(api.GeneralQuery)
	req.Url = url
	req.Prove = prove
	var resp T
	require.NoError(s, s.PartitionFor(url).API.RequestAPIv2(context.Background(), "query", req, &resp))
	return resp
}

// RunAndReset runs everything in a batch (which is discarded) then resets the
// simulator state.
func (s *Simulator) RunAndReset(fn func()) {
	for _, x := range s.Executors {
		old, new := x.Database, x.Database.Begin(true)
		x.Database = new
		defer func(x *ExecEntry) {
			x.Database = old
			new.Discard()

			x.BlockIndex = 0
			x.blockTime = GenesisTime
			x.nextBlock = nil
			x.currentBlock = nil
		}(x)
	}

	fn()
}

func (s *Simulator) InitFromGenesis() {
	s.InitFromGenesisWith(nil)
}

func (s *Simulator) InitFromGenesisWith(values *core.GlobalValues) {
	s.Helper()

	if values == nil {
		values = new(core.GlobalValues)
	}
	genDocs, err := accumulated.BuildGenesisDocs(s.netInit, values, GenesisTime, s.Logger, s.opts.FactomAddresses)
	require.NoError(s, err)

	// Execute bootstrap after the entire network is known
	for _, x := range s.Executors {
		batch := x.Database.Begin(true)
		defer batch.Discard()
		var snapshot []byte
		require.NoError(s, json.Unmarshal(genDocs[x.Partition.Id].AppState, &snapshot))
		require.NoError(tb{s}, x.Executor.RestoreSnapshot(batch, ioutil2.NewBuffer(snapshot)))
		require.NoError(tb{s}, batch.Commit())
	}
}

func (s *Simulator) InitFromSnapshot(filename func(string) string) {
	s.Helper()

	for _, partition := range s.Partitions {
		x := s.Partition(partition.Id)
		InitFromSnapshot(s, x.Database, x.Executor, filename(partition.Id))
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
	for _, partition := range s.Partitions {
		s.Partition(partition.Id).executeBlock(errg, statusChan)
	}

	// Wait for all partitions to complete
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
		partition, err := s.Router().Route(envelope)
		require.NoError(s, err)
		x := s.Partition(partition)

		// Normalize - use a copy to avoid weird issues caused by modifying values
		deliveries, err := chain.NormalizeEnvelope(envelope.Copy())
		if err != nil {
			return nil, err
		}

		// Check
		batch := x.Database.Begin(false)
		defer batch.Discard()
		x.Executor.ValidateEnvelopeSet(batch, deliveries, func(e error, _ *chain.Delivery, _ *protocol.TransactionStatus) {
			err = e
		})
		if err != nil {
			return nil, err
		}

		// Enqueue
		x.Submit(false, envelope)
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
		if status.Code.Success() {
			continue
		}

		if status.Error != nil {
			assert.NoError(s, status.Error)
		} else {
			assert.False(s, status.Failed())
		}
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
		return nil, errors.Wrap(errors.StatusUnknownError, err)
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
		if ids[s.TxID.Hash()] {
			status = append(status, s)
		}
	}
	for s := range ch2 {
		if ids[s.TxID.Hash()] {
			status = append(status, s)
		}
	}

	return status, nil
}

func (s *Simulator) findTxn(status func(*protocol.TransactionStatus) bool, hash []byte) *ExecEntry {
	s.Helper()

	for _, partition := range s.Partitions {
		x := s.Partition(partition.Id)

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

type ExecEntry struct {
	tb
	mu                      sync.Mutex
	BlockIndex              uint64
	blockTime               time.Time
	nextBlock, currentBlock []*chain.Delivery

	Partition  *config.Partition
	Database   database.Beginner
	Executor   *block.Executor
	API        *client.Client
	Validators [][]byte

	// SubmitHook can be used to control how envelopes are submitted to the
	// partition. It is not safe to change SubmitHook concurrently with calls to
	// Submit.
	SubmitHook func([]*chain.Delivery) ([]*chain.Delivery, bool)
}

func (x *ExecEntry) Begin(writable bool) *database.Batch         { return x.Database.Begin(writable) }
func (x *ExecEntry) Update(fn func(*database.Batch) error) error { return x.Database.Update(fn) }
func (x *ExecEntry) View(fn func(*database.Batch) error) error   { return x.Database.View(fn) }

// Submit adds the envelopes to the next block's queue.
//
// By adding transactions to the next block and swaping queues when a block is
// executed, we roughly simulate the process Tendermint uses to build blocks.
func (x *ExecEntry) Submit(pretend bool, envelopes ...*protocol.Envelope) []*chain.Delivery {
	var deliveries []*chain.Delivery
	for _, env := range envelopes {
		normalized, err := chain.NormalizeEnvelope(env)
		require.NoErrorf(x, err, "Normalizing envelopes for %s", x.Executor.Describe.PartitionId)
		deliveries = append(deliveries, normalized...)
	}

	// Capturing the field in a variable is more concurrency safe than using the
	// field directly
	if hook := x.SubmitHook; hook != nil {
		var keep bool
		deliveries, keep = hook(deliveries)
		if !keep {
			x.SubmitHook = nil
		}
	}

	if pretend {
		return deliveries
	}

	x.mu.Lock()
	defer x.mu.Unlock()
	x.nextBlock = append(x.nextBlock, deliveries...)
	return deliveries
}

// takeSubmitted returns the envelopes for the current block.
func (x *ExecEntry) takeSubmitted() []*chain.Delivery {
	x.mu.Lock()
	defer x.mu.Unlock()
	submitted := x.currentBlock
	x.currentBlock = x.nextBlock
	x.nextBlock = nil
	return submitted
}

func (x *ExecEntry) executeBlock(errg *errgroup.Group, statusChan chan<- *protocol.TransactionStatus) {
	if x.BlockIndex > 0 {
		x.BlockIndex++
	} else {
		_ = x.Database.View(func(batch *database.Batch) error {
			var ledger *protocol.SystemLedger
			err := batch.Account(x.Executor.Describe.Ledger()).GetStateAs(&ledger)
			switch {
			case err == nil:
				x.BlockIndex = ledger.Index + 1
			case errors.Is(err, errors.StatusNotFound):
				x.BlockIndex = protocol.GenesisBlock + 1
			default:
				require.NoError(tb{x.tb}, err)
			}
			return nil
		})
	}
	x.blockTime = x.blockTime.Add(time.Second)
	block := new(block.Block)
	block.Index = x.BlockIndex
	block.Time = x.blockTime
	block.IsLeader = true
	block.Batch = x.Database.Begin(true)

	// Run background tasks in the error group to ensure they complete before the next block begins
	x.Executor.Background = func(f func()) { errg.Go(func() error { f(); return nil }) }

	deliveries := x.takeSubmitted()
	errg.Go(func() error {
		defer block.Batch.Discard()

		err := x.Executor.BeginBlock(block)
		require.NoError(x, err)

		for i := 0; i < len(deliveries); i++ {
			status, err := deliveries[i].LoadTransaction(block.Batch)
			if err == nil {
				continue
			}
			if !errors.Is(err, errors.StatusDelivered) {
				return errors.Wrap(errors.StatusUnknownError, err)
			}
			if statusChan != nil {
				status.TxID = deliveries[i].Transaction.ID()
				statusChan <- status
			}
			deliveries = append(deliveries[:i], deliveries[i+1:]...)
			i--
		}

		results := x.Executor.ExecuteEnvelopeSet(block, deliveries, func(e error, _ *chain.Delivery, _ *protocol.TransactionStatus) {
			err = e
		})
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
		if statusChan != nil {
			for _, result := range results {
				statusChan <- result
			}
		}

		require.NoError(x, x.Executor.EndBlock(block))

		// Is the block empty?
		if !block.State.Empty() {
			// Commit the batch
			require.NoError(x, block.Batch.Commit())
		}

		return nil
	})
}
