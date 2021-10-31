package node

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/AccumulateNetwork/accumulated/config"
	web "github.com/AccumulateNetwork/accumulated/internal/web/static"
	"github.com/AccumulateNetwork/accumulated/networks"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	nm "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	rpcclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
)

// AppFactory creates and returns an ABCI application.
type AppFactory func(*privval.FilePV) (abci.Application, error)

// Node wraps a Tendermint node.
type Node struct {
	service.Service
	Config *config.Config
}

// New initializes a Tendermint node for the given ABCI application.
func New(config *config.Config, app abci.Application, logger log.Logger) (*Node, error) {
	node := new(Node)
	node.Config = config

	// create node
	var err error
	node.Service, err = nm.New(&config.Config, logger, proxy.NewLocalClientCreator(app), nil)
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

	if n.Config.Accumulate.WebsiteEnabled {
		u, err := url.Parse(n.Config.Accumulate.WebsiteListenAddress)
		if err != nil {
			return fmt.Errorf("invalid website listen address: %v", err)
		}
		if u.Scheme != "tcp" {
			return fmt.Errorf("invalid website listen address: expected scheme tcp, got %q", u.Scheme)
		}

		website := http.Server{Addr: u.Host, Handler: http.FileServer(http.FS(web.FS))}
		go func() {
			<-n.Quit()
			website.Shutdown(context.Background())
		}()
		go func() {
			stdlog.Printf("Starting website on %s", u.Host)
			err := website.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				stdlog.Fatalf("Failed to start website: %v", err)
			}
		}()
	}

	n.waitForGRPC()

	if n.Config.Accumulate.API.EnableSubscribeTX {
		return n.waitForRPC()
	}
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

func (n *Node) waitForRPC() error {
	for _, bvc := range n.Config.Accumulate.Networks {
		addr, err := networks.GetRpcAddr(bvc, TmRpcPortOffset)
		if err != nil {
			return err
		}

		client, err := rpcclient.New(addr)
		if err != nil {
			return err
		}

		result := new(ctypes.ResultStatus)
		for {
			_, err := client.Call(context.Background(), "status", map[string]interface{}{}, result)
			if err == nil {
				break
			}
			if !errIsConnRefused(err) {
				return err
			}

			time.Sleep(time.Millisecond)
		}
	}
	return nil
}

func errIsConnRefused(err error) bool {
	var err1 *url.Error
	if !errors.As(err, &err1) {
		return false
	}

	var err2 *net.OpError
	if !errors.As(err1.Err, &err2) {
		return false
	}

	var err3 *os.SyscallError
	if !errors.As(err2.Err, &err3) {
		return false
	}

	return errors.Is(err3.Err, syscall.ECONNREFUSED)
}
