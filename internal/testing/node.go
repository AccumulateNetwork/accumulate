package testing

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/abci"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/chain"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/types/state"
	tmcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
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
		config[i].Accumulate.Networks = []string{remoteIP[0]}
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

func NewBVCNode(dir string, cleanup func(func())) (*node.Node, *privval.FilePV, error) {
	cfg, err := cfg.Load(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %v", err)
	}

	dbPath := filepath.Join(cfg.RootDir, "valacc.db")
	bvcId := sha256.Sum256([]byte(cfg.Instrumentation.Namespace))
	sdb := new(state.StateDB)
	err = sdb.Open(dbPath, bvcId[:], false, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database %s: %v", dbPath, err)
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
		return nil, nil, fmt.Errorf("failed to load file PV: %v", err)
	}

	rpcClient, err := rpchttp.New(cfg.RPC.ListenAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reate RPC client: %v", err)
	}

	mgr, err := chain.NewBlockValidator(api.NewQuery(relay.New(rpcClient)), sdb, pv.Key.PrivKey.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create chain manager: %v", err)
	}

	logger, err := log.NewDefaultLogger(cfg.LogFormat, cfg.LogLevel, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse log level: %v", err)
		os.Exit(1)
	}

	app, err := abci.NewAccumulator(sdb, pv.Key.PubKey.Address(), mgr, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ABCI app: %v", err)
	}

	node, err := node.New(cfg, app, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create node: %v", err)
	}
	cleanup(func() {
		_ = node.Stop()
		node.Wait()
	})

	return node, pv, nil
}
