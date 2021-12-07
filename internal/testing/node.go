package testing

import (
	"fmt"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"os"
	"path/filepath"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/rpc/client/local"
)

var LocalBVN = &networks.Subnet{
	Name: "Local",
	Type: config.BlockValidator,
	Port: 35550,
	Nodes: []networks.Node{
		{IP: "127.0.0.1", Type: config.Validator},
	},
}

func NodeInitOptsForNetwork(network *networks.Subnet) (node.InitOptions, error) {
	listenIP := make([]string, len(network.Nodes))
	remoteIP := make([]string, len(network.Nodes))
	config := make([]*cfg.Config, len(network.Nodes))

	for i, net := range network.Nodes {
		listenIP[i] = "tcp://localhost"
		remoteIP[i] = net.IP

		config[i] = cfg.Default(network.Type, net.Type)
		config[i].Consensus.CreateEmptyBlocks = false
		config[i].Accumulate.Type = network.Type
		config[i].Accumulate.Networks = []string{fmt.Sprintf("tcp://%s:%d", remoteIP[0], network.Port)}
	}

	return node.InitOptions{
		ShardName: "accumulate.",
		SubnetID:  network.Name,
		Port:      network.Port,
		Config:    config,
		RemoteIP:  remoteIP,
		ListenIP:  listenIP,
	}, nil
}

type BVNNOptions struct {
	Dir       string
	MemDB     bool
	LogWriter func(string) io.Writer
	Logger    func(io.Writer) zerolog.Logger
}

func NewBVNN(opts BVNNOptions, cleanup func(func())) (*node.Node, *state.StateDB, *privval.FilePV, error) {
	cfg, err := cfg.Load(opts.Dir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %v", err)
	}

	var logWriter io.Writer
	if opts.LogWriter == nil {
		logWriter, err = logging.NewConsoleWriter(cfg.LogFormat)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		logWriter = opts.LogWriter(cfg.LogFormat)
	}

	logLevel, logWriter, err := logging.ParseLogLevel(cfg.LogLevel, logWriter)

	var zl zerolog.Logger
	if opts.Logger == nil {
		zl = zerolog.New(logWriter)
	} else {
		zl = opts.Logger(logWriter)
	}

	logger, err := logging.NewTendermintLogger(zl, logLevel, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse log level: %v", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(cfg.RootDir, "valacc.db")
	//ToDo: FIX:::  bvcId := sha256.Sum256([]byte(cfg.Instrumentation.Namespace))
	sdb, err := openStateDB(dbPath, opts.MemDB, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup(func() {
		_ = sdb.GetDB().Close()
	})

	// read private validator
	pv, err := privval.LoadFilePV(
		cfg.PrivValidator.KeyFile(),
		cfg.PrivValidator.StateFile(),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load file PV: %v", err)
	}

	clientProxy := node.NewLocalClient()
	mgr, err := chain.NewBlockValidatorExecutor(chain.ExecutorOptions{
		Local:           clientProxy,
		DB:              sdb,
		Logger:          logger,
		Key:             pv.Key.PrivKey.Bytes(),
		Directory:       cfg.Accumulate.Directory,
		BlockValidators: cfg.Accumulate.Networks,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create chain manager: %v", err)
	}

	app, err := abci.NewAccumulator(sdb, pv.Key.PubKey.Address(), mgr, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create ABCI app: %v", err)
	}

	node, err := node.New(cfg, app, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create node: %v", err)
	}
	go func() {
		<-node.Quit()
		_ = sdb.GetDB().Close()
	}()
	cleanup(func() {
		_ = node.Stop()
		node.Wait()
	})

	lnode, ok := node.Service.(local.NodeService)
	if !ok {
		return nil, nil, nil, fmt.Errorf("node is not a local node service!")
	}
	lclient, err := local.New(lnode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create local node client: %v", err)
	}
	clientProxy.Set(lclient)

	return node, sdb, pv, nil
}

func openStateDB(dbPath string, memDB bool, logger log.Logger) (*state.StateDB, error) {
	stateDBBuilder := state.NewStateDB().WithDebug().WithLogger(logger)
	if memDB {
		sdb, err := stateDBBuilder.OpenInMemory()
		if err != nil {
			return nil, fmt.Errorf("failed to open database %s: %v", dbPath, err)
		}
		return sdb, nil
	} else {
		sdb, err := stateDBBuilder.OpenFromFile(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open in-memory database: %v", err)
		}
		return sdb, nil
	}
}
