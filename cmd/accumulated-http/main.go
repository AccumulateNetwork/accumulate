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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/rs/cors"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	Key          string
	LogLevel     string
	HttpListen   []multiaddr.Multiaddr
	P2pListen    []multiaddr.Multiaddr
	Peers        []multiaddr.Multiaddr
	Timeout      time.Duration
	ConnLimit    int
	CorsOrigins  []string
	LetsEncrypt  []string
	TlsCert      string
	TlsKey       string
	PeerDatabase string
}{
	Peers: accumulate.BootstrapServers,
}

func init() {
	cu, _ := user.Current()
	if cu != nil {
		flag.PeerDatabase = filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json")
	}

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
	cmd.Flags().StringVar(&flag.PeerDatabase, "peer-db", flag.PeerDatabase, "Track peers using a persistent database.")
	cmd.Flags().BoolVar(&jsonrpc2.DebugMethodFunc, "debug", false, "Print out a stack trace if an API method fails")
}

var nodeMainnetApollo = peer.AddrInfo{
	ID:    mustParsePeer("12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj"),
	Addrs: []multiaddr.Multiaddr{mustParseMulti("/dns/apollo-mainnet.accumulate.defidevs.io")},
}
var nodeMainnetYutu = peer.AddrInfo{
	ID:    mustParsePeer("12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7"),
	Addrs: []multiaddr.Multiaddr{mustParseMulti("/dns/yutu-mainnet.accumulate.defidevs.io")},
}
var nodeMainnetChandrayaan = peer.AddrInfo{
	ID:    mustParsePeer("12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL"),
	Addrs: []multiaddr.Multiaddr{mustParseMulti("/dns/chandrayaan-mainnet.accumulate.defidevs.io")},
}

func run(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = cmdutil.ContextForMainProcess(ctx)

	if len(flag.HttpListen) == 0 && len(flag.LetsEncrypt) == 0 {
		// Default listen address
		a, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8080/http")
		Check(err)
		flag.HttpListen = append(flag.HttpListen, a)
	}
	if len(flag.Peers) == 0 {
		Fatalf("must specify at least one peer")
	}

	logger := NewConsoleLogger(flag.LogLevel)

	node, err := p2p.New(p2p.Options{
		Key:               loadOrGenerateKey(),
		Network:           args[0],
		Listen:            flag.P2pListen,
		BootstrapPeers:    flag.Peers,
		PeerDatabase:      flag.PeerDatabase,
		EnablePeerTracker: true,
	})
	Check(err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	router, err := apiutil.InitRouter(ctx, node, args[0])
	Check(err)

	apiOpts := nodehttp.Options{
		Logger:    logger,
		Node:      node,
		Router:    router,
		MaxWait:   10 * time.Second,
		NetworkId: args[0],
	}
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: args[0],
		Router:  routing.MessageRouter{Router: router},
		Dialer:  node.DialNetwork(),
	}}
	timestamps := &TimestampService{
		querier: &api.Collator{Querier: client, Network: client},
		cache:   memory.New(nil),
	}

	if strings.EqualFold(args[0], "MainNet") {
		// Hard code the peers used for the MainNet as a hack for stability
		apiOpts.PeerMap = map[string][]peer.AddrInfo{
			"apollo":      {nodeMainnetApollo},
			"yutu":        {nodeMainnetYutu},
			"chandrayaan": {nodeMainnetChandrayaan},
			"directory":   {nodeMainnetApollo, nodeMainnetYutu, nodeMainnetChandrayaan},
		}
	}

	api, err := nodehttp.NewHandler(apiOpts)
	Check(err)

	api2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/timestamp/"
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, prefix) {
			api.ServeHTTP(w, r)
			return
		}

		var res any
		id, err := url.ParseTxID(r.URL.Path[len(prefix):])
		if err == nil {
			res, err = timestamps.GetTimestamp(r.Context(), id)
		}

		if err == nil {
			err := json.NewEncoder(w).Encode(res)
			if err != nil {
				slog.ErrorCtx(r.Context(), "Failed to encode response", "error", err)
			}
			return
		}

		err2 := errors.UnknownError.Wrap(err).(*errors.Error)
		w.WriteHeader(int(err2.Code))

		err = json.NewEncoder(w).Encode(err2)
		if err != nil {
			slog.ErrorCtx(r.Context(), "Failed to encode response", "error", err)
		}
	})

	c := cors.New(cors.Options{
		AllowedOrigins: flag.CorsOrigins,
	})
	server := &http.Server{
		Handler:           c.Handler(api2),
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

func mustParsePeer(s string) peer.ID {
	id, err := peer.Decode(s)
	if err != nil {
		panic(err)
	}
	return id
}

func mustParseMulti(s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}
