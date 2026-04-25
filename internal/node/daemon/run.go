// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/exp/loki"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	nodeapi "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Daemon struct {
	Config *config.Config
	Logger logging.Logger

	done      chan struct{}
	db        *database.Database
	apiServer *http.Server
	privVal   *FilePV
	p2pnode   *p2p.Node
	api       *nodeapi.Handler
	nodeKey   *NodeKey
	router    routing.Router
	eventBus  *events.Bus
	tracer    trace.Tracer

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
	daemon.Config = cfg
	daemon.done = make(chan struct{})

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

	if cfg.Accumulate.Logging.EnableLoki {
		hostname, _ := os.Hostname()
		ch, err := loki.Start(loki.Options{
			Url:      cfg.Accumulate.Logging.LokiUrl,
			Username: cfg.Accumulate.Logging.LokiUsername,
			Password: cfg.Accumulate.Logging.LokiPassword,
			Labels: map[string]string{
				"hostname":  hostname,
				"process":   "accumulated",
				"network":   cfg.Accumulate.Network.Id,
				"partition": cfg.Accumulate.PartitionId,
			},
		})
		if err != nil {
			return nil, errors.BadRequest.WithFormat("init Loki: %v", err)
		}

		pipe := make(chan *loki.Entry)
		go func() {
			defer close(ch)
			for {
				select {
				case e := <-pipe:
					ch <- e
				case <-daemon.done:
					return
				}
			}
		}()

		logWriter = io.MultiWriter(logWriter, writeFunc(func(b []byte) (int, error) {
			var evt struct {
				// Time  time.Time     `json:"time"`
				Level zerolog.Level `json:"level"`
			}
			if json.Unmarshal(b, &evt) != nil || evt.Level < zerolog.InfoLevel {
				return len(b), nil
			}
			// if evt.Time.IsZero() {
			// 	evt.Time = time.Now()
			// }

			pipe <- &loki.Entry{
				Timestamp: timestamppb.Now(),
				Line:      string(b),
			}

			return len(b), nil
		}))
	}

	tmLogger, err := logging.NewTendermintLogger(zerolog.New(logWriter), logLevel, false)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize logger: %v", err)
	}
	daemon.Logger = tmLogger

	daemon.eventBus = events.NewBus(daemon.Logger.With("module", "events"))
	return &daemon, nil
}

func (d *Daemon) Key() ed25519.PrivateKey {
	return d.privVal.Key.PrivKey
}

func (d *Daemon) DB_TESTONLY() *database.Database { return d.db }
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
	defer func() {
		if err != nil {
			close(d.done)
		}
	}()

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

	// CometBFT ABCI app, consensus, and services have been removed.
	// Use accumulated-dagbft instead.
	return errors.NotAllowed.With("CometBFT consensus removed; use accumulated-dagbft")
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
	d.privVal, err = LoadFilePV(
		d.Config.PrivValidatorKeyFile(),
		d.Config.PrivValidatorStateFile(),
	)
	if err != nil {
		return errors.UnknownError.WithFormat("load private validator key: %v", err)
	}

	d.nodeKey, err = LoadNodeKey(d.Config.NodeKeyFile())
	if err != nil {
		return errors.UnknownError.WithFormat("load node key: %v", err)
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
		Key:            d.nodeKey.PrivKey,
		DiscoveryMode:  dht.ModeServer,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("initialize P2P: %w", err)
	}
	return nil
}

func (d *Daemon) startAPI() error {
	d.router = routing.NewRouter(routing.RouterOptions{
		Events: d.eventBus,
		Logger: d.Logger,
	})

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


func (d *Daemon) ConnectDirectly(e *Daemon) error {
	if d.nodeKey.PrivKey.Equal(e.nodeKey.PrivKey) {
		return errors.Conflict.With("cannot connect nodes directly as they have the same node key")
	}

	err := d.p2pnode.ConnectDirectly(e.p2pnode)
	if err != nil {
		return err
	}

	return e.p2pnode.ConnectDirectly(d.p2pnode)
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

// ErrAlreadyStopped is returned by Stop when the daemon has already been
// stopped. Callers (e.g. cmd_run.go's runNode) treat this as a benign
// condition. Use errors.Is to match it.
var ErrAlreadyStopped = stderrors.New("already stopped")

func (d *Daemon) Stop() error {
	select {
	case <-d.done:
		return ErrAlreadyStopped
	default:
	}
	close(d.done)
	return nil
}

func (d *Daemon) Done() <-chan struct{} {
	return d.done
}

type writeFunc func([]byte) (int, error)

func (l writeFunc) Write(b []byte) (int, error) {
	return l(b)
}
