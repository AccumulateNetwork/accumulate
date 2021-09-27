package node

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AccumulateNetwork/accumulated/config"
	abci "github.com/tendermint/tendermint/abci/types"
	tmconfig "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/rpc/client/local"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	rpcclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
)

// AppFactory creates and returns an ABCI application.
type AppFactory func(*privval.FilePV) (abci.Application, error)

// Node wraps a Tendermint node.
type Node struct {
	*nm.Node
	config      *config.Config
	PV          *privval.FilePV
	APIClient   coregrpc.BroadcastAPIClient
	LocalClient *local.Local
}

// New initializes a Tendermint node for the given ABCI application.
func New(config *config.Config, appFactory AppFactory) (*Node, error) {
	node := new(Node)
	node.config = config

	// create logger
	var err error
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
	logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, tmconfig.DefaultLogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}

	// read private validator
	node.PV = privval.LoadFilePV(
		config.PrivValidatorKeyFile(),
		config.PrivValidatorStateFile(),
	)

	// read node key
	nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load node's key: %w", err)
	}

	// initialize application
	app, err := appFactory(node.PV)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize application: %w", err)
	}

	// create node
	node.Node, err = nm.NewNode(
		&config.Config,
		node.PV,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(&config.Config),
		nm.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
	}

	return node, nil
}

// Start starts the Tendermint node.
func (n *Node) Start() error {
	err := n.Node.Start()
	if err != nil {
		return err
	}

	n.LocalClient = local.New(n.Node)
	n.APIClient = n.waitForGRPC()

	return n.waitForRPC()
}

func (n *Node) waitForGRPC() coregrpc.BroadcastAPIClient {
	client := coregrpc.StartGRPCClient(n.config.RPC.GRPCListenAddress)
	for {
		_, err := client.Ping(context.Background(), &coregrpc.RequestPing{})
		if err == nil {
			return client
		}
	}
}

func (n *Node) waitForRPC() error {
	client, err := rpcclient.New(n.config.RPC.ListenAddress)
	if err != nil {
		return err
	}

	result := new(ctypes.ResultStatus)
	for {
		_, err := client.Call(context.Background(), "status", map[string]interface{}{}, result)
		if err == nil {
			return nil
		}

		time.Sleep(time.Millisecond)
	}
}
