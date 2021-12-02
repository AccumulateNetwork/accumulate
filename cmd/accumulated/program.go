package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
)

type Program struct {
	cmd   *cobra.Command
	db    *state.StateDB
	node  *node.Node
	relay *relay.Relay
	api   *http.Server
}

func NewProgram(cmd *cobra.Command) *Program {
	p := new(Program)
	p.cmd = cmd
	return p
}

func (p *Program) workDir() (string, error) {
	if p.cmd.Flag("node").Changed {
		nodeDir := fmt.Sprintf("Node%d", flagRun.Node)
		return filepath.Join(flagMain.WorkDir, nodeDir), nil
	}

	if p.cmd.Flag("work-dir").Changed {
		return flagMain.WorkDir, nil
	}

	if service.Interactive() {
		fmt.Fprint(os.Stderr, "Error: at least one of --work-dir or --node is required\n")
		printUsageAndExit1(p.cmd, []string{})
		return "", nil // Not reached
	} else {
		return "", fmt.Errorf("at least one of --work-dir or --node is required")
	}
}

func (p *Program) Start(s service.Service) error {
	workDir, err := p.workDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(workDir)
	if err != nil {
		return fmt.Errorf("reading config file: %v", err)
	}

	var logWriter io.Writer
	if service.Interactive() {
		logWriter, err = logging.NewConsoleWriter(cfg.LogFormat)
	} else {
		logWriter, err = logging.NewServiceLogger(s, cfg.LogFormat)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize log writer: %v", err)
		os.Exit(1)
	}

	logLevel, logWriter, err := logging.ParseLogLevel(cfg.LogLevel, logWriter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse log level: %v", err)
		os.Exit(1)
	}

	logger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize logger: %v", err)
		os.Exit(1)
	}

	if cfg.Accumulate.SentryDSN != "" {
		opts := sentry.ClientOptions{
			Dsn:           cfg.Accumulate.SentryDSN,
			Environment:   "Accumulate",
			HTTPTransport: sentryHack{},
		}
		if accumulate.IsVersionKnown() {
			opts.Release = accumulate.Commit
		}
		err = sentry.Init(opts)
		if err != nil {
			return fmt.Errorf("configuring sentry: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	dbPath := filepath.Join(cfg.RootDir, "valacc.db")
	//ToDo: FIX:::  bvcId := sha256.Sum256([]byte(config.Instrumentation.Namespace))
	p.db = new(state.StateDB)
	err = p.db.Open(dbPath, false, false, logger)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// read private validator
	pv, err := privval.LoadFilePV(
		cfg.PrivValidator.KeyFile(),
		cfg.PrivValidator.StateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	p.relay, err = relay.NewWith(cfg.Accumulate.Networks...)
	if err != nil {
		return fmt.Errorf("failed to create RPC relay: %v", err)
	}

	// Create a proxy local client which we will populate with the local client
	// after the node has been created.
	clientProxy := node.NewLocalClient()

	execOpts := chain.ExecutorOptions{
		Query:  apiv1.NewQuery(p.relay),
		Local:  clientProxy,
		DB:     p.db,
		Logger: logger,
		Key:    pv.Key.PrivKey.Bytes(),
	}
	var exec *chain.Executor
	switch cfg.Accumulate.Type {
	case config.BlockValidator:
		exec, err = chain.NewBlockValidatorExecutor(execOpts)
	case config.Directory:
		exec, err = chain.NewDirectoryExecutor(execOpts)
	default:
		return fmt.Errorf("%q is not a valid Accumulate subnet type", cfg.Accumulate.Type)
	}
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	app, err := abci.NewAccumulator(p.db, pv.Key.PubKey.Address(), exec, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize ACBI app: %v", err)
	}

	// Create node
	p.node, err = node.New(cfg, app, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %v", err)
	}

	// Start node
	// TODO Feed Tendermint logger to service logger
	err = p.node.Start()
	if err != nil {
		return fmt.Errorf("failed to start node: %v", err)
	}

	if cfg.Accumulate.API.EnableSubscribeTX {
		err = p.relay.Start()
		if err != nil {
			return fmt.Errorf("failed to start RPC relay: %v", err)
		}
	}

	// Create a local client
	lnode, ok := p.node.Service.(local.NodeService)
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
	jrpcOpts.Config = &cfg.Accumulate.API
	jrpcOpts.QueueDuration = time.Second / 4
	jrpcOpts.QueueDepth = 100
	jrpcOpts.QueryV1 = apiv1.NewQuery(p.relay)
	jrpcOpts.Local = lclient

	// Build the list of remote addresses and query clients
	jrpcOpts.Remote = make([]string, len(cfg.Accumulate.Networks))
	clients := make([]api.ABCIQueryClient, len(cfg.Accumulate.Networks))
	for i, net := range cfg.Accumulate.Networks {
		switch {
		case net == "self", net == cfg.Accumulate.Network, net == cfg.RPC.ListenAddress:
			jrpcOpts.Remote[i] = "local"
			clients[i] = lclient

		default:
			addr, err := networks.GetRpcAddr(net, networks.TmRpcPortOffset)
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
		TxMaxWaitTime: cfg.Accumulate.API.TxMaxWaitTime,
	})

	jrpc, err := api.NewJrpc(jrpcOpts)
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}

	// Run JSON-RPC server
	p.api = &http.Server{Handler: jrpc.NewMux()}
	l, secure, err := listenHttpUrl(cfg.Accumulate.API.JSONListenAddress)
	if err != nil {
		return fmt.Errorf("failed to start JSON-RPC: %v", err)
	}
	if secure {
		return fmt.Errorf("failed to start JSON-RPC: HTTPS is not supported")
	}

	go func() {
		err := p.api.Serve(l)
		if err != nil {
			logger.Error("JSON-RPC server", "err", err)
		}
	}()

	return nil
}

func (p *Program) Stop(service.Service) error {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	var errs []error
	errs = append(errs, p.node.Stop())
	if p.node.Config.Accumulate.API.EnableSubscribeTX {
		errs = append(errs, p.relay.Stop())
	}
	errs = append(errs, p.api.Shutdown(ctx))
	errs = append(errs, p.db.GetDB().Close())

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
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

type sentryHack struct{}

func (sentryHack) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "gitlab.com" {
		return http.DefaultTransport.RoundTrip(req)
	}

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("Failed to send event to sentry: %v", r)
		}
	}()

	defer req.Body.Close()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var v map[string]interface{}
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}

	ex := v["exception"]

	// GitLab expects the event to have a different shape
	v["exception"] = map[string]interface{}{
		"values": ex,
	}

	b, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}

	req.ContentLength = int64(len(b))
	req.Body = io.NopCloser(bytes.NewReader(b))
	resp, err := http.DefaultTransport.RoundTrip(req)
	return resp, err
}
