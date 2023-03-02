// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// startDHT bootstraps a DHT instance.
func startDHT(host host.Host, logger log.Logger, ctx context.Context, mode dht.ModeOpt, bootstrapPeers []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
	// Allocate a DHT
	d, err := dht.New(ctx, host, dht.Mode(mode))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create DHT: %w", err)
	}

	// Connect to the bootstrap peers
	wg := new(sync.WaitGroup)
	for _, addr := range bootstrapPeers {
		pi, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("parse address: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			err := host.Connect(ctx, *pi)
			if err != nil {
				logger.Info("Unable to connect to bootstrap peer", "error", err)
			}
		}()
	}
	wg.Wait()

	// Bootstrap the DHT?
	err = d.Bootstrap(ctx)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("bootstrap DHT: %w", err)
	}

	return d, nil
}

// startServiceDiscovery sets up pubsub for node events.
func startServiceDiscovery(ctx context.Context, host host.Host, logger log.Logger) (chan<- Event, <-chan Event, error) {
	// Create the pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		return nil, nil, err
	}

	// Join the topic
	topic, err := ps.Join(api.ServiceTypeNode.Address().String())
	if err != nil {
		return nil, nil, err
	}

	// Subscribe
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, nil, err
	}

	// Parse events and forward them to a channel
	recv := make(chan Event)
	go func() {
		defer close(recv)

		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logger.Error("Failed to get next topic message", "error", err)
				return
			}

			event, err := UnmarshalEvent(msg.Data)
			if err != nil {
				logger.Info("Received bad message", "error", err)
				continue
			}

			select {
			case recv <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Publish events read from a channel
	send := make(chan Event)
	go func() {
		for {
			var event Event
			select {
			case <-ctx.Done():
				return
			case event = <-send:
			}

			b, err := event.MarshalBinary()
			if err != nil {
				logger.Error("Failed to marshal event", "error", err)
				continue
			}

			err = topic.Publish(ctx, b)
			if err != nil {
				logger.Error("Failed to send topic message", "error", err)
				return
			}
		}
	}()

	return send, recv, nil
}
