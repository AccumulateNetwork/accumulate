// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"strings"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
	Key      string
	Listen   []multiaddr.Multiaddr
	Peers    []multiaddr.Multiaddr
	External multiaddr.Multiaddr
}{}

func init() {
	cmd.Flags().StringVar(&flag.Key, "key", "", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Listen), "listen", "l", "Listening address")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().Var(MultiaddrFlag{Value: &flag.External}, "external", "External address to advertize")
}

func run(*cobra.Command, []string) {
	cfg := &Config{
		P2P: &P2P{
			Key:            loadOrGenerateKey(),
			Listen:         flag.Listen,
			BootstrapPeers: flag.Peers,
			DiscoveryMode:  DhtMode(dht.ModeAutoServer),
			External:       flag.External,
		},
	}

	ctx := cmdutil.ContextForMainProcess(context.Background())
	inst, err := Start(ctx, cfg)
	Check(err)

	<-ctx.Done()
	Check(inst.Stop())
}

func loadOrGenerateKey() PrivateKey {
	if flag.Key == "" {
		return &TransientPrivateKey{}
	}

	if strings.HasPrefix(flag.Key, "seed:") {
		return &PrivateKeySeed{Seed: record.NewKey(flag.Key[5:])}
	}

	sk := LoadKey(flag.Key)
	addr := &address.PrivateKey{
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  sk[32:],
		},
		Key: sk,
	}
	return &RawPrivateKey{Address: addr.String()}
}
