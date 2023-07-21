// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package faucet

import (
	"context"
	"crypto/ed25519"

	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/libs/log"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Options struct {
	Key     ed25519.PrivateKey
	Network string
	Peers   []multiaddr.Multiaddr
	Listen  []multiaddr.Multiaddr
	Logger  log.Logger
}

func StartLite(ctx context.Context, opts Options) (*p2p.Node, error) {
	faucet, err := protocol.LiteTokenAddress(opts.Key[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		return nil, err
	}

	// Create the faucet node
	node, err := p2p.New(p2p.Options{
		Network:        opts.Network,
		Key:            opts.Key,
		BootstrapPeers: opts.Peers,
		Listen:         opts.Listen,
	})
	if err != nil {
		return nil, err
	}

	client, err := p2p.NewClientWith(node)
	if err != nil {
		return nil, err
	}

	// Create the faucet service
	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    opts.Logger.With("module", "faucet"),
		Account:   faucet,
		Key:       build.ED25519PrivateKey(opts.Key),
		Submitter: client,
		Querier:   client,
		Events:    client,
	})
	if err != nil {
		return nil, err
	}

	// Cleanup
	go func() {
		<-ctx.Done()
		faucetSvc.Stop()
		err := client.Close()
		if err != nil {
			opts.Logger.Error("Error shutting p2p node for faucet", "module", "faucet", "error", err)
		}
	}()

	// Register it
	handler, err := message.NewHandler(message.Faucet{Faucet: faucetSvc})
	if err != nil {
		return nil, err
	}
	if !client.RegisterService(faucetSvc.Type().AddressForUrl(protocol.AcmeUrl()), handler.Handle) {
		panic("unable to register faucet service")
	}
	return node, nil
}
