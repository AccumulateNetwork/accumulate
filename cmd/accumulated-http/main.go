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

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/multiformats/go-multiaddr"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/exp/slog"
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
	Key         string
	LogLevel    string
	HttpListen  []multiaddr.Multiaddr
	P2pListen   []multiaddr.Multiaddr
	Peers       []multiaddr.Multiaddr
	Timeout     time.Duration
	ConnLimit   int
	CorsOrigins []string
	LetsEncrypt []string
	TlsCert     string
	TlsKey      string
}{
	Peers: api.BootstrapServers,
}

func init() {
	cmd.Flags().StringVar(&flag.Key, "key", "", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.HttpListen), "http-listen", "l", "HTTP listening address(es) (default /ip4/0.0.0.0/tcp/8080/http)")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.P2pListen), "p2p-listen", "P2P listening address(es)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")
	cmd.Flags().DurationVar(&flag.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmd.Flags().IntVar(&flag.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")
	cmd.Flags().StringSliceVar(&flag.CorsOrigins, "cors-origin", nil, "Allowed CORS origins")
	cmd.Flags().StringSliceVar(&flag.LetsEncrypt, "lets-encrypt", nil, "Enable HTTPS on 443 and use Let's Encrypt to retrieve a certificate. Use of this feature implies acceptance of the LetsEncrypt Terms of Service.")
	cmd.Flags().StringVar(&flag.TlsCert, "tls-cert", "", "Certificate used for HTTPS")
	cmd.Flags().StringVar(&flag.TlsKey, "tls-key", "", "Private key used for HTTPS")
	cmd.Flags().BoolVar(&jsonrpc2.DebugMethodFunc, "debug", false, "Print out a stack trace if an API method fails")
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

	if len(flag.HttpListen) == 0 && len(flag.LetsEncrypt) == 0 {
		// Default listen address
		a, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8080/http")
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
		Key:               loadOrGenerateKey(),
		Network:           args[0],
		Listen:            flag.P2pListen,
		BootstrapPeers:    flag.Peers,
		EnablePeerTracker: true,
	})
	Check(err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	router := new(routing.MessageRouter)
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: args[0],
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}

	if strings.EqualFold(args[0], "mainnet") {
		// Hard code this for now
		fmt.Println("Using hard-coded routing table for MainNet")
		rt := &protocol.RoutingTable{
			Overrides: []protocol.RouteOverride{
				{
					Account:   url.MustParse("acc://staking.acme"),
					Partition: "Directory",
				},
				{
					Account:   url.MustParse("acc://a642a8674f46696cc47fdb6b65f9c87b2a19c5ea8123b3d2f0c13b6f33a9d5ef"),
					Partition: "Chandrayaan",
				},
				{
					Account:   url.MustParse("acc://ACME"),
					Partition: "Directory",
				},
				{
					Account:   url.MustParse("acc://bvn-Apollo.acme"),
					Partition: "Apollo",
				},
				{
					Account:   url.MustParse("acc://bvn-Chandrayaan.acme"),
					Partition: "Chandrayaan",
				},
				{
					Account:   url.MustParse("acc://bvn-Yutu.acme"),
					Partition: "Yutu",
				},
				{
					Account:   url.MustParse("acc://dn.acme"),
					Partition: "Directory",
				},
			},
			Routes: []protocol.Route{
				{
					Length:    2,
					Partition: "Apollo",
				},
				{
					Length:    2,
					Value:     1,
					Partition: "Yutu",
				},
				{
					Length:    2,
					Value:     2,
					Partition: "Chandrayaan",
				},
				{
					Length:    3,
					Value:     6,
					Partition: "Apollo",
				},
				{
					Length:    4,
					Value:     14,
					Partition: "Yutu",
				},
				{
					Length:    4,
					Value:     15,
					Partition: "Chandrayaan",
				},
			},
		}

		router.Router, err = routing.NewStaticRouter(rt, logger)
		Check(err)

	} else {
		fmt.Println("Waiting for a live network service")
		svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(args[0])
		Check(err)
		Check(node.WaitForService(ctx, svcAddr))

		fmt.Println("Fetching routing information")
		ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		Check(err)

		router.Router, err = routing.NewStaticRouter(ns.Routing, logger)
		Check(err)
	}

	api, err := nodehttp.NewHandler(nodehttp.Options{
		Logger:    logger,
		Node:      node,
		Router:    router.Router,
		MaxWait:   10 * time.Second,
		NetworkId: args[0],
	})
	Check(err)

	c := cors.New(cors.Options{
		AllowedOrigins: flag.CorsOrigins,
	})
	server := &http.Server{
		Handler:           c.Handler(api),
		ReadHeaderTimeout: flag.Timeout,
	}

	wg := new(sync.WaitGroup)
	for _, l := range flag.HttpListen {
		var proto, addr, port string
		var secure bool
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
			case multiaddr.P_HTTP:
				// Ok
			case multiaddr.P_HTTPS:
				secure = true
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
		serve(server, l, secure, wg, logger)
	}

	if len(flag.LetsEncrypt) > 0 {
		serve(server, autocert.NewListener(flag.LetsEncrypt...), false, wg, logger)
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

func serve(server *http.Server, l net.Listener, secure bool, wg *sync.WaitGroup, logger log.Logger) {
	if secure && (flag.TlsCert == "" || flag.TlsKey == "") {
		Fatalf("--tls-cert and --tls-key are required to listen on %v", l)
	}

	wg.Add(1)
	scheme := "http"
	if secure {
		scheme = "https"
	}
	fmt.Printf("Listening on %v (%v)\n", l.Addr(), scheme)

	if flag.ConnLimit > 0 {
		pool := make(chan struct{}, flag.ConnLimit)
		for i := 0; i < flag.ConnLimit; i++ {
			pool <- struct{}{}
		}
		l = &accumulated.RateLimitedListener{Listener: l, Pool: pool}
	}

	go func() {
		defer wg.Done()
		var err error
		if secure {
			err = server.ServeTLS(l, flag.TlsCert, flag.TlsKey)
		} else {
			err = server.Serve(l)
		}
		slog.Error("Server stopped", "error", err, "address", l.Addr())
	}()
}
