// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"

	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"golang.org/x/exp/slog"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "accumulate-faucet <network>",
	Short: "Accumulate HTTP API node",
	Run:   run,
	Args:  cobra.ExactArgs(1),
}

var flag = struct {
	NodeKey  PrivateKeyFlag
	Key      PrivateKeyFlag
	LogLevel string
	Account  UrlFlag
	Listen   []multiaddr.Multiaddr
	Peers    []multiaddr.Multiaddr
}{}

func init() {
	flag.Key.Value = &TransientPrivateKey{}
	flag.Peers = accumulate.BootstrapServers

	cmd.Flags().Var(&flag.NodeKey, "node-key", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().Var(&flag.Key, "key", "The key used to sign faucet transactions")
	cmd.Flags().Var(&flag.Account, "account", "The faucet account")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.Listen), "listen", "P2P listening address(es)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")

	_ = cmd.MarkFlagRequired("peer")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("key")
}

func run(_ *cobra.Command, args []string) {
	svcCfg := &FaucetService{
		Account:    flag.Account.V,
		SigningKey: flag.Key.Value,
		Router:     ServiceValue(&RouterService{}),
	}
	cfg := &Config{
		Network: args[0],
		Logging: &Logging{
			Rules: []*LoggingRule{{
				Level: slog.LevelInfo,
			}},
		},
		P2P: &P2P{
			Key:            flag.NodeKey.Value,
			Listen:         flag.Listen,
			BootstrapPeers: flag.Peers,
		},
		Services: []Service{svcCfg},
	}

	ctx := cmdutil.ContextForMainProcess(context.Background())
	inst, err := Start(ctx, cfg)
	Check(err)
	<-ctx.Done()
	inst.Stop()
}
