// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package node

import (
	"context"

	"github.com/cometbft/cometbft/node"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	corerpc "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

// Node wraps a Tendermint node.
type Node struct {
	*node.Node
	Config *config.Config
}

// Start starts the Tendermint node.
func (n *Node) Start() error {
	err := n.Node.Start()
	if err != nil {
		return err
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
