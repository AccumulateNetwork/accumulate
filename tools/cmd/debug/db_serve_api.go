// Copyright 2024 The Accumulate Authors
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
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/julienschmidt/httprouter"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

var cmdDbServeApi = &cobra.Command{
	Use:  "serve-api [database...]",
	Args: cobra.MinimumNArgs(1),
	Run:  serveApiFromDatabases,
}

var flagDbServe = struct {
	LogLevel   string
	HttpListen []multiaddr.Multiaddr
	Timeout    time.Duration
	ConnLimit  int
}{}

func init() {
	cmdDb.AddCommand(cmdDbServeApi)

	cmdDbServeApi.Flags().VarP((*MultiaddrSliceFlag)(&flagDbServe.HttpListen), "http-listen", "l", "HTTP listening address(es) (default /ip4/0.0.0.0/tcp/8080/http)")
	cmdDbServeApi.Flags().StringVar(&flagDbServe.LogLevel, "log-level", "error", "Log level")
	cmdDbServeApi.Flags().DurationVar(&flagDbServe.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmdDbServeApi.Flags().IntVar(&flagDbServe.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")
	cmdDbServeApi.Flags().BoolVar(&jsonrpc2.DebugMethodFunc, "debug", false, "Print out a stack trace if an API method fails")
}

func serveApiFromDatabases(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = cmdutil.ContextForMainProcess(ctx)

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
		thePart := getPartition(db, arg)

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
	router := routing.NewRouter(routing.RouterOptions{Initial: g.Routing, Logger: logger})

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

	// JSON-RPC API v2
	//
	// Use query isolation to avoid weird bugs
	v2, err := v2.NewJrpc(v2.Options{
		Logger:  logger,
		Querier: queryIsolation{q: C},
	})
	Check(err)

	// Set up mux
	mux := httprouter.New()
	Check(v2.Register(mux))
	mux.POST("/v3", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		v3.ServeHTTP(w, r)
	})

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

type queryIsolation struct {
	q v3.Querier
}

func (q queryIsolation) Query(ctx context.Context, scope *url.URL, query v3.Query) (v3.Record, error) {
	r, err := q.q.Query(ctx, scope, query)
	if r != nil {
		b, err := r.MarshalBinary()
		if err == nil {
			s, err := v3.UnmarshalRecord(b)
			if err == nil {
				r = s
			}
		}
	}
	return r, err
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
