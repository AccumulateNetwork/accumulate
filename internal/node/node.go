package node

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"

	abciclient "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/privval"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	"gitlab.com/accumulatenetwork/accumulate/config"
	web "gitlab.com/accumulatenetwork/accumulate/internal/web/static"
)

// AppFactory creates and returns an ABCI application.
type AppFactory func(*privval.FilePV) (abci.Application, error)

// Node wraps a Tendermint node.
type Node struct {
	service.Service
	Config *config.Config
	ABCI   abci.Application
	logger log.Logger
}

// New initializes a Tendermint node for the given ABCI application.
func New(config *config.Config, app abci.Application, logger log.Logger) (*Node, error) {
	node := new(Node)
	node.Config = config
	node.ABCI = app
	node.logger = logger

	// create node
	var err error
	node.Service, err = nm.New(&config.Config, logger, abciclient.NewLocalCreator(app), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Tendermint node: %w", err)
	}

	return node, nil
}

// Start starts the Tendermint node.
func (n *Node) Start() error {
	err := n.Service.Start()
	if err != nil {
		return err
	}
	if n.Config.Accumulate.Website.Enabled {
		u, err := url.Parse(n.Config.Accumulate.Website.ListenAddress)
		if err != nil {
			return fmt.Errorf("invalid website listen address: %v", err)
		}
		if u.Scheme != "http" {
			return fmt.Errorf("invalid website listen address: expected scheme http, got %q", u.Scheme)
		}

		website := http.Server{Addr: u.Host, Handler: http.FileServer(http.FS(web.FS))}
		go func() {
			<-n.Quit()
			_ = website.Shutdown(context.Background())
		}()
		go func() {
			n.logger.Info("Listening", "host", u.Host, "module", "website")
			err := website.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				stdlog.Fatalf("Failed to start website: %v", err)
			}
		}()
	}
	n.waitForGRPC()
	return nil
}

func (n *Node) waitForGRPC() coregrpc.BroadcastAPIClient {
	client := coregrpc.StartGRPCClient(n.Config.RPC.GRPCListenAddress)
	for {
		_, err := client.Ping(context.Background(), &coregrpc.RequestPing{})
		if err == nil {
			return client
		}
	}
}
