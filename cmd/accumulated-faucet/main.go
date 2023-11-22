// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "accumulate-faucet <network>",
	Short: "Accumulate HTTP API node",
	Run:   run,
	Args:  cobra.ExactArgs(1),
}

var flag = struct {
	NodeKey  string
	Key      string
	LogLevel string
	Account  *url.URL
	Listen   []multiaddr.Multiaddr
	Peers    []multiaddr.Multiaddr
}{}

func init() {
	cmd.Flags().StringVar(&flag.NodeKey, "node-key", "", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().StringVar(&flag.Key, "key", "", "The key used to sign faucet transactions")
	cmd.Flags().Var(UrlFlag{V: &flag.Account}, "account", "The faucet account")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.Listen), "listen", "P2P listening address(es)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")

	_ = cmd.MarkFlagRequired("peer")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("key")
}

func run(_ *cobra.Command, args []string) {
	ctx := cmdutil.ContextForMainProcess(context.Background())

	lw, err := logging.NewConsoleWriter("plain")
	Check(err)
	ll, lw, err := logging.ParseLogLevel(flag.LogLevel, lw)
	Check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(lw), ll, false)
	Check(err)

	node, err := p2p.New(p2p.Options{
		Key:            loadOrGenerateKey(),
		Network:        args[0],
		Listen:         flag.Listen,
		BootstrapPeers: flag.Peers,
	})
	Check(err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	fmt.Println("Waiting for a live network service")
	svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(args[0])
	Check(err)
	Check(node.WaitForService(ctx, svcAddr))

	fmt.Println("Fetching routing information")
	router := new(routing.MessageRouter)
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: args[0],
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	Check(err)
	router.Router = routing.NewRouter(routing.RouterOptions{Initial: ns.Routing, Logger: logger})

	faucetSvc, err := v3impl.NewFaucet(context.Background(), v3impl.FaucetParams{
		Logger:    logger.With("module", "faucet"),
		Account:   flag.Account,
		Key:       build.ED25519PrivateKey(LoadKey(flag.Key)),
		Submitter: client,
		Querier:   client,
		Events:    client,
	})
	Check(err)
	defer faucetSvc.Stop()

	handler, err := message.NewHandler(message.Faucet{Faucet: faucetSvc})
	Check(err)
	if !node.RegisterService(faucetSvc.Type().AddressForUrl(protocol.AcmeUrl()), handler.Handle) {
		Fatalf("failed to register faucet service")
	}

	// Wait for SIGINT
	fmt.Println("Running")
	<-ctx.Done()
}

func loadOrGenerateKey() ed25519.PrivateKey {
	if strings.HasPrefix(flag.NodeKey, "seed:") {
		Warnf("Generating a new key from a seed. This is not at all secure.")
		h := sha256.Sum256([]byte(flag.NodeKey))
		return ed25519.NewKeyFromSeed(h[:])
	}

	if flag.NodeKey != "" {
		return LoadKey(flag.NodeKey)
	}

	// Generate a key if necessary
	Warnf("Generating a new key. This is highly discouraged for permanent infrastructure.")
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	Checkf(err, "generate key")
	return sk
}
