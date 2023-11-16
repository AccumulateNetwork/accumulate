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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/abci/types"
	tmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto"
	tmlog "github.com/cometbft/cometbft/libs/log"
	service2 "github.com/cometbft/cometbft/libs/service"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client/local"
	"github.com/fatih/color"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/exp/tendermint"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	nodeapi "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	done             chan struct{}
	db               *database.Database
	node             *node.Node
	apiServer        *http.Server
	privVal          *privval.FilePV
	p2pnode          *p2p.Node
	api              *nodeapi.Handler
	nodeKey          *tmp2p.NodeKey
	router           routing.Router
	eventBus         *events.Bus
	localTm          *tendermint.DeferredClient
	snapshotSchedule cron.Schedule
	snapshotLock     *sync.Mutex
	tracer           trace.Tracer
	local            map[string]tendermint.DispatcherClient

	// knobs for tests
	// IsTest   bool
	UseMemDB bool
}

func Load(dir string, newWriter func(*config.Config) (io.Writer, error)) (*Daemon, error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("reading config file: %v", err)
	}

	return New(cfg, newWriter)
}

func New(cfg *config.Config, newWriter func(*config.Config) (io.Writer, error)) (*Daemon, error) {
	var daemon Daemon
	daemon.snapshotLock = new(sync.Mutex)
	daemon.Config = cfg
	daemon.localTm = tendermint.NewDeferredClient()

	if newWriter == nil {
		newWriter = func(c *config.Config) (io.Writer, error) {
			return logging.NewConsoleWriter(c.LogFormat)
		}
	}

	logWriter, err := newWriter(daemon.Config)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize log writer: %v", err)
	}

	logLevel, logWriter, err := logging.ParseLogLevel(daemon.Config.LogLevel, logWriter)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid parse log level: %v", err)
	}

	daemon.Logger, err = logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize logger: %v", err)
	}

	daemon.eventBus = events.NewBus(daemon.Logger.With("module", "events"))
	return &daemon, nil
}

func (d *Daemon) Key() crypto.PrivKey {
	return d.privVal.Key.PrivKey
}

func (d *Daemon) DB_TESTONLY() *database.Database { return d.db }
func (d *Daemon) Node_TESTONLY() *node.Node       { return d.node }
func (d *Daemon) P2P_TESTONLY() *p2p.Node         { return d.p2pnode }
func (d *Daemon) API() *nodeapi.Handler           { return d.api }
func (d *Daemon) EventBus() *events.Bus           { return d.eventBus }

// StartSecondary starts this daemon as a secondary process of the given daemon
// (which must already be running).
func (d *Daemon) StartSecondary(e *Daemon, others ...*Daemon) error {
	// Reuse the P2P node. Otherwise, start everything normally.
	d.p2pnode = e.p2pnode
	return d.Start(append(others, e)...)
}

func (d *Daemon) Start(others ...*Daemon) (err error) {
	d.local = map[string]tendermint.DispatcherClient{}
	d.local[strings.ToLower(d.Config.Accumulate.PartitionId)] = d.localTm
	for _, e := range others {
		part := strings.ToLower(e.Config.Accumulate.PartitionId)
		if d.local[part] == nil {
			d.local[part] = e.localTm
		}
	}

	if d.Config.Accumulate.API.DebugJSONRPC {
		jsonrpc2.DebugMethodFunc = true
	}

	// Set up analysis
	if d.Config.Accumulate.AnalysisLog.Enabled {
		err = d.startAnalysis()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Set up shutdown notification
	if d.done != nil {
		return errors.BadRequest.With("already started")
	}
	d.done = make(chan struct{})
	defer func() {
		if err != nil {
			close(d.done)
		}
	}()

	// Parse the snapshot schedule
	if s, err := core.Cron.Parse(d.Config.Accumulate.Snapshots.Schedule); err != nil {
		d.Logger.Error("Ignoring invalid snapshot schedule", "error", err, "value", d.Config.Accumulate.Snapshots.Schedule)
	} else {
		d.snapshotSchedule = s
	}

	// Load keys
	err = d.loadKeys()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	switch d.Config.Accumulate.NetworkType {
	case protocol.PartitionTypeDirectory,
		protocol.PartitionTypeBlockValidator:
		err = d.startValidator()
	case protocol.PartitionTypeBlockSummary:
		err = d.startSummary()
	default:
		return errors.InternalError.WithFormat("unknown partition type %v", d.Config.Accumulate.NetworkType)
	}
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	d.startMonitoringAndCleanup()
	return nil
}

func (d *Daemon) startValidator() (err error) {
	// Start the database
	d.db, err = database.Open(d.Config, d.Logger)
	if err != nil {
		return errors.UnknownError.WithFormat("open database: %w", err)
	}
	defer func() {
		if err != nil {
			_ = d.db.Close()
		}
	}()

	// Setup the event bus
	events.SubscribeSync(d.eventBus, d.onDidCommitBlock)

	globals := make(chan *core.GlobalValues, 1)
	events.SubscribeSync(d.eventBus, func(e events.WillChangeGlobals) error {
		select {
		case globals <- e.New:
		default:
		}
		return nil
	})

	// Start the API
	err = d.startAPI()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start the block summary collector
	if d.Config.Accumulate.SummaryNetwork != "" {
		err = d.startCollector()
		if err != nil {
			return errors.UnknownError.WithFormat("start collector: %w", err)
		}
	}

	// Start the executor and ABCI
	app, err := d.startApp()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start Tendermint
	err = d.startConsensus(app)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start services
	err = d.startServices(globals)
	return errors.UnknownError.Wrap(err)
}

func (d *Daemon) startAnalysis() error {
	// Create the directory
	dir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.AnalysisLog.Directory)
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return errors.UnknownError.WithFormat("create analysis log directory: %w", err)
	}

	// Open the log file (tagged with date and time)
	ymd, hm := logging.GetCurrentDateTime()
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("trace_%v_%v.json", ymd, hm)))
	if err != nil {
		return errors.UnknownError.WithFormat("open analysis log file: %w", err)
	}
	go func() { <-d.done; _ = f.Close() }()

	// Define the service
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("accumulate"),
			semconv.ServiceVersionKey.String(accumulate.Version),
		),
	)
	if err != nil {
		return err
	}

	// Initialize the exporter
	exp, err := stdouttrace.New(stdouttrace.WithWriter(f))
	if err != nil {
		return err
	}

	// Initialize the tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	)
	go func() { <-d.done; _ = tp.Shutdown(context.Background()) }()

	// otel.SetTracerProvider(tp)
	d.tracer = tp.Tracer("Accumulate")
	return nil
}

