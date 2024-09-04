// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log/slog"
	"os/user"
	"path/filepath"

	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
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
	NodeKey      PrivateKeyFlag
	Key          PrivateKeyFlag
	LogLevel     string
	Account      UrlFlag
	Listen       []multiaddr.Multiaddr
	PromListen   []multiaddr.Multiaddr
	Peers        []multiaddr.Multiaddr
	PeerDatabase string
}{}

var cu = func() *user.User {
	cu, _ := user.Current()
	return cu
}()

func init() {
	flag.Key.Value = &TransientPrivateKey{}

	if cu != nil {
		flag.PeerDatabase = filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json")
	}

	cmd.Flags().Var(&flag.NodeKey, "node-key", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().Var(&flag.Key, "key", "The key used to sign faucet transactions")
	cmd.Flags().Var(&flag.Account, "account", "The faucet account")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.Listen), "listen", "P2P listening address(es)")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.PromListen), "prom-listen", "Prometheus listening address(es) (default /ip4/0.0.0.0/tcp/8081/http)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.PeerDatabase, "peer-db", flag.PeerDatabase, "Track peers using a persistent database.")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flag("prom-listen").Changed {
			flag.PromListen = []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/0.0.0.0/tcp/8081/http")}
		}
		if !cmd.Flag("peer").Changed {
			flag.Peers = accumulate.BootstrapServers
		}
	}

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
		Instrumentation: &Instrumentation{
			HttpListener: HttpListener{
				Listen: flag.PromListen,
			},
		},
		P2P: &P2P{
			Key:                flag.NodeKey.Value,
			Listen:             flag.Listen,
			BootstrapPeers:     flag.Peers,
			PeerDB:             &flag.PeerDatabase,
			EnablePeerTracking: true,
		},
		Services: []Service{svcCfg},
	}

	ctx := cmdutil.ContextForMainProcess(context.Background())
	inst, err := Start(ctx, cfg)
	Check(err)
	<-ctx.Done()
	inst.Stop()
}
