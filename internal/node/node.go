// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package node

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/privval"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	corerpc "github.com/tendermint/tendermint/rpc/jsonrpc/client"
	"gitlab.com/accumulatenetwork/accumulate/config"
	web "gitlab.com/accumulatenetwork/accumulate/internal/web/static"
)

// AppFactory creates and returns an ABCI application.
type AppFactory func(*privval.FilePV) (abci.Application, error)

// Node wraps a Tendermint node.
type Node struct {
	*node.Node
	Config *config.Config
	ABCI   abci.Application
}

// Start starts the Tendermint node.
func (n *Node) Start() error {
	err := n.Node.Start()
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
			n.Logger.Info("Listening", "host", u.Host, "module", "website")
			err := website.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				stdlog.Fatalf("Failed to start website: %v", err)
			}
		}()
	}
	_, err = n.waitForRPC()
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) waitForRPC() (*corerpc.Client, error) {
	client, err := corerpc.New(n.Config.RPC.ListenAddress)
	if err != nil {
		return nil, err
	}
	result := new(*coretypes.ResultStatus)
	for {
		_, err := client.Call(context.Background(), "status", nil, &result)
		if err == nil {
			return client, nil
		}
	}

}