func (d *Daemon) loadKeys() error {
	if d.privVal != nil {
		return nil
	}

	var err error
	d.privVal, err = config.LoadFilePV(
		d.Config.PrivValidatorKeyFile(),
		d.Config.PrivValidatorStateFile(),
	)
	if err != nil {
		return errors.UnknownError.WithFormat("load private validator key: %v", err)
	}

	d.nodeKey, err = tmp2p.LoadNodeKey(d.Config.NodeKeyFile())
	if err != nil {
		return errors.UnknownError.WithFormat("load node key: %v", err)
	}

	return nil
}

func (d *Daemon) startApp() (types.Application, error) {
	dialer := d.p2pnode.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: d.Config.Accumulate.Network.Id,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: d.router},
	}}
	execOpts := execute.Options{
		Logger:        d.Logger,
		Database:      d.db,
		Key:           d.Key().Bytes(),
		Describe:      d.Config.Accumulate.Describe,
		Router:        d.router,
		EventBus:      d.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: d.Config.Accumulate.Healing.Enable,
	}

	if _, ok := d.local["directory"]; !ok ||
		!d.Config.Accumulate.DisableDirectDispatch {
		// If we are not attached to a DN node, or direct dispatch is disabled,
		// use the API dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return newDispatcher(d.Config.Accumulate.Network.Id, d.router, dialer)
		}

	} else {
		// Otherwise, use the Tendermint dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return tendermint.NewDispatcher(d.router, d.local)
		}
	}

	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize chain executor: %v", err)
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		Address:  d.Key().PubKey().Address(),
		Executor: exec,
		Logger:   d.Logger,
		EventBus: d.eventBus,
		Config:   d.Config,
		Tracer:   d.tracer,
		Database: d.db,
	})
	return app, nil
}

func (d *Daemon) startConsensus(app types.Application) error {
	// Create node
	tmn, err := tmnode.NewNode(
		&d.Config.Config,
		d.privVal,
		d.nodeKey,
		proxy.NewLocalClientCreator(app),
		genesis.DocProvider(d.Config),
		tmcfg.DefaultDBProvider,
		tmnode.DefaultMetricsProvider(d.Config.Instrumentation),
		d.Logger,
	)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize consensus: %v", err)
	}
	d.node = &node.Node{Node: tmn, Config: d.Config, ABCI: app}

	// Start node
	err = d.node.Start()
	if err != nil {
		return errors.UnknownError.WithFormat("start consensus: %v", err)
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
	d.localTm.Set(local.New(d.node.Node))

	return nil
}

