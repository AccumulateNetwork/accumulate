package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated"
	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/getsentry/sentry-go"
	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

type Program struct {
	cmd   *cobra.Command
	db    *state.StateDB
	node  *node.Node
	relay *relay.Relay
	api   *api.API
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

	config, err := config.Load(workDir)
	if err != nil {
		return fmt.Errorf("reading config file: %v", err)
	}

	if config.Accumulate.SentryDSN != "" {
		opts := sentry.ClientOptions{
			Dsn:           config.Accumulate.SentryDSN,
			Environment:   "Accumulate",
			HTTPTransport: sentryHack{},
			Debug:         true,
		}
		if accumulated.IsVersionKnown() {
			opts.Release = accumulated.Commit
		}
		err = sentry.Init(opts)
		if err != nil {
			return fmt.Errorf("configuring sentry: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	dbPath := filepath.Join(config.RootDir, "valacc.db")
	bvcId := sha256.Sum256([]byte(config.Instrumentation.Namespace))
	p.db = new(state.StateDB)
	err = p.db.Open(dbPath, bvcId[:], false, true)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %v", dbPath, err)
	}

	// read private validator
	pv, err := privval.LoadFilePV(
		config.PrivValidator.KeyFile(),
		config.PrivValidator.StateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	rpcClient, err := rpchttp.New(config.RPC.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to reate RPC client: %v", err)
	}

	query := api.NewQuery(relay.New(rpcClient))
	mgr, err := chain.NewBlockValidator(query, p.db, pv.Key.PrivKey.Bytes())
	if err != nil {
		return fmt.Errorf("failed to initialize chain manager: %v", err)
	}

	var logger log.Logger
	if service.Interactive() {
		logger, err = log.NewDefaultLogger(config.LogFormat, config.LogLevel, false)
	} else {
		logger, err = newDefaultLogger(s, config.LogFormat, config.LogLevel, false)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize logger: %v", err)
		os.Exit(1)
	}

	app, err := abci.NewAccumulator(p.db, pv.Key.PubKey.Address(), mgr, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize ACBI app: %v", err)
	}

	// Create node
	p.node, err = node.New(config, app, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %v", err)
	}

	// Start node
	// TODO Feed Tendermint logger to service logger
	err = p.node.Start()
	if err != nil {
		return fmt.Errorf("failed to start node: %v", err)
	}

	p.relay, err = relay.NewWith(config.Accumulate.Networks...)
	if err != nil {
		return fmt.Errorf("failed to start relay: %v", err)
	}

	query = api.NewQuery(p.relay)
	p.api, err = api.StartAPI(&config.Accumulate.API, query)
	if err != nil {
		return fmt.Errorf("failed to start API: %v", err)
	}
	return nil
}

func (p *Program) Stop(service.Service) error {
	var errs []error
	errs = append(errs, p.node.Stop())
	// TODO stop relay
	// TODO stop API
	errs = append(errs, p.db.GetDB().Close())

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
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

type serviceLogger struct {
	service.Logger
	w   *zerolog.ConsoleWriter
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func newServiceLogger(sl service.Logger, cw *zerolog.ConsoleWriter) *serviceLogger {
	l := new(serviceLogger)
	l.Logger = sl
	if cw != nil {
		l.w = cw
		l.buf = new(bytes.Buffer)
		l.w.Out = l.buf
		l.mu = new(sync.Mutex)
	}
	return l
}

var _ zerolog.LevelWriter = (*serviceLogger)(nil)

func (l *serviceLogger) Write(b []byte) (int, error) {
	return l.WriteLevel(zerolog.NoLevel, b)
}

func (l *serviceLogger) WriteLevel(level zerolog.Level, b []byte) (int, error) {
	// Use zerolog's console writer to format the log message
	if l.w != nil {
		l.mu.Lock()
		l.buf.Reset()
		_, _ = l.w.Write(b)
		b = make([]byte, l.buf.Len())
		copy(b, l.buf.Bytes())
		l.mu.Unlock()
	}

	switch level {
	case zerolog.PanicLevel, zerolog.FatalLevel, zerolog.ErrorLevel:
		_ = l.Error(string(b))
	case zerolog.WarnLevel:
		_ = l.Warning(string(b))
	default:
		_ = l.Info(string(b))
	}
	return len(b), nil
}
