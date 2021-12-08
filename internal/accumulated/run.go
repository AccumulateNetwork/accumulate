package accumulated

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
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
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
)

type Daemon struct {
	Config *config.Config
	Logger tmlog.Logger

	db    *state.StateDB
	node  *node.Node
	relay *relay.Relay
	api   *http.Server
}

func Load(dir string, newWriter func(string) (io.Writer, error)) (*Daemon, error) {
	var daemon Daemon
	var err error
	daemon.Config, err = config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}

	var logWriter io.Writer
	if newWriter == nil {
		logWriter = os.Stderr
	} else {
		logWriter, err = newWriter(daemon.Config.LogFormat)
	}
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

func (d *Daemon) Start() error {
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
	err := d.db.Open(dbPath, false, false, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// read private validator
	pv, err := privval.LoadFilePV(
		d.Config.PrivValidator.KeyFile(),
		d.Config.PrivValidator.StateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	// Create a proxy local client which we will populate with the local client
	// after the node has been created.
	clientProxy := node.NewLocalClient()

	d.relay, err = relay.NewWith(clientProxy, d.Config.Accumulate.Networks...)
	if err != nil {
		return fmt.Errorf("failed to create RPC relay: %v", err)
	}

	execOpts := chain.ExecutorOptions{
		SubnetType:      d.Config.Accumulate.Type,
		Local:           clientProxy,
		DB:              d.db,
		Logger:          d.Logger,
		Key:             pv.Key.PrivKey.Bytes(),
		Directory:       d.Config.Accumulate.Directory,
		BlockValidators: d.Config.Accumulate.Networks,
	}
	exec, err := chain.NewNodeExecutor(execOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	app, err := abci.NewAccumulator(d.db, pv.Key.PubKey.Address(), exec, d.Logger)
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

	if d.Config.Accumulate.API.EnableSubscribeTX {
		err = d.relay.Start()
		if err != nil {
			return fmt.Errorf("failed to start RPC relay: %v", err)
		}
	}

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

	// Configure JSON-RPC
	var jrpcOpts api.JrpcOptions
	jrpcOpts.Config = &d.Config.Accumulate.API
	jrpcOpts.QueueDuration = time.Second / 4
	jrpcOpts.QueueDepth = 100
	jrpcOpts.QueryV1 = apiv1.NewQuery(d.relay)
	jrpcOpts.Local = lclient
	jrpcOpts.Logger = d.Logger

	// Build the list of remote addresses and query clients
	jrpcOpts.Remote = make([]string, len(d.Config.Accumulate.Networks))
	clients := make([]api.ABCIQueryClient, len(d.Config.Accumulate.Networks))
	for i, net := range d.Config.Accumulate.Networks {
		switch {
		case net == "self", net == d.Config.Accumulate.Network, net == d.Config.RPC.ListenAddress:
			jrpcOpts.Remote[i] = "local"
			clients[i] = lclient

		default:
			addr, err := networks.GetRpcAddr(net)
			if err != nil {
				return fmt.Errorf("invalid network name or address: %v", err)
			}

			jrpcOpts.Remote[i] = addr
			clients[i], err = rpchttp.New(addr)
			if err != nil {
				return fmt.Errorf("failed to create RPC client: %v", err)
			}
		}
	}

	jrpcOpts.Query = api.NewQueryDispatch(clients, api.QuerierOptions{
		TxMaxWaitTime: d.Config.Accumulate.API.TxMaxWaitTime,
	})

	jrpc, err := api.NewJrpc(jrpcOpts)
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	jrpc.EnableDebug(lclient)

	// Run JSON-RPC server
	d.api = &http.Server{Handler: jrpc.NewMux()}
	l, secure, err := listenHttpUrl(d.Config.Accumulate.API.JSONListenAddress)
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
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	var errs []error
	errs = append(errs, d.node.Stop())
	if d.node.Config.Accumulate.API.EnableSubscribeTX {
		errs = append(errs, d.relay.Stop())
	}
	errs = append(errs, d.api.Shutdown(ctx))
	errs = append(errs, d.db.GetDB().Close())

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
