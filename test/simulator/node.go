// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
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
	peerID     peer.ID
	describe   config.Describe

	consensus *consensus.Node

	database *database.Database
	services *message.Handler
}

func (o *Options) newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.init = init
	n.simulator = s
	n.partition = p
	n.services, _ = message.NewHandler()
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

	// Set up services
	sk, err := crypto.UnmarshalEd25519PrivateKey(n.nodeKey)
	if err != nil {
		return nil, err
	}
	n.peerID, err = peer.IDFromPrivateKey(sk)
	if err != nil {
		return nil, err
	}

	_ = n.services.Register(
		message.ConsensusService{ConsensusService: n},
		message.Submitter{Submitter: n},
		message.Validator{Validator: n},
	)

	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeConsensus.AddressFor(n.partition.ID), n.services.Handle)
	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeSubmit.AddressFor(n.partition.ID), n.services.Handle)
	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeValidate.AddressFor(n.partition.ID), n.services.Handle)

	// This is hacky, but 🤷 I don't see another choice that wouldn't be
	// significantly less readable
	if p.Type == protocol.PartitionTypeDirectory && node == 0 {
		events.SubscribeSync(n.eventBus, s.router.willChangeGlobals)
	}

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
	querySvc := apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
	})

	// Create an Event service
	eventSvc := apiimpl.NewEventService(apiimpl.EventServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Network service
	netSvc := apiimpl.NewNetworkService(apiimpl.NetworkServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Sequencer service
	seqSvc := apiimpl.NewSequencer(apiimpl.SequencerParams{
		Logger:       n.logger.With("module", "acc-rpc"),
		Database:     n.database,
		EventBus:     n.eventBus,
		Partition:    n.partition.ID,
		ValidatorKey: n.privValKey,
	})

	// Register the services
	_ = n.services.Register(
		message.Querier{Querier: querySvc},
		message.EventService{EventService: eventSvc},
		message.NetworkService{NetworkService: netSvc},
		&message.Sequencer{Sequencer: seqSvc},
	)
	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeQuery.AddressFor(n.partition.ID), n.services.Handle)
	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeEvent.AddressFor(n.partition.ID), n.services.Handle)
	n.simulator.services.RegisterService(n.peerID, api.ServiceTypeNetwork.AddressFor(n.partition.ID), n.services.Handle)
	n.simulator.services.RegisterService(n.peerID, private.ServiceTypeSequencer.AddressFor(n.partition.ID), n.services.Handle)

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
		Sequencer:     n.simulator.Services().Private(),
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

// ConsensusStatus implements [api.ConsensusService].
func (n *Node) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	status, err := n.consensus.Status(&consensus.StatusRequest{})
	if err != nil {
		return nil, err
	}
	info, err := n.consensus.Info(&consensus.InfoRequest{})
	if err != nil {
		return nil, err
	}
	return &api.ConsensusStatus{
		Ok: true,
		LastBlock: &api.LastBlock{
			Height:    int64(status.BlockIndex),
			Time:      status.BlockTime,
			StateRoot: info.LastHash,
			// TODO: chain root, directory height
		},
		NodeKeyHash:      sha256.Sum256(n.nodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(n.privValKey[32:]),
		PartitionID:      n.partition.ID,
		PartitionType:    n.partition.Type,
	}, nil
}
func (n *Node) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return n.submit(envelope, false)
}

func (n *Node) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return n.submit(envelope, true)
}

func (n *Node) submit(envelope *messaging.Envelope, pretend bool) ([]*api.Submission, error) {
	st, err := n.partition.Submit(envelope, pretend)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	subs := make([]*api.Submission, len(st))
	for i, st := range st {
		// Create an api.Submission
		subs[i] = new(api.Submission)
		subs[i].Status = st
		subs[i].Success = st.Code.Success()
		if st.Error != nil {
			subs[i].Message = st.Error.Message
		}
	}

	return subs, nil
}
