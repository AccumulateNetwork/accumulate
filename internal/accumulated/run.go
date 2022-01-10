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

	"github.com/AccumulateNetwork/accumulate"
	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	apiv1 "github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	tmcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	done  chan struct{}
	db    *database.Database
	node  *node.Node
	relay *relay.Relay
	query *apiv1.Query
	api   *http.Server
	pv    *privval.FilePV
	jrpc  *api.JrpcMethods

	// knobs for tests
	IsTest   bool
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

func (d *Daemon) Query_TESTONLY() *apiv1.Query    { return d.query }
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

	if d.Config.Mode == tmcfg.ModeSeed {
		//addrBookFilePath := d.Config.P2P.AddrBookFile
	addrBookPath := filepath.Join(d.Config.P2P.RootDir, "addrbook.json")
	d.Config.P2P.AddrBook = addrBookPath
//	nodeKey, err = types.LoadOrGenNodeKey(d.Config.NodeKeyFile())
//	if err != nil {
//		return fmt.Errorf("failed to load or generate node key: %v", err)
//	}
	d.node, err = node.New(d.Config, nil, d.Logger)
	if err != nil {
		return fmt.Errorf("creating node: %v", err)
	}
	err = d.node.Start()
	if err != nil {
		return fmt.Errorf("starting node: %v", err)
	}
	
	d.node.Service.Start()

		defer func() {
			if err != nil {
				_ = d.node.Stop()
				d.node.Wait()
			}
		}()
	
	}

	if d.Config.Mode == tmcfg.ModeValidator {
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
		Local:   clientProxy,
		DB:      d.db,
		Logger:  d.Logger,
		Key:     d.Key().Bytes(),
		Network: d.Config.Accumulate.Network,
		IsTest:  d.IsTest,
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
	clientProxy.Set(lclient)

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
	jrpcOpts.Local = lclient
	jrpcOpts.Logger = d.Logger

	// Build the list of remote addresses and query clients
	jrpcOpts.Remote = d.Config.Accumulate.Network.BvnAddressesWithPortOffset(0)
	clients := make([]api.ABCIQueryClient, len(jrpcOpts.Remote))
	for i, addr := range jrpcOpts.Remote {
		switch {
		case d.Config.Accumulate.Network.BvnNames[i] == d.Config.Accumulate.Network.ID:
			jrpcOpts.Remote[i] = "local"
			clients[i] = lclient

		default:
			jrpcOpts.Remote[i], err = config.OffsetPort(addr, networks.AccRouterJsonPortOffset)
			if err != nil {
				return fmt.Errorf("invalid BVN address: %v", err)
			}
			jrpcOpts.Remote[i] += "/v2"

			addr, err = config.OffsetPort(addr, networks.TmRpcPortOffset)
			if err != nil {
				return fmt.Errorf("invalid BVN address: %v", err)
			}

			clients[i], err = rpchttp.New(addr)
			if err != nil {
				return fmt.Errorf("failed to create RPC client: %v", err)
			}
		}
	}

	// Create the querier for JSON-RPC
	jrpcOpts.Query = api.NewQueryDispatch(clients, api.QuerierOptions{
		TxMaxWaitTime: d.Config.Accumulate.API.TxMaxWaitTime,
	})

	// Create the JSON-RPC handler
	d.jrpc, err = api.NewJrpc(jrpcOpts)
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	// Enable debug methods
	if d.Config.Accumulate.API.EnableDebugMethods {
		d.jrpc.EnableDebug(lclient)
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
	}
	return nil
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
