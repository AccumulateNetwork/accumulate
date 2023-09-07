// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Node struct {
	id         int
	init       *accumulated.NodeInit
	simulator  *Simulator
	partition  *Partition
	logger     logging.OptionalLogger
	eventBus   *events.Bus
	database   *database.Database
	nodeKey    []byte
	privValKey []byte
	describe   config.Describe

	consensus *consensus.Node

	apiV2    *apiv2.JrpcMethods
	clientV2 *client.Client
	querySvc api.Querier
	eventSvc api.EventService
	netSvc   api.NetworkService
	seqSvc   private.Sequencer
}

func newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.init = init
	n.simulator = s
	n.partition = p
	n.logger.Set(p.logger, "node", node)
	n.eventBus = events.NewBus(n.logger)
	n.privValKey = init.PrivValKey
	switch p.Type {
	case protocol.PartitionTypeDirectory:
		n.nodeKey = init.DnNodeKey
	case protocol.PartitionTypeBlockValidator:
		n.nodeKey = init.BvnNodeKey
	case protocol.PartitionTypeBlockSummary:
		n.nodeKey = init.BsnNodeKey
	default:
		return nil, errors.InternalError.WithFormat("unknown partition type %v", p.Type)
	}

	var rec consensus.Recorder
	if s.Recordings != nil {
		// Set up the recorder
		f, err := s.Recordings(p.ID, node)
		if err != nil {
			return nil, errors.InternalError.WithFormat("open record file: %w", err)
		}
		r := newRecorder(f)
		rec = r

		// Record the header
		err = r.WriteHeader(&recordHeader{
			Partition: &p.PartitionInfo,
			Config:    init,
			NodeID:    fmt.Sprint(node),
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// This is hacky, but 🤷 I don't see another choice that wouldn't be
	// significantly less readable
	if p.Type == protocol.PartitionTypeDirectory && node == 0 {
		events.SubscribeSync(n.eventBus, s.router.willChangeGlobals)
	}

	var err error
	switch n.partition.Type {
	case protocol.PartitionTypeDirectory,
		protocol.PartitionTypeBlockValidator:
		err = n.initValidator(rec)
	case protocol.PartitionTypeBlockSummary:
		err = n.initSummary(rec)
	default:
		err = errors.BadRequest.WithFormat("unknown partition type %v", n.partition.Type)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect and submit block summaries
	if s.init.Bsn != nil && n.partition.Type != protocol.PartitionTypeBlockSummary {
		err := n.initCollector()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return n, nil
}

func (n *Node) initValidator(rec consensus.Recorder) error {
	store := n.simulator.database(n.partition.ID, n.id, n.logger)
	n.database = database.New(store, n.logger)

	// Create a Querier service
	n.querySvc = apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n,
		Partition: n.partition.ID,
	})

	// Create an Event service
	n.eventSvc = apiimpl.NewEventService(apiimpl.EventServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Network service
	n.netSvc = apiimpl.NewNetworkService(apiimpl.NetworkServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Sequencer service
	n.seqSvc = apiimpl.NewSequencer(apiimpl.SequencerParams{
		Logger:       n.logger.With("module", "acc-rpc"),
		Database:     n,
		EventBus:     n.eventBus,
		Partition:    n.partition.ID,
		ValidatorKey: n.privValKey,
	})

	// Describe the network, from the node's perspective
	n.describe = config.Describe{
		NetworkType:  n.partition.Type,
		PartitionId:  n.partition.ID,
		LocalAddress: n.init.AdvertizeAddress,
		Network:      *n.simulator.netcfg,
	}

	// Set up the executor options
	execOpts := block.ExecutorOptions{
		Logger:        n.logger,
		Database:      n,
		Key:           n.init.PrivValKey,
		Describe:      n.describe,
		Router:        n.simulator.router,
		EventBus:      n.eventBus,
		EnableHealing: true,
		NewDispatcher: n.simulator.newDispatcher,
		Sequencer:     n.simulator.Services(),
		Querier:       n.simulator.Services(),
	}

	// Add background tasks to the block's error group. The simulator must call
	// Group.Wait before changing the group, to ensure no race conditions.
	execOpts.BackgroundTaskLauncher = func(f func()) {
		n.simulator.blockErrGroup.Go(func() error {
			f()
			return nil
		})
	}

	// Initialize the major block scheduler
	if n.partition.Type == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(n.eventBus)
	}

	// Set up the API
	var err error
	n.apiV2, err = apiv2.NewJrpc(apiv2.Options{
		Logger:        n.logger,
		TxMaxWaitTime: time.Hour,
		Describe:      &n.describe,
		LocalV3:       (*nodeService)(n),
		Querier:       (*simService)(n.simulator),
		Submitter:     (*simService)(n.simulator),
		Network:       (*simService)(n.simulator),
		Faucet:        (*simService)(n.simulator),
		Validator:     (*simService)(n.simulator),
		Sequencer:     (*simService)(n.simulator),
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Create an API client
	n.clientV2 = testing.DirectJrpcClient(n.apiV2)

	// Create the executor
	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Create the consensus node
	n.consensus = consensus.NewNode(&consensus.ExecutorApp{Executor: exec, EventBus: n.eventBus, Database: n.database, Describe: &n.describe}, rec)
	return nil
}

func (n *Node) initCollector() error {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: n.partition.ID,
		Database:  n,
		Events:    n.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("start collector: %w", err)
	}

	signer := nodeSigner{n}
	events.SubscribeAsync(n.eventBus, func(e bsn.DidCollectBlock) {
		env, err := build.SignatureForMessage(e.Summary).
			Url(protocol.PartitionUrl(n.partition.ID)).
			Signer(signer).
			Done()
		if err != nil {
			n.logger.Error("Failed to sign block summary", "error", err)
			return
		}

		msg := new(messaging.BlockAnchor)
		msg.Anchor = e.Summary
		msg.Signature = env.Signatures[0].(protocol.KeySignature)

		st, err := n.simulator.SubmitTo(n.simulator.init.Bsn.Id, &messaging.Envelope{Messages: []messaging.Message{msg}})
		if err != nil {
			n.logger.Error("Failed to submit block summary envelope", "error", err)
			return
		}
		for _, st := range st {
			if st.Error != nil {
				n.logger.Error("Block summary envelope failed", "error", st.AsError())
			}
		}
	})

	return nil
}

func (n *Node) initSummary(rec consensus.Recorder) error {
	exec, err := bsn.NewExecutor(bsn.ExecutorOptions{
		PartitionID: n.partition.ID,
		Logger:      n.logger,
		Store:       n.simulator.database(n.partition.ID, n.id, n.logger),
		EventBus:    n.eventBus,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	n.consensus = consensus.NewNode(&consensus.ExecutorApp{Executor: exec, EventBus: n.eventBus, Database: n.database, Describe: &n.describe}, rec)
	return nil
}

func (n *Node) Begin(writable bool) *database.Batch         { return n.database.Begin(writable) }
func (n *Node) Update(fn func(*database.Batch) error) error { return n.database.Update(fn) }
func (n *Node) View(fn func(*database.Batch) error) error   { return n.database.View(fn) }
func (n *Node) SetObserver(observer database.Observer)      { n.database.SetObserver(observer) }

func (n *Node) initChain(snapshot ioutil2.SectionReader) ([]byte, error) {
	res, err := n.consensus.Init(&consensus.InitRequest{Snapshot: snapshot})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.Hash, nil
}

func (n *Node) checkTx(envelope *messaging.Envelope, new bool) ([]*protocol.TransactionStatus, error) {
	res, err := n.consensus.Check(&consensus.CheckRequest{Envelope: envelope, New: new})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.Results, nil
}

func (n *Node) beginBlock(params execute.BlockParams) (execute.Block, error) {
	res, err := n.consensus.Begin(&consensus.BeginRequest{Params: params})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.Block, nil
}

func (n *Node) deliverTx(block execute.Block, envelopes []*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	return n.consensus.Deliver(block, envelopes)
}

func (n *Node) endBlock(block execute.Block) (execute.BlockState, error) {
	return n.consensus.EndBlock(block)
}

func (n *Node) commit(state execute.BlockState) ([]byte, error) {
	return n.consensus.Commit(state)
}
