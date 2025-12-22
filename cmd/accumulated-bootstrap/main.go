// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "accumulated-bootstrap",
	Short: "Accumulate network bootstrap node",
	Run:   run,
	Args:  cobra.NoArgs,
}

var flag = struct {
	Key        PrivateKeyFlag
	Listen     []multiaddr.Multiaddr
	PromListen []multiaddr.Multiaddr
	InfoListen multiaddr.Multiaddr
	Peers      []multiaddr.Multiaddr
	External   []multiaddr.Multiaddr
}{
	Key: PrivateKeyFlag{Value: &TransientPrivateKey{}},
}

func init() {
	cmd.Flags().Var(&flag.Key, "key", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Listen), "listen", "l", "Listening address")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.PromListen), "prom-listen", "Prometheus listening address(es) (default /ip4/0.0.0.0/tcp/8081/http)")
	cmd.Flags().Var(MultiaddrFlag{Value: &flag.InfoListen}, "info-listen", "Info server listening address (default /ip4/0.0.0.0/tcp/8080/http)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.External), "external", "External address(es) to advertize")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flag("prom-listen").Changed {
			flag.PromListen = []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/0.0.0.0/tcp/8081/http")}
		}
		if !cmd.Flag("info-listen").Changed {
			flag.InfoListen = multiaddr.StringCast("/ip4/0.0.0.0/tcp/8080/http")
		}
	}
}

func run(*cobra.Command, []string) {
	// Use first external address for P2P config, or nil if none provided
	var external multiaddr.Multiaddr
	if len(flag.External) > 0 {
		external = flag.External[0]
	}

	cfg := &Config{
		Instrumentation: &Instrumentation{
			HttpListener: HttpListener{
				Listen: flag.PromListen,
			},
		},
		P2P: &P2P{
			Key:            flag.Key.Value,
			Listen:         flag.Listen,
			BootstrapPeers: flag.Peers,
			DiscoveryMode:  Ptr(DhtMode(dht.ModeAutoServer)),
			External:       external,
		},
	}

	ctx := ContextForMainProcess(context.Background())
	inst, err := Start(ctx, cfg)
	Check(err)

	// Start info server on port 8080 if configured
	if flag.InfoListen != nil {
		infoServer, err := NewInfoServer(inst.P2P().Host(), flag.InfoListen, flag.External)
		if err != nil {
			inst.Stop()
			Checkf(err, "failed to start info server")
		}

		// Register shutdown handler via internal method access
		// Note: cleanup is a private method, so we'll handle shutdown differently
		defer func() {
			if infoServer != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = infoServer.Shutdown(shutdownCtx)
			}
		}()
	}

	<-inst.Done()
	inst.Stop()
}
