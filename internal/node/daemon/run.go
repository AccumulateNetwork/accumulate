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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/fatih/color"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/crypto"
	tmlog "github.com/tendermint/tendermint/libs/log"
	service2 "github.com/tendermint/tendermint/libs/service"
	tmnode "github.com/tendermint/tendermint/node"
	tmp2p "github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/connections"
	statuschk "gitlab.com/accumulatenetwork/accumulate/internal/node/connections/status"
	nodeapi "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	done              chan struct{}
	db                *database.Database
	node              *node.Node
	apiServer         *http.Server
	privVal           *privval.FilePV
	apiv2             *v2.JrpcMethods
	p2pnode           *p2p.Node
	api               *nodeapi.Handler
	nodeKey           *tmp2p.NodeKey
	connectionManager connections.ConnectionInitializer
	eventBus          *events.Bus
	localTm           tmclient.Client
	snapshotSchedule  cron.Schedule
	snapshotLock      *sync.Mutex
	tracer            trace.Tracer

	// knobs for tests
	// IsTest   bool
	UseMemDB bool
}

func Load(dir string, newWriter func(*config.Config) (io.Writer, error)) (*Daemon, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}

	return New(cfg, newWriter)
}

func New(cfg *config.Config, newWriter func(*config.Config) (io.Writer, error)) (*Daemon, error) {
	var daemon Daemon
	daemon.snapshotLock = new(sync.Mutex)
	daemon.Config = cfg

	if newWriter == nil {
		newWriter = func(c *config.Config) (io.Writer, error) {
			return logging.NewConsoleWriter(c.LogFormat)
		}
	}

	logWriter, err := newWriter(daemon.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize log writer: %v", err)
	}

	logLevel, logWriter, err := logging.ParseLogLevel(daemon.Config.LogLevel, logWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %v", err)
	}

	daemon.Logger, err = logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %v", err)
	}

	return &daemon, nil
}

func (d *Daemon) Key() crypto.PrivKey {
	return d.privVal.Key.PrivKey
}

func (d *Daemon) DB_TESTONLY() *database.Database { return d.db }
func (d *Daemon) Node_TESTONLY() *node.Node       { return d.node }
func (d *Daemon) Jrpc_TESTONLY() *v2.JrpcMethods  { return d.apiv2 }
func (d *Daemon) API() *nodeapi.Handler           { return d.api }

