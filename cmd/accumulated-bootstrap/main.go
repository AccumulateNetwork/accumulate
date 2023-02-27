// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"os/signal"
	"strings"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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
	LogLevel string
	Listen   []multiaddr.Multiaddr
	Peers    []multiaddr.Multiaddr
}{}

func init() {
	cmd.Flags().StringVar(&flag.Key, "key", "", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrFlag)(&flag.Listen), "listen", "l", "Listening address")
	cmd.Flags().VarP((*MultiaddrFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")
}

func run(*cobra.Command, []string) {
	w, err := logging.NewConsoleWriter("plain")
	Check(err)
	level, w, err := logging.ParseLogLevel(flag.LogLevel, w)
	Check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(w), level, false)
	Check(err)

	node, err := p2p.New(p2p.Options{
		Key:            loadOrGenerateKey(),
		Logger:         logger,
		Listen:         flag.Listen,
		BootstrapPeers: flag.Peers,
		DiscoveryMode:  dht.ModeAutoServer,
	})
	Check(err)
	defer func() { _ = node.Close() }()

	fmt.Println("We are")
	for _, a := range node.Addrs() {
		fmt.Printf("  %s\n", a)
	}
	fmt.Println()

	// Wait for SIGINT
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	<-sigs
}

func loadOrGenerateKey() ed25519.PrivateKey {
	if strings.HasPrefix(flag.Key, "seed:") {
		Warnf("Generating a new key from a seed. This is not at all secure.")
		h := sha256.Sum256([]byte(flag.Key))
		return ed25519.NewKeyFromSeed(h[:])
	}

	if flag.Key != "" {
		return LoadKey(flag.Key)
	}

	// Generate a key if necessary
	Warnf("Generating a new key. This is highly discouraged for permanent infrastructure.")
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	Checkf(err, "generate key")
	return sk
}
