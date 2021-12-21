package accumulated

import (
	"context"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/AccumulateNetwork/accumulate"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	apiv1 "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/crypto"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/rpc/client/local"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	done  chan struct{}
	db    *state.StateDB
	node  *node.Node
	relay *relay.Relay
	query *apiv1.Query
	api   *http.Server
	pv    *privval.FilePV
	jrpc  *api.JrpcMethods

	// knobs for tests
	IsTest   bool
	UseMemDB bool

	// Connection & router accessible for tests
	ConnMgr    connections.ConnectionManager
	ConnRouter connections.ConnectionRouter
}

func Load(dir string, newWriter func(string) (io.Writer, error)) (*Daemon, error) {
	var daemon Daemon

	var err error
	daemon.Config, err = config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}

	if newWriter == nil {
		newWriter = logging.NewConsoleWriter
	}

	logWriter, err := newWriter(daemon.Config.LogFormat)
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
	return d.pv.Key.PrivKey
}

func (d *Daemon) Query_TESTONLY() *apiv1.Query    { return d.query }
func (d *Daemon) DB_TESTONLY() *state.StateDB     { return d.db }
func (d *Daemon) Node_TESTONLY() *node.Node       { return d.node }
func (d *Daemon) Jrpc_TESTONLY() *api.JrpcMethods { return d.jrpc }

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

	if d.Config.Accumulate.SentryDSN != "" {
		opts := sentry.ClientOptions{
			Dsn:           d.Config.Accumulate.SentryDSN,
			Environment:   "Accumulate",
			HTTPTransport: sentryHack{},
		}
		if accumulate.IsVersionKnown() {
			opts.Release = accumulate.Commit
		}
		err := sentry.Init(opts)
		if err != nil {
			return fmt.Errorf("configuring sentry: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	dbPath := filepath.Join(d.Config.RootDir, "valacc.db")
	//ToDo: FIX:::  bvcId := sha256.Sum256([]byte(config.Instrumentation.Namespace))
	d.db = new(state.StateDB)
	err = d.db.Open(dbPath, d.UseMemDB, false, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// Close the database if start fails (mostly for tests)
	defer func() {
		if err != nil {
			_ = d.db.GetDB().Close()
		}
	}()

	// read private validator
	d.pv, err = privval.LoadFilePV(
		d.Config.PrivValidator.KeyFile(),
		d.Config.PrivValidator.StateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	// Create a connection manager & router
	d.ConnMgr = connections.NewConnectionManager(&d.Config.Accumulate, d.Logger)
	d.ConnRouter = connections.NewConnectionRouter(d.ConnMgr, d.IsTest)

	// Create a proxy local client which we will populate with the local client
	// after the node has been created.
	clientProxy := node.NewLocalClient()

	bvnAddrs := d.Config.Accumulate.Network.BvnAddressesWithPortOffset(networks.TmRpcPortOffset)
	d.relay, err = relay.NewWith(clientProxy, bvnAddrs...)
	if err != nil {
		return fmt.Errorf("failed to create RPC relay: %v", err)
	}
	d.query = apiv1.NewQuery(d.relay)

	execOpts := chain.ExecutorOptions{
		Local:            clientProxy,
		ConnectionMgr:    d.ConnMgr,
		ConnectionRouter: d.ConnRouter,
		DB:               d.db,
		Logger:           d.Logger,
		Key:              d.Key().Bytes(),
		Network:          d.Config.Accumulate.Network,
		IsTest:           d.IsTest,
	}
	exec, err := chain.NewNodeExecutor(execOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	app, err := abci.NewAccumulator(d.db, d.Key().PubKey().Address(), exec, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to initialize ACBI app: %v", err)
	}

	// Create node
	d.node, err = node.New(d.Config, app, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %v", err)
	}

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
			d.node.Wait()
		}
	}()

	// Create a local client
	lnode, ok := d.node.Service.(local.NodeService)
	if !ok {
		return fmt.Errorf("node is not a local node service!")
	}

	// Create a local client
	lclClient, err := local.New(lnode)
	if err != nil {
		return fmt.Errorf("failed to create local node client: %v", err)
	}
	connInitializer := d.ConnMgr.(connections.ConnectionInitializer)
	err = connInitializer.CreateClients(lclClient)
	if err != nil {
		return fmt.Errorf("failed to initialize connection manager: %v", err)
	}
	clientProxy.Set(lclClient)

	if d.Config.Accumulate.API.EnableSubscribeTX {
		err = d.relay.Start()
		if err != nil {
			return fmt.Errorf("failed to start RPC relay: %v", err)
		}
	}

	// Configure JSON-RPC
	var jrpcOpts api.JrpcOptions
	jrpcOpts.Config = &d.Config.Accumulate.API
	jrpcOpts.QueueDuration = time.Second / 4
	jrpcOpts.QueueDepth = 100
	jrpcOpts.QueryV1 = d.query
	jrpcOpts.ConnectionRouter = d.ConnRouter
	jrpcOpts.Logger = d.Logger

	// Create the querier for JSON-RPC
	jrpcOpts.Query = api.NewQueryDispatch(d.ConnRouter, api.QuerierOptions{
		TxMaxWaitTime: d.Config.Accumulate.API.TxMaxWaitTime,
	})

	// Create the JSON-RPC handler
	d.jrpc, err = api.NewJrpc(jrpcOpts)
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	// Enable debug methods
	if d.Config.Accumulate.API.EnableDebugMethods {
		d.jrpc.EnableDebug()
	}

	// Run JSON-RPC server
	d.api = &http.Server{Handler: d.jrpc.NewMux()}
	l, secure, err := listenHttpUrl(d.Config.Accumulate.API.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to start JSON-RPC: %v", err)
	}
	if secure {
		return fmt.Errorf("failed to start JSON-RPC: HTTPS is not supported")
	}

	go func() {
		err := d.api.Serve(l)
		if err != nil {
			d.Logger.Error("JSON-RPC server", "err", err)
		}
	}()

	// Clean up once the node is stopped (mostly for tests)
	go func() {
		defer close(d.done)

		d.node.Wait()

		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
		defer cancel()

		if d.node.Config.Accumulate.API.EnableSubscribeTX {
			err := d.relay.Stop()
			if err != nil {
				d.Logger.Error("Error stopping relay", "module", "relay", "error", err)
			}
		}

		err := d.api.Shutdown(ctx)
		if err != nil {
			d.Logger.Error("Error stopping API", "module", "jrpc", "error", err)
		}

		err = d.db.GetDB().Close()
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