func (d *Daemon) Start() (err error) {
	if d.done != nil {
		return fmt.Errorf("already started")
	}
	d.done = make(chan struct{})

	defer func() {
		if err != nil {
			close(d.done)
		}
	}()

	if d.Config.Accumulate.AnalysisLog.Enabled {
		dir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.AnalysisLog.Directory)
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return err
		}

		ymd, hm := logging.GetCurrentDateTime()
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("trace_%v_%v.json", ymd, hm)))
		if err != nil {
			return err
		}

		exp, err := stdouttrace.New(stdouttrace.WithWriter(f))
		if err != nil {
			f.Close()
			return err
		}

		r, err := resource.Merge(
			resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("accumulate"),
				semconv.ServiceVersionKey.String(accumulate.Version),
			),
		)
		if err != nil {
			f.Close()
			return err
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(r),
		)
		go func() {
			<-d.done
			_ = tp.Shutdown(context.Background())
			f.Close()
		}()
		// otel.SetTracerProvider(tp)
		d.tracer = tp.Tracer("Accumulate")
	}

	if s, err := core.Cron.Parse(d.Config.Accumulate.Snapshots.Schedule); err != nil {
		d.Logger.Error("Ignoring invalid snapshot schedule", "error", err, "value", d.Config.Accumulate.Snapshots.Schedule)
	} else {
		d.snapshotSchedule = s
	}

	d.db, err = database.Open(d.Config, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Close the database if start fails (mostly for tests)
	defer func() {
		if err != nil {
			_ = d.db.Close()
		}
	}()

	// read private validator
	d.privVal, err = config.LoadFilePV(
		d.Config.PrivValidatorKeyFile(),
		d.Config.PrivValidatorStateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	d.nodeKey, err = tmp2p.LoadNodeKey(d.Config.NodeKeyFile())
	if err != nil {
		return fmt.Errorf("failed to load node key: %v", err)
	}

	d.connectionManager = connections.NewConnectionManager(d.Config, d.Logger, func(server string) (connections.APIClient, error) {
		return client.New(server)
	})

	d.eventBus = events.NewBus(d.Logger.With("module", "events"))
	events.SubscribeSync(d.eventBus, d.onDidCommitBlock)

	router := routing.NewRouter(d.eventBus, d.connectionManager, d.Logger)
	execOpts := block.ExecutorOptions{
		Logger:           d.Logger,
		Key:              d.Key().Bytes(),
		Describe:         d.Config.Accumulate.Describe,
		Router:           router,
		EventBus:         d.eventBus,
		BatchReplayLimit: d.Config.Accumulate.BatchReplayLimit,
	}

	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	exec, err := block.NewNodeExecutor(execOpts, d.db)
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		DB:       d.db,
		Address:  d.Key().PubKey().Address(),
		Executor: (*execute.ExecutorV1)(exec),
		Logger:   d.Logger,
		EventBus: d.eventBus,
		Config:   d.Config,
		Tracer:   d.tracer,
	})

	// Create node
	tmn, err := tmnode.NewNode(
		&d.Config.Config,
		d.privVal,
		d.nodeKey,
		proxy.NewLocalClientCreator(app),
		tmnode.DefaultGenesisDocProviderFunc(&d.Config.Config),
		tmnode.DefaultDBProvider,
		tmnode.DefaultMetricsProvider(d.Config.Instrumentation),
		d.Logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %v", err)
	}
	d.node = &node.Node{Node: tmn, Config: d.Config, ABCI: app}

	// Start node
	// TODO Feed Tendermint logger to service logger
	err = d.node.Start()
	if err != nil {
		return fmt.Errorf("failed to start node: %v", err)
	}

	// Stop the node if start fails (mostly for tests)
	defer func() {
		if err != nil {
			_ = d.node.Stop()
			<-d.node.Quit()
		}
	}()

	events.SubscribeAsync(d.eventBus, func(e events.FatalError) {
		d.Logger.Error("Shutting down due to a fatal error", "error", e.Err)
		err := d.Stop()
		if errors.Is(err, service2.ErrAlreadyStopped) {
			return
		}
		if err != nil {
			d.Logger.Error("Error while shutting down", "error", err)
		}
	})

	// Create a local client
	d.localTm = local.New(d.node.Node)

	if d.Config.Accumulate.API.DebugJSONRPC {
		jsonrpc2.DebugMethodFunc = true
	}

	// Create the JSON-RPC handler
	d.apiv2, err = v2.NewJrpc(v2.Options{
		Logger:            d.Logger,
		Describe:          &d.Config.Accumulate.Describe,
		Router:            router,
		PrometheusServer:  d.Config.Accumulate.API.PrometheusServer,
		TxMaxWaitTime:     d.Config.Accumulate.API.TxMaxWaitTime,
		Database:          d.db,
		ConnectionManager: d.connectionManager,
		Key:               d.Key().Bytes(),
	})
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	// Let the connection manager create and assign clients
	statusChecker := statuschk.NewNodeStatusChecker()
	err = d.connectionManager.InitClients(d.localTm, statusChecker)

	if err != nil {
		return fmt.Errorf("failed to initialize the connection manager: %v", err)
	}

	d.p2pnode, err = p2p.New(p2p.Options{
		Logger:         d.Logger.With("module", "acc-rpc"),
		Listen:         d.Config.Accumulate.P2P.Listen,
		BootstrapPeers: app.Accumulate.P2P.BootstrapPeers,
		Key:            ed25519.PrivateKey(d.nodeKey.PrivKey.Bytes()),
		Partitions: []p2p.PartitionOptions{
			{
				Moniker: d.Config.Moniker,
				ID:      d.Config.Accumulate.PartitionId,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("initialize P2P: %w", err)
	}

	nodeSvc := tm.NewNodeService(tm.NodeServiceParams{
		Logger:           d.Logger.With("module", "acc-rpc"),
		Local:            d.localTm,
		Database:         d.db,
		PartitionID:      d.Config.Accumulate.PartitionId,
		PartitionType:    d.Config.Accumulate.NetworkType,
		EventBus:         d.eventBus,
		NodeKeyHash:      sha256.Sum256(d.nodeKey.PubKey().Bytes()),
		ValidatorKeyHash: sha256.Sum256(d.privVal.Key.PubKey.Bytes()),
	})
	netSvc := api.NewNetworkService(api.NetworkServiceParams{
		Logger:   d.Logger.With("module", "acc-rpc"),
		EventBus: d.eventBus,
		Globals:  exec.ActiveGlobals_TESTONLY(),
	})
	querySvc := api.NewQuerier(api.QuerierParams{
		Logger:    d.Logger.With("module", "acc-rpc"),
		Database:  d.db,
		Partition: d.Config.Accumulate.PartitionId,
	})
	metricsSvc := api.NewMetricsService(api.MetricsServiceParams{
		Logger:  d.Logger.With("module", "acc-rpc"),
		Node:    nodeSvc,
		Querier: querySvc,
	})
	submitSvc := tm.NewSubmitter(tm.SubmitterParams{
		Logger: d.Logger.With("module", "acc-rpc"),
		Local:  d.localTm,
	})
	validateSvc := tm.NewValidator(tm.ValidatorParams{
		Logger: d.Logger.With("module", "acc-rpc"),
		Local:  d.localTm,
	})
	eventSvc := api.NewEventService(api.EventServiceParams{
		Logger:    d.Logger.With("module", "acc-rpc"),
		Database:  d.db,
		Partition: d.Config.Accumulate.PartitionId,
		EventBus:  d.eventBus,
	})
	p2ph, err := message.NewHandler(
		d.Logger.With("module", "acc-rpc"),
		&message.NodeService{NodeService: nodeSvc},
		&message.MetricsService{MetricsService: metricsSvc},
		&message.NetworkService{NetworkService: netSvc},
		&message.Querier{Querier: querySvc},
		&message.Submitter{Submitter: submitSvc},
		&message.Validator{Validator: validateSvc},
		&message.EventService{EventService: eventSvc},
	)
	if err != nil {
		return fmt.Errorf("initialize P2P handler: %w", err)
	}
	d.p2pnode.SetRpcHandler(d.Config.Accumulate.PartitionId, p2ph.Handle)

	d.api, err = nodeapi.NewHandler(nodeapi.Options{
		Logger:    d.Logger.With("module", "acc-rpc"),
		Node:      d.p2pnode,
		Partition: d.Config.Accumulate.PartitionId,
		Router:    router,
		V2:        d.apiv2,
	})
	if err != nil {
		return fmt.Errorf("initialize API: %w", err)
	}

	d.apiServer = &http.Server{Handler: d.api, ReadHeaderTimeout: d.Config.Accumulate.API.ReadHeaderTimeout}
	l, secure, err := listenHttpUrl(d.Config.Accumulate.API.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to start JSON-RPC: %v", err)
	}
	if secure {
		return fmt.Errorf("failed to start JSON-RPC: HTTPS is not supported")
	}

	if d.Config.Accumulate.API.ConnectionLimit > 0 {
		pool := make(chan struct{}, d.Config.Accumulate.API.ConnectionLimit)
		for i := 0; i < d.Config.Accumulate.API.ConnectionLimit; i++ {
			pool <- struct{}{}
		}
		l = &rateLimitedListener{Listener: l, Pool: pool}
	}

	go func() {
		err := d.apiServer.Serve(l)
		if err != nil {
			d.Logger.Error("JSON-RPC server", "err", err)
		}
	}()

	// Shut down the node if the disk space gets too low
	go d.ensureSufficientDiskSpace(d.Config.RootDir)
	for !d.node.IsRunning() {
		color.HiMagenta("Syncing ....")
		time.Sleep(time.Second * 1)
	}
	color.HiBlue(" %s node running at %s :", d.node.Config.Accumulate.NetworkType, d.node.Config.Accumulate.Describe.LocalAddress)

	// Clean up once the node is stopped (mostly for tests)
	go func() {
		defer close(d.done)

		d.node.Wait()

		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
		defer cancel()

		err := d.apiServer.Shutdown(ctx)
		if err != nil {
			d.Logger.Error("Error stopping API", "module", "jrpc", "error", err)
		}

		err = d.db.Close()
		if err != nil {
			module := "badger"
			if d.UseMemDB {
				module = "memdb"
			}
			d.Logger.Error("Error closing database", "module", module, "error", err)
		}
	}()

	return nil
}

func (d *Daemon) LocalClient() (connections.ABCIClient, error) {
	ctx, err := d.connectionManager.SelectConnection(d.Config.Accumulate.PartitionId, false)
	if err != nil {
		return nil, err
	}

	return ctx.GetABCIClient(), nil
}

func (d *Daemon) ConnectDirectly(e *Daemon) error {
	err := d.p2pnode.ConnectDirectly(e.p2pnode)
	if err != nil {
		return err
	}

	err = e.p2pnode.ConnectDirectly(d.p2pnode)
	if err != nil {
		return err
	}

	err = d.connectionManager.ConnectDirectly(e.connectionManager)
	if err != nil {
		return err
	}

	return e.connectionManager.ConnectDirectly(d.connectionManager)
}

func (d *Daemon) ensureSufficientDiskSpace(dbPath string) {
	defer func() { _ = d.node.Stop() }()

	logger := d.Logger.With("module", "disk-monitor")

	for {
		free, err := diskUsage(dbPath)
		if err != nil {
			logger.Error("Failed to get disk size, shutting down", "error", err)
			return
		}

		if free < 0.05 {
			logger.Error("Less than 5% disk space available, shutting down", "free", free)
			return
		}

		logger.Info("Disk usage", "free", free)

		time.Sleep(10 * time.Minute)
	}
}

// listenHttpUrl takes a string such as `http://localhost:123` and creates a TCP
// listener.
func listenHttpUrl(s string) (net.Listener, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false, fmt.Errorf("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, fmt.Errorf("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("invalid address: unsupported scheme %q", u.Scheme)
	}

	l, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, false, err
	}

	return l, secure, nil
}

func (d *Daemon) Stop() error {
	err := d.node.Stop()
	if err != nil {
		return err
	}

	<-d.done
	return nil
}

func (d *Daemon) Done() <-chan struct{} {
	return d.node.Quit()
}
