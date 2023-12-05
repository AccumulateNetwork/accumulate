// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/light"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHeal = &cobra.Command{
	Use: "heal",
}

func init() {
	peerDb = filepath.Join(currentUser.HomeDir, ".accumulate", "cache", "peerdb.json")
	lightDb = filepath.Join(currentUser.HomeDir, ".accumulate", "cache", "light.db")

	cmd.AddCommand(cmdHeal)

	cmdHeal.PersistentFlags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	cmdHeal.PersistentFlags().BoolVarP(&pretend, "pretend", "n", false, "Do not submit envelopes, only scan")
	cmdHeal.PersistentFlags().BoolVar(&waitForTxn, "wait", false, "Wait for the message to finalize (defaults to true for heal synth)")
	cmdHeal.PersistentFlags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every minute")
	cmdHeal.PersistentFlags().StringVar(&peerDb, "peer-db", peerDb, "Track peers using a persistent database")
	cmdHealSynth.PersistentFlags().StringVar(&lightDb, "light-db", lightDb, "Light client database for persisting chain data")

	_ = cmdHeal.MarkFlagFilename("cached-scan", ".json")
}

type healer struct {
	healing.Healer

	healSingle   func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID)
	healSequence func(h *healer, src, dst *protocol.PartitionInfo)

	ctx   context.Context
	C1    *jsonrpc.Client
	C2    *message.Client
	net   *healing.NetworkInfo
	light *light.Client

	accounts map[[32]byte]protocol.Account
}

func (h *healer) Reset() {
	h.Healer.Reset()
	h.accounts = map[[32]byte]protocol.Account{}
}

func (h *healer) heal(args []string) {
	ctx := cmdutil.ContextForMainProcess(context.Background())
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()
	h.ctx = ctx
	h.accounts = map[[32]byte]protocol.Account{}

	// We should be able to use only the p2p client but it doesn't work well for
	// some reason
	h.C1 = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	h.C1.Client.Timeout = time.Hour

	ni, err := h.C1.NodeInfo(ctx, api.NodeInfoOptions{})
	check(err)

	node, err := p2p.New(p2p.Options{
		Network:           ni.Network,
		BootstrapPeers:    accumulate.BootstrapServers,
		PeerDatabase:      peerDb,
		EnablePeerTracker: true,

		// Use the peer tracker, but don't update it between reboots
		PeerScanFrequency:    -1,
		PeerPersistFrequency: -1,
	})
	checkf(err, "start p2p node")
	defer func() { _ = node.Close() }()

	fmt.Fprintf(os.Stderr, "We are %v\n", node.ID())

	if cachedScan != "" {
		data, err := os.ReadFile(cachedScan)
		check(err)
		check(json.Unmarshal(data, &h.net))
	}

	router, err := apiutil.InitRouter(ctx, node, args[0])
	check(err)

	dialer := node.DialNetwork()
	if _, ok := node.Tracker().(*dial.PersistentTracker); !ok {
		// Use a hack dialer that uses the API for peer discovery
		dialer = &apiutil.StaticDialer{
			Scan:   h.net,
			Nodes:  h.C1,
			Dialer: dialer,
		}
	}

	h.C2 = &message.Client{
		Transport: &message.RoutedTransport{
			Network: ni.Network,
			Dialer:  dialer,
			Router:  routing.MessageRouter{Router: router},
		},
	}

	if lightDb != "" {
		db, err := bolt.Open(lightDb, bolt.WithPlainKeys)
		check(err)
		h.light, err = light.NewClient(
			light.Store(db, ""),
			light.Server(accumulate.ResolveWellKnownEndpoint(args[0], "v2")),
			light.Querier(h.C2),
			light.Router(router),
		)
		check(err)
		defer func() { _ = h.light.Close() }()
	}

	if cachedScan == "" {
		h.net, err = healing.ScanNetwork(ctx, h.C2)
		check(err)
	}

	// Heal all partitions
	if len(args) < 2 {
	heal:
		for _, src := range h.net.Status.Network.Partitions {
			for _, dst := range h.net.Status.Network.Partitions {
				h.healSequence(h, src, dst)
			}
		}

		// Heal continuously?
		if healContinuous {
			time.Sleep(time.Minute)
			h.Reset()
			goto heal
		}
		return
	}

	parts := map[string]*protocol.PartitionInfo{}
	for _, p := range h.net.Status.Network.Partitions {
		parts[strings.ToLower(p.ID)] = p
	}

	// Heal a specific transaction
	txid, err := url.ParseTxID(args[1])
	if err == nil {
		r, err := api.Querier2{Querier: h.C2}.QueryTransaction(ctx, txid, nil)
		check(err)
		if r.Sequence == nil {
			fatalf("%v is not sequenced", txid)
		}
		srcId, _ := protocol.ParsePartitionUrl(r.Sequence.Source)
		dstId, _ := protocol.ParsePartitionUrl(r.Sequence.Destination)
		h.healSingle(h, parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)], r.Sequence.Number, txid)
		return
	}

	// Heal all message from a given partition
	if src, ok := parts[strings.ToLower(args[1])]; ok {
		for _, dst := range h.net.Status.Network.Partitions {
			h.healSequence(h, src, dst)
		}
		return
	}

	// Heal a specific sequence
	s := strings.Split(args[1], "→")
	if len(s) != 2 {
		fatalf("invalid transaction ID or sequence specifier: %q", args[1])
	}
	srcId, dstId := strings.TrimSpace(s[0]), strings.TrimSpace(s[1])

	// Heal the entire sequence
	if len(args) < 3 {
		h.healSequence(h, parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)])
		return
	}

	// Heal a specific entry
	seqNo, err := strconv.ParseUint(args[2], 10, 64)
	check(err)
	h.healSingle(h, parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)], seqNo, nil)
}

// getAccount fetches the given account.
func getAccount[T protocol.Account](h *healer, u *url.URL) T {
	// Fetch and cache the account
	a, ok := h.accounts[u.AccountID32()]
	if !ok {
		r, err := api.Querier2{Querier: h.C1}.QueryAccount(h.ctx, u, nil)
		checkf(err, "get %v", u)
		a = r.Account
		h.accounts[u.AccountID32()] = a
	}

	b, ok := a.(T)
	if !ok {
		fatalf("%v is a %T not a %v", u, a, reflect.TypeOf(new(T)).Elem())
	}
	return b
}
