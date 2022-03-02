package accumulated

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/crypto"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/rpc/client/local"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	statuschk "gitlab.com/accumulatenetwork/accumulate/internal/connections/status"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	done              chan struct{}
	db                *database.Database
	node              *node.Node
	api               *http.Server
	pv                *privval.FilePV
	jrpc              *api.JrpcMethods
	connectionManager connections.ConnectionInitializer

	// knobs for tests
	// IsTest   bool
	UseMemDB bool
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

func (d *Daemon) DB_TESTONLY() *database.Database { return d.db }
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
	d.db, err = database.Open(dbPath, d.UseMemDB, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// Close the database if start fails (mostly for tests)
	defer func() {
		if err != nil {
			_ = d.db.Close()
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

	d.connectionManager = connections.NewConnectionManager(d.Config, d.Logger)

	router := routing.RouterInstance{
		ConnectionManager: d.connectionManager,
		Network:           &d.Config.Accumulate.Network,
	}
	execOpts := chain.ExecutorOptions{
		DB:      d.db,
		Logger:  d.Logger,
		Key:     d.Key().Bytes(),
		Network: d.Config.Accumulate.Network,
		Router:  &router,
	}
	exec, err := chain.NewNodeExecutor(execOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	err = exec.Start()
	if err != nil {
		return fmt.Errorf("failed to start chain executor: %v", err)
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		DB:      d.db,
		Address: d.Key().PubKey().Address(),
		Chain:   exec,
		Logger:  d.Logger,
		Network: d.Config.Accumulate.Network,
	})

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
	lclient, err := local.New(lnode)
	if err != nil {
		return fmt.Errorf("failed to create local node client: %v", err)
	}

	if d.Config.Accumulate.API.DebugJSONRPC {
		jsonrpc2.DebugMethodFunc = true
	}

	// Create the JSON-RPC handler
	d.jrpc, err = api.NewJrpc(api.Options{
		Logger:           d.Logger,
		Network:          &d.Config.Accumulate.Network,
		Router:           &router,
		PrometheusServer: d.Config.Accumulate.API.PrometheusServer,
		TxMaxWaitTime:    d.Config.Accumulate.API.TxMaxWaitTime,
	})
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	// Let the connection manager create and assign clients
	statusChecker := statuschk.NewNodeStatusChecker()
	err = d.connectionManager.InitClients(lclient, statusChecker)
	if err != nil {
		return fmt.Errorf("failed to initialize the connection manager: %v", err)
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

	// Shut down the node if the disk space gets too low
	go d.ensureSufficientDiskSpace(dbPath)

	// Clean up once the node is stopped (mostly for tests)
	go func() {
		defer close(d.done)

		d.node.Wait()

		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
		defer cancel()

		err := d.api.Shutdown(ctx)
		if err != nil {
			d.Logger.Error("Error stopping API", "module", "jrpc", "error", err)
		}

		err = exec.Stop()
		if err != nil {
			d.Logger.Error("Error stopping executor", "module", "executor", "error", err)
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

func (d *Daemon) ConnectDirectly(e *Daemon) error {
	err := d.connectionManager.ConnectDirectly(e.connectionManager)
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
