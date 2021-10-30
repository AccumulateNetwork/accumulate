package testing

import (
	"fmt"
	"os"
	"path/filepath"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/logging"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/rs/zerolog"
	tmcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/privval"
)

func NodeInitOptsForNetwork(name string) (node.InitOptions, error) {
	network := networks.Networks[name]
	listenIP := make([]string, len(network.Nodes))
	remoteIP := make([]string, len(network.Nodes))
	config := make([]*cfg.Config, len(network.Nodes))

	for i, net := range network.Nodes {
		listenIP[i] = "tcp://localhost"
		remoteIP[i] = net.IP

		config[i] = new(cfg.Config)
		switch net.Type {
		case cfg.Validator:
			config[i].Config = *tmcfg.DefaultValidatorConfig()
		case cfg.Follower:
			config[i].Config = *tmcfg.DefaultConfig()
		default:
			return node.InitOptions{}, fmt.Errorf("Error: hard-coded network has invalid node type: %q\n", net.Type)
		}

		config[i].LogLevel = "error"
		config[i].Consensus.CreateEmptyBlocks = false
		config[i].Accumulate.Type = network.Type
		config[i].Accumulate.Networks = []string{fmt.Sprintf("tcp://%s:%d", remoteIP[0], network.Port)}
	}

	return node.InitOptions{
		ShardName: "accumulate.",
		ChainID:   network.Name,
		Port:      network.Port,
		Config:    config,
		RemoteIP:  remoteIP,
		ListenIP:  listenIP,
	}, nil
}

func NewBVCNode(dir string, memDB bool, relayTo []string, newZL func(string) zerolog.Logger, cleanup func(func())) (*node.Node, *state.StateDB, *privval.FilePV, error) {
	cfg, err := cfg.Load(dir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %v", err)
	}

	dbPath := filepath.Join(cfg.RootDir, "valacc.db")
	//ToDo: FIX:::  bvcId := sha256.Sum256([]byte(cfg.Instrumentation.Namespace))
	sdb := new(state.StateDB)
	err = sdb.Open(dbPath, memDB, true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open database %s: %v", dbPath, err)
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

	if relayTo == nil {
		relayTo = []string{cfg.RPC.ListenAddress}
	}

	relay, err := relay.NewWith(relayTo...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create RPC relay: %v", err)
	}

	mgr, err := chain.NewBlockValidator(api.NewQuery(relay), sdb, pv.Key.PrivKey.Bytes())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create chain manager: %v", err)
	}

	var zl zerolog.Logger
	if newZL == nil {
		w, err := logging.NewConsoleWriter(cfg.LogFormat)
		if err != nil {
			return nil, nil, nil, err
		}
		zl = zerolog.New(w)
	} else {
		zl = newZL(cfg.LogFormat)
	}

	logger, err := logging.NewTendermintLogger(zl, cfg.LogLevel, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse log level: %v", err)
		os.Exit(1)
	}

	sdb.SetLogger(logger)

	app, err := abci.NewAccumulator(sdb, pv.Key.PubKey.Address(), mgr, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create ABCI app: %v", err)
	}

	node, err := node.New(cfg, app, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create node: %v", err)
	}
	cleanup(func() {
		_ = node.Stop()
		node.Wait()
	})

	return node, sdb, pv, nil
}
