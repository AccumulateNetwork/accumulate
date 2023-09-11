// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

var cmdDbServe = &cobra.Command{
	Use:   "serve [database...]",
	Short: "Collect a snapshot",
	Args:  cobra.MinimumNArgs(1),
	Run:   serveDatabases,
}

var flagDbServe = struct {
	LogLevel   string
	HttpListen []multiaddr.Multiaddr
	Timeout    time.Duration
	ConnLimit  int
}{}

func init() {
	cmdDb.AddCommand(cmdDbServe)

	cmdDbServe.Flags().VarP((*MultiaddrSliceFlag)(&flagDbServe.HttpListen), "http-listen", "l", "HTTP listening address(es) (default /ip4/0.0.0.0/tcp/8080/http)")
	cmdDbServe.Flags().StringVar(&flagDbServe.LogLevel, "log-level", "error", "Log level")
	cmdDbServe.Flags().DurationVar(&flagDbServe.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmdDbServe.Flags().IntVar(&flagDbServe.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")
	cmdDbServe.Flags().BoolVar(&jsonrpc2.DebugMethodFunc, "debug", false, "Print out a stack trace if an API method fails")
}

func serveDatabases(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		cancel()
	}()

	if len(flagDbServe.HttpListen) == 0 {
		// Default listen address
		a, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8080/http")
		Check(err)
		flagDbServe.HttpListen = append(flagDbServe.HttpListen, a)
	}

	logger := NewConsoleLogger(flagDbServe.LogLevel)

	databases := map[string]*coredb.Database{}
	for _, arg := range args {
		// Open database
		db, err := coredb.OpenBadger(arg, logger)
		Check(err)
		defer db.Close()

		// Scan for the partition account
		var thePart string
		batch := db.Begin(false)
		defer batch.Discard()
		Check(batch.ForEachAccount(func(account *coredb.Account, _ [32]byte) error {
			if !account.Url().IsRootIdentity() {
				return nil
			}

			part, ok := protocol.ParsePartitionUrl(account.Url())
			if !ok {
				return nil
			}
			fmt.Printf("Found %v in %s\n", account.Url(), arg)

			if thePart == "" {
				thePart = part
				return nil
			}

			Fatalf("%s has multiple partition accounts", arg)
			panic("not reached")
		}))

		// Record the database and partition
		thePart = strings.ToLower(thePart)
		if _, ok := databases[thePart]; ok {
			Fatalf("%s has multiple databases", thePart)
		}
		databases[thePart] = db
	}

	// Load network globals from DN
	g := new(network.GlobalValues)
	if db, ok := databases[strings.ToLower(protocol.Directory)]; !ok {
		Fatalf("missing DN database")
	} else {
		batch := db.Begin(false)
		defer batch.Discard()
		Check(g.Load(protocol.DnUrl(), func(u *url.URL, target interface{}) error {
			return batch.Account(u).Main().GetAs(target)
		}))
	}
	router, err := routing.NewStaticRouter(g.Routing, logger)
	Check(err)

	// Make a querier for each partition
	Q := &Querier{
		router: router,
		parts:  map[string]*api.Querier{},
	}
	for _, part := range g.Network.Partitions {
		db, ok := databases[strings.ToLower(part.ID)]
		if !ok {
			Fatalf("missing database for %s", part.ID)
		}

		Q.parts[part.ID] = api.NewQuerier(api.QuerierParams{
			Logger:    logger,
			Database:  db,
			Partition: part.ID,
		})
	}

	// JSON-RPC API v3
	N := (*NetworkService)(g)
	C := &v3.Collator{Querier: Q, Network: N}
	v3, err := jsonrpc.NewHandler(
		jsonrpc.NetworkService{NetworkService: N},
		jsonrpc.Querier{Querier: C},
	)
	Check(err)

	v2, err := v2.NewJrpc(v2.Options{
		Logger:  logger,
		Querier: C,
	})
	Check(err)

	// Set up mux
	mux := v2.NewMux()
	mux.Handle("/v3", v3)

	// Serve
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: flagDbServe.Timeout,
	}

	wg := new(sync.WaitGroup)
	for _, l := range flagDbServe.HttpListen {
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
			case multiaddr.P_HTTP:
				// Ok
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
		serve(server, l, wg, logger)
	}

	go func() { wg.Wait(); cancel() }()

	// Wait for SIGINT or all the listeners to stop
	<-ctx.Done()
}

type NetworkService network.GlobalValues

func (n *NetworkService) NetworkStatus(ctx context.Context, opts v3.NetworkStatusOptions) (*v3.NetworkStatus, error) {
	return &v3.NetworkStatus{
		Oracle:          n.Oracle,
		Globals:         n.Globals,
		Network:         n.Network,
		Routing:         n.Routing,
		ExecutorVersion: protocol.ExecutorVersionV2,
	}, nil
}

type Querier struct {
	router routing.Router
	parts  map[string]*api.Querier
}

func (q *Querier) Query(ctx context.Context, scope *url.URL, query v3.Query) (v3.Record, error) {
	part, err := q.router.RouteAccount(scope)
	if err != nil {
		return nil, err
	}

	return q.parts[part].Query(ctx, scope, query)
}

func serve(server *http.Server, l net.Listener, wg *sync.WaitGroup, logger log.Logger) {
	wg.Add(1)
	fmt.Printf("Listening on %v\n", l.Addr())

	if flagDbServe.ConnLimit > 0 {
		pool := make(chan struct{}, flagDbServe.ConnLimit)
		for i := 0; i < flagDbServe.ConnLimit; i++ {
			pool <- struct{}{}
		}
		l = &accumulated.RateLimitedListener{Listener: l, Pool: pool}
	}

	go func() {
		defer wg.Done()
		err := server.Serve(l)
		slog.Error("Server stopped", "error", err, "address", l.Addr())
	}()
}
