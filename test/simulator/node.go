// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

type Node struct {
	id         int
	init       *accumulated.NodeInit
	simulator  *Simulator
	partition  *Partition
	logger     logging.OptionalLogger
	eventBus   *events.Bus
	nodeKey    []byte
	privValKey []byte
	describe   config.Describe

	consensus *consensus.Node

	database *database.Database
	querySvc api.Querier
	eventSvc api.EventService
	netSvc   api.NetworkService
	seqSvc   private.Sequencer
}

func (o *Options) newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
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

	// This is hacky, but 🤷 I don't see another choice that wouldn't be
	// significantly less readable
	if p.Type == protocol.PartitionTypeDirectory && node == 0 {
		events.SubscribeSync(n.eventBus, s.router.willChangeGlobals)
	}

	var err error
	switch n.partition.Type {
	case protocol.PartitionTypeDirectory,
		protocol.PartitionTypeBlockValidator:
		err = n.initValidator(o)
	case protocol.PartitionTypeBlockSummary:
		err = n.initSummary(o)
	default:
		err = errors.BadRequest.WithFormat("unknown partition type %v", n.partition.Type)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect and submit block summaries
	if s.opts.network.Bsn != nil && n.partition.Type != protocol.PartitionTypeBlockSummary {
		err := n.initCollector()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	if s.opts.recordings != nil {
		// Set up the recorder
		f, err := s.opts.recordings(p.ID, node)
		if err != nil {
			return nil, errors.InternalError.WithFormat("open record file: %w", err)
		}
		r := newRecorder(f)

		// Record the header
		err = r.WriteHeader(&recordHeader{
			Partition: &p.PartitionInfo,
			Config:    init,
			NodeID:    fmt.Sprint(node),
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		n.consensus.SetRecorder(r)
	}

	return n, nil
}

func (n *Node) initValidator(o *Options) error {
	store := o.database(n.partition.ID, n.id, n.logger)
	n.database = database.New(store, n.logger)

	// Create a Querier service
	n.querySvc = apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
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
		Database:     n.database,
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
		Database:      n.database,
		Key:           n.init.PrivValKey,
		Describe:      n.describe,
		Router:        n.simulator.router,
		EventBus:      n.eventBus,
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

	// Create an executor
	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	app, err := o.application(n, exec)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Create the consensus node
	n.consensus = n.newConsensusNode(o, app)
	return nil
}

func (n *Node) initCollector() error {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: n.partition.ID,
		Database:  n.database,
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

		st, err := n.simulator.SubmitTo(n.simulator.opts.network.Bsn.Id, &messaging.Envelope{Messages: []messaging.Message{msg}})
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

func (n *Node) initSummary(o *Options) error {
	exec, err := bsn.NewExecutor(bsn.ExecutorOptions{
		PartitionID: n.partition.ID,
		Logger:      n.logger,
		Store:       o.database(n.partition.ID, n.id, n.logger),
		EventBus:    n.eventBus,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	app, err := o.application(n, exec)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	n.consensus = n.newConsensusNode(o, app)
	return nil
}

func (n *Node) newConsensusNode(o *Options, app consensus.App) *consensus.Node {
	cn := consensus.NewNode(n.privValKey, app, n.partition.gossip, n.logger)
	cn.SkipProposalCheck = o.skipProposalCheck
	cn.IgnoreDeliverResults = o.ignoreDeliverResults
	cn.IgnoreCommitResults = o.ignoreCommitResults
	return cn
}
