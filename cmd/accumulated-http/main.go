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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "accumulate-http <network>",
	Short: "Accumulate HTTP API node",
	Run:   run,
	Args:  cobra.ExactArgs(1),
}

var flag = struct {
	Key        string
	LogLevel   string
	HttpListen []multiaddr.Multiaddr
	P2pListen  []multiaddr.Multiaddr
	Peers      []multiaddr.Multiaddr
	Timeout    time.Duration
	ConnLimit  int
}{}

func init() {
	cmd.Flags().StringVar(&flag.Key, "key", "", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.HttpListen), "http-listen", "l", "HTTP listening address(es) (default /ip4/0.0.0.0/tcp/8080)")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.P2pListen), "p2p-listen", "P2P listening address(es)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")
	cmd.Flags().DurationVar(&flag.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmd.Flags().IntVar(&flag.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")

	_ = cmd.MarkFlagRequired("peer")
}

func run(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel()
	}()

	if len(flag.HttpListen) == 0 {
		// Default listen address
		a, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8080")
		Check(err)
		flag.HttpListen = append(flag.HttpListen, a)
	}
	if len(flag.Peers) == 0 {
		Fatalf("must specify at least one peer")
	}

	lw, err := logging.NewConsoleWriter("plain")
	Check(err)
	ll, lw, err := logging.ParseLogLevel(flag.LogLevel, lw)
	Check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(lw), ll, false)
	Check(err)

	node, err := p2p.New(p2p.Options{
		Key:            loadOrGenerateKey(),
		Network:        args[0],
		Logger:         logger,
		Listen:         flag.P2pListen,
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
	router.Router, err = routing.NewStaticRouter(ns.Routing, nil, logger)
	Check(err)

	api, err := nodehttp.NewHandler(nodehttp.Options{
		Logger:  logger,
		Node:    node,
		Router:  router.Router,
		MaxWait: 10 * time.Second,
		Network: &config.Describe{
			Network: config.Network{
				Id: args[0],
			},
		},
	})
	Check(err)

	server := &http.Server{Handler: api, ReadHeaderTimeout: flag.Timeout}

	wg := new(sync.WaitGroup)
	wg.Add(len(flag.HttpListen))
	for _, l := range flag.HttpListen {
		var proto, addr, port string
		multiaddr.ForEach(l, func(c multiaddr.Component) bool {
			switch c.Protocol().Code {
			case multiaddr.P_IP4,
				multiaddr.P_IP6,
				multiaddr.P_DNS,
				multiaddr.P_DNS4,
				multiaddr.P_DNS6:
				addr = c.Value()
			case multiaddr.P_TCP,
				multiaddr.P_UDP:
				proto = c.Protocol().Name
				port = c.Value()
			default:
				Fatalf("invalid listen address: %v", l)
			}
			return true
		})
		if proto == "" || port == "" {
			Fatalf("invalid listen address: %v", l)
		}
		addr += ":" + port

		l, err := net.Listen(proto, addr)
		Check(err)
		fmt.Printf("Listening on %v\n", l.Addr())

		if flag.ConnLimit > 0 {
			pool := make(chan struct{}, flag.ConnLimit)
			for i := 0; i < flag.ConnLimit; i++ {
				pool <- struct{}{}
			}
			l = &accumulated.RateLimitedListener{Listener: l, Pool: pool}
		}

		go func() {
			defer wg.Done()
			err := server.Serve(l)
			logger.Info("Server stopped", "err", err, "address", "")
		}()
	}

	go func() { wg.Wait(); cancel() }()

	// Wait for SIGINT or all the listeners to stop
	<-ctx.Done()
}

func loadOrGenerateKey() ed25519.PrivateKey {
	if strings.HasPrefix(flag.Key, "seed:") {
		Warnf("Generating a new key from a seed. This is not at all secure.")
		h := sha256.Sum256([]byte(flag.Key))
		return ed25519.NewKeyFromSeed(h[:])
	}

	if flag.Key != "" {
		return LoadKey(flag.Key)
	}

	// Generate a key if necessary
	Warnf("Generating a new key. This is highly discouraged for permanent infrastructure.")
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	Checkf(err, "generate key")
	return sk
}
