// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (d *Daemon) startCollector() error {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: d.Config.Accumulate.PartitionId,
		Database:  d.db,
		Events:    d.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("start collector: %w", err)
	}

	client := &message.Client{Transport: &message.RoutedTransport{
		Network: d.Config.Accumulate.Network.Id,
		Dialer:  d.p2pnode.DialNetwork(),
		Router:  routing.MessageRouter{Router: summaryRouter(d.Config.Accumulate.SummaryNetwork)},
	}}

	events.SubscribeAsync(d.eventBus, func(e bsn.DidCollectBlock) {
		/*// Wait for the summary network to appear
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		err := d.p2pnode.WaitForService(ctx, api.ServiceTypeSubmit.AddressFor(d.Config.Accumulate.SummaryNetwork).Multiaddr())
		if err != nil {
			d.Logger.Error("Summary network did not appear after an hour", "error", err)
			return
		}//*/

		env, err := build.SignatureForMessage(e.Summary).
			Url(protocol.PartitionUrl(d.Config.Accumulate.PartitionId)).
			PrivateKey(d.privVal.Key.PrivKey.Bytes()).
			Done()
		if err != nil {
			d.Logger.Error("Failed to sign block summary", "error", err)
			return
		}

		msg := new(messaging.BlockAnchor)
		msg.Anchor = e.Summary
		msg.Signature = env.Signatures[0].(protocol.KeySignature)

		sub, err := client.Submit(context.Background(), &messaging.Envelope{Messages: []messaging.Message{msg}}, v3.SubmitOptions{})
		if err != nil {
			d.Logger.Error("Failed to submit block summary envelope", "error", err)
			return
		}
		for _, sub := range sub {
			if !sub.Success {
				d.Logger.Error("Block summary envelope failed", "error", sub.Message)
			}
		}
	})
	return nil
}

func (d *Daemon) startSummary() error {
	// Setup the p2p node
	var err error
	if d.p2pnode == nil {
		d.p2pnode, err = p2p.New(p2p.Options{
			Logger:         d.Logger.With("module", "acc-rpc"),
			Network:        d.Config.Accumulate.Network.Id,
			Listen:         d.Config.Accumulate.P2P.Listen,
			BootstrapPeers: d.Config.Accumulate.P2P.BootstrapPeers,
			Key:            ed25519.PrivateKey(d.nodeKey.PrivKey.Bytes()),
			DiscoveryMode:  dht.ModeServer,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("initialize P2P: %w", err)
		}
	}

	// Setup the executor and ABCI
	app, err := d.startSummaryApp()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start Tendermint
	err = d.startConsensus(app)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start services
	err = d.startSummaryServices()
	return errors.UnknownError.Wrap(err)
}

func (d *Daemon) startSummaryApp() (types.Application, error) {
	var store keyvalue.Beginner
	var err error
	switch d.Config.Accumulate.Storage.Type {
	case config.MemoryStorage:
		store = memory.New(nil)
	case config.BadgerStorage:
		store, err = badger.New(config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Storage.Path), d.Logger.With("module", "storage"))
	default:
		return nil, errors.BadRequest.WithFormat("unknown storage format %q", d.Config.Accumulate.Storage.Type)
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load database: %w", err)
	}

	exec, err := bsn.NewExecutor(bsn.ExecutorOptions{
		PartitionID: d.Config.Accumulate.PartitionId,
		Logger:      d.Logger,
		Store:       store,
		EventBus:    d.eventBus,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start executor: %w", err)
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		Address:  d.Key().PubKey().Address(),
		Executor: exec,
		Logger:   d.Logger,
		EventBus: d.eventBus,
		Config:   d.Config,
		Tracer:   d.tracer,
	})

	return app, nil
}

func (d *Daemon) startSummaryServices() error {
	// Initialize all the services
	nodeSvc := tm.NewConsensusService(tm.ConsensusServiceParams{
		Logger:           d.Logger.With("module", "acc-rpc"),
		Local:            d.localTm,
		PartitionID:      d.Config.Accumulate.PartitionId,
		PartitionType:    d.Config.Accumulate.NetworkType,
		EventBus:         d.eventBus,
		NodeKeyHash:      sha256.Sum256(d.nodeKey.PubKey().Bytes()),
		ValidatorKeyHash: sha256.Sum256(d.privVal.Key.PubKey.Bytes()),
	})
	submitSvc := tm.NewSubmitter(tm.SubmitterParams{
		Logger: d.Logger.With("module", "acc-rpc"),
		Local:  d.localTm,
	})
	validateSvc := tm.NewValidator(tm.ValidatorParams{
		Logger: d.Logger.With("module", "acc-rpc"),
		Local:  d.localTm,
	})
	messageHandler, err := message.NewHandler(
		d.Logger.With("module", "acc-rpc"),
		&message.ConsensusService{ConsensusService: nodeSvc},
		&message.Submitter{Submitter: submitSvc},
		&message.Validator{Validator: validateSvc},
	)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize P2P handler: %w", err)
	}

	services := []interface{ Type() v3.ServiceType }{
		nodeSvc,
		submitSvc,
		validateSvc,
	}
	for _, s := range services {
		d.p2pnode.RegisterService(&v3.ServiceAddress{
			Type:     s.Type(),
			Argument: d.Config.Accumulate.PartitionId,
		}, messageHandler.Handle)
	}

	return nil
}

type summaryRouter string

func (r summaryRouter) RouteAccount(*url.URL) (string, error) {
	return string(r), nil
}

func (r summaryRouter) Route(env ...*messaging.Envelope) (string, error) {
	return string(r), nil
}
