// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	nodehttp "gitlab.com/accumulatenetwork/accumulate/internal/node/http"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdApi = &cobra.Command{
	Use:   "api",
	Short: "Standalone API",
	Run:   runApi,
}

var flagApi = struct {
	KeyFile   string
	LogLevel  string
	Peers     []string
	Timeout   time.Duration
	Listen    string
	ConnLimit int
}{}

func init() {
	cmdMain.AddCommand(cmdApi)

	cmdApi.Flags().StringVarP(&flagApi.KeyFile, "key-file", "k", "", "The key file for the API (leave blank to generate a key")
	cmdApi.Flags().StringVar(&flagApi.LogLevel, "log-level", "error", "Log level")
	cmdApi.Flags().StringSliceVar(&flagApi.Peers, "peers", nil, "The peers to connect to")
	cmdApi.Flags().DurationVar(&flagApi.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmdApi.Flags().StringVarP(&flagApi.Listen, "listen", "l", ":26660", "Address to listen on")
	cmdApi.Flags().IntVar(&flagApi.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")

	_ = cmdApi.MarkFlagRequired("peers")
}

func runApi(*cobra.Command, []string) {
	var key ed25519.PrivateKey
	if flagApi.KeyFile != "" {
		b, err := os.ReadFile(flagApi.KeyFile)
		check(err)
		block, _ := pem.Decode(b)
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		check(err)
		key = k.(ed25519.PrivateKey)
	}

	lw, err := logging.NewConsoleWriter("plain")
	check(err)
	ll, lw, err := logging.ParseLogLevel(flagApi.LogLevel, lw)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(lw), ll, false)
	check(err)

	var peers []multiaddr.Multiaddr
	for _, p := range flagApi.Peers {
		a, err := multiaddr.NewMultiaddr(p)
		check(err)
		peers = append(peers, a)
	}

	node, err := p2p.New(p2p.Options{
		Logger:         logger,
		BootstrapPeers: peers,
		Key:            key,
	})
	check(err)

	for i := 0; i < 10 && len(node.Peers()) == 0; i++ {
		time.Sleep(time.Second)
	}
	if len(node.Peers()) == 0 {
		fatalf("no peers")
	}

	client := &message.Client{
		Router: routerFunc(func(m message.Message) (multiaddr.Multiaddr, error) {
			return multiaddr.NewComponent("acc", protocol.Directory)
		}),
		Dialer: node.Dialer(),
	}
	ns, err := client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	check(err)
	router, err := routing.NewStaticRouter(ns.Routing, nil, logger)
	check(err)

	client.Router = routing.MessageRouter{Router: router}

	api, err := nodehttp.NewHandler(nodehttp.Options{
		Logger: logger,
		Node:   node,
		Router: router,
	})
	check(err)

	server := &http.Server{Handler: api, ReadHeaderTimeout: flagApi.Timeout}
	l, err := net.Listen("tcp", flagApi.Listen)
	check(err)
	fmt.Printf("Listening on %v\n", l.Addr())

	if flagApi.ConnLimit > 0 {
		pool := make(chan struct{}, flagApi.ConnLimit)
		for i := 0; i < flagApi.ConnLimit; i++ {
			pool <- struct{}{}
		}
		l = &accumulated.RateLimitedListener{Listener: l, Pool: pool}
	}

	check(server.Serve(l))
}

type routerFunc func(message.Message) (multiaddr.Multiaddr, error)

func (fn routerFunc) Route(msg message.Message) (multiaddr.Multiaddr, error) { return fn(msg) }

type roundTripperFunc func(ctx context.Context, requests []message.Message, callback func(res, req message.Message) error) error

func (fn roundTripperFunc) RoundTrip(ctx context.Context, requests []message.Message, callback func(res, req message.Message) error) error {
	return fn(ctx, requests, callback)
}