func (d *Daemon) startServices(chGlobals <-chan *core.GlobalValues) error {
	// Wait for the executor to finish loading everything
	globals := <-chGlobals

	// Initialize all the services
	consensusSvc := tm.NewConsensusService(tm.ConsensusServiceParams{
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
		Logger:    d.Logger.With("module", "acc-rpc"),
		EventBus:  d.eventBus,
		Partition: d.Config.Accumulate.PartitionId,
		Database:  d.db,
	})
	querySvc := api.NewQuerier(api.QuerierParams{
		Logger:    d.Logger.With("module", "acc-rpc"),
		Database:  d.db,
		Partition: d.Config.Accumulate.PartitionId,
		Consensus: consensusSvc,
	})
	metricsSvc := api.NewMetricsService(api.MetricsServiceParams{
		Logger:  d.Logger.With("module", "acc-rpc"),
		Node:    consensusSvc,
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
	sequencerSvc := api.NewSequencer(api.SequencerParams{
		Logger:       d.Logger.With("module", "acc-rpc"),
		Database:     d.db,
		EventBus:     d.eventBus,
		Partition:    d.Config.Accumulate.PartitionId,
		Globals:      globals,
		ValidatorKey: d.Key().Bytes(),
	})
	messageHandler, err := message.NewHandler(
		&message.ConsensusService{ConsensusService: consensusSvc},
		&message.MetricsService{MetricsService: metricsSvc},
		&message.NetworkService{NetworkService: netSvc},
		&message.Querier{Querier: querySvc},
		&message.Submitter{Submitter: submitSvc},
		&message.Validator{Validator: validateSvc},
		&message.EventService{EventService: eventSvc},
		&message.Sequencer{Sequencer: sequencerSvc},
	)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize P2P handler: %w", err)
	}

	services := []interface{ Type() v3.ServiceType }{
		consensusSvc,
		metricsSvc,
		netSvc,
		querySvc,
		submitSvc,
		validateSvc,
		eventSvc,
		sequencerSvc,
	}
	for _, s := range services {
		d.p2pnode.RegisterService(&v3.ServiceAddress{
			Type:     s.Type(),
			Argument: d.Config.Accumulate.PartitionId,
		}, messageHandler.Handle)
	}

	return nil
}

func (d *Daemon) StartP2P() error {
	if d.p2pnode != nil {
		return nil
	}

	err := d.loadKeys()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	d.p2pnode, err = p2p.New(p2p.Options{
		Network:        d.Config.Accumulate.Network.Id,
		Listen:         d.Config.Accumulate.P2P.Listen,
		BootstrapPeers: d.Config.Accumulate.P2P.BootstrapPeers,
		Key:            ed25519.PrivateKey(d.nodeKey.PrivKey.Bytes()),
		DiscoveryMode:  dht.ModeServer,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("initialize P2P: %w", err)
	}
	return nil
}

func (d *Daemon) startAPI() error {
	d.router = routing.NewRouter(d.eventBus, d.Logger)

	// Setup the p2p node
	err := d.StartP2P()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	d.api, err = nodeapi.NewHandler(nodeapi.Options{
		Logger:  d.Logger.With("module", "acc-rpc"),
		Node:    d.p2pnode,
		Router:  d.router,
		Network: &d.Config.Accumulate.Describe,
		MaxWait: d.Config.Accumulate.API.TxMaxWaitTime,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("initialize API: %w", err)
	}

	d.apiServer = &http.Server{Handler: d.api, ReadHeaderTimeout: d.Config.Accumulate.API.ReadHeaderTimeout}
	l, secure, err := listenHttpUrl(d.Config.Accumulate.API.ListenAddress)
	if err != nil {
		return errors.UnknownError.WithFormat("start JSON-RPC: %v", err)
	}
	if secure {
		return errors.BadRequest.WithFormat("cannot start JSON-RPC: HTTPS is not supported")
	}

	if d.Config.Accumulate.API.ConnectionLimit > 0 {
		pool := make(chan struct{}, d.Config.Accumulate.API.ConnectionLimit)
		for i := 0; i < d.Config.Accumulate.API.ConnectionLimit; i++ {
			pool <- struct{}{}
		}
		l = &RateLimitedListener{Listener: l, Pool: pool}
	}

	go func() {
		err := d.apiServer.Serve(l)
		if err != nil {
			d.Logger.Error("JSON-RPC server", "err", err)
		}
	}()

	return nil
}

func (d *Daemon) startMonitoringAndCleanup() {
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

		if d.apiServer != nil {
			err := d.apiServer.Shutdown(ctx)
			if err != nil {
				d.Logger.Error("Error stopping API", "module", "jrpc", "error", err)
			}
		}

		if d.db != nil {
			err := d.db.Close()
			if err != nil {
				module := "badger"
				if d.UseMemDB {
					module = "memdb"
				}
				d.Logger.Error("Error closing database", "module", module, "error", err)
			}
		}
	}()
}

func (d *Daemon) ConnectDirectly(e *Daemon) error {
	if d.nodeKey.PrivKey.Equals(e.nodeKey.PrivKey) {
		return errors.Conflict.With("cannot connect nodes directly as they have the same node key")
	}

	err := d.p2pnode.ConnectDirectly(e.p2pnode)
	if err != nil {
		return err
	}

	return e.p2pnode.ConnectDirectly(d.p2pnode)
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
		return nil, false, errors.BadRequest.WithFormat("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, errors.BadRequest.WithFormat("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, errors.BadRequest.WithFormat("invalid address: unsupported scheme %q", u.Scheme)
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
