// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
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
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHeal = &cobra.Command{
	Use: "heal",
}

var flagMaxResponseAge time.Duration = time.Minute

func init() {
	lightDb = filepath.Join(currentUser.HomeDir, ".accumulate", "cache", "light.db")

	cmd.AddCommand(cmdHeal)

	cmdHeal.PersistentFlags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	cmdHeal.PersistentFlags().BoolVarP(&pretend, "pretend", "n", false, "Do not submit envelopes, only scan")
	cmdHeal.PersistentFlags().BoolVar(&waitForTxn, "wait", false, "Wait for the message to finalize (defaults to true for heal synth)")
	cmdHeal.PersistentFlags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every minute")
	cmdHeal.PersistentFlags().StringVar(&peerDb, "peer-db", "", "Track peers using a persistent database")
	cmdHeal.PersistentFlags().StringVar(&pprof, "pprof", "", "Address to run net/http/pprof on")
	cmdHeal.PersistentFlags().BoolVar(&debug, "debug", false, "Debug network requests")
	cmdHealAnchor.PersistentFlags().DurationVar(&flagMaxResponseAge, "max-response-age", flagMaxResponseAge, "Maximum age of a response before it is considered too stale to use")
	cmdHealSynth.PersistentFlags().StringVar(&lightDb, "light-db", lightDb, "Light client database for persisting chain data")

	_ = cmdHeal.MarkFlagFilename("cached-scan", ".json")

	cmdHeal.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flag("peer-db").Changed {
			path := filepath.Join(".accumulate", "cache", strings.ToLower(args[0])+"-peers.json")
			peerDb = filepath.Join(currentUser.HomeDir, path)
			slog.Info("Automatically selected peer database", "path", filepath.Join("~", path))
		}
		if !cmd.Flag("cached-scan").Changed {
			path := filepath.Join(".accumulate", "cache", strings.ToLower(args[0])+".json")
			cachedScan = filepath.Join(currentUser.HomeDir, path)
			slog.Info("Automatically selected cached scan", "path", filepath.Join("~", path))
		}
		if f := cmd.Flag("light-db"); f != nil && !f.Changed {
			path := filepath.Join(".accumulate", "cache", strings.ToLower(args[0])+".db")
			lightDb = filepath.Join(currentUser.HomeDir, path)
			slog.Info("Automatically selected light database", "path", filepath.Join("~", path))
		}
	}
}

type healer struct {
	healing.Healer

	healSingle   func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID)
	healSequence func(h *healer, src, dst *protocol.PartitionInfo)

	ctx    context.Context
	C1     *jsonrpc.Client
	C2     *message.Client
	net    *healing.NetworkInfo
	light  *light.Client
	router routing.Router

	submit chan []messaging.Message

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

	if pprof != "" {
		l, err := net.Listen("tcp", pprof)
		check(err)
		s := new(http.Server)
		s.ReadHeaderTimeout = time.Minute
		go func() { check(s.Serve(l)) }()
		go func() { <-ctx.Done(); _ = s.Shutdown(context.Background()) }()
	}

	// We should be able to use only the p2p client but it doesn't work well for
	// some reason
	h.C1 = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	h.C1.Client.Timeout = time.Hour
	h.C1.Debug = debug

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

	h.router, err = apiutil.InitRouter(apiutil.RouterOptions{
		Context: ctx,
		Node:    node,
		Network: args[0],
	})
	check(err)

	ok := <-h.router.(*routing.RouterInstance).Ready()
	if !ok {
		fatalf("railed to initialize router")
	}

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
			Router:  routing.MessageRouter{Router: h.router},
			Debug:   debug,
		},
	}

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	h.submit = make(chan []messaging.Message)
	wg.Add(1)
	go h.submitLoop(wg)

	if lightDb != "" {
		cv2, err := client.New(accumulate.ResolveWellKnownEndpoint(args[0], "v2"))
		check(err)
		cv2.DebugRequest = debug

		db, err := bolt.Open(lightDb, bolt.WithPlainKeys)
		check(err)

		h.light, err = light.NewClient(
			light.Store(db, ""),
			light.ClientV2(cv2),
			light.Querier(h.C2),
			light.Router(h.router),
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

func (h *healer) submitLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()

	var messages []messaging.Message
	var stop bool
	for !stop {
		select {
		case <-h.ctx.Done():
			stop = true
		case msg := <-h.submit:
			messages = append(messages, msg...)
			if len(messages) < 50 {
				continue
			}
		case <-t.C:
		}
		if len(messages) == 0 {
			continue
		}

		env := &messaging.Envelope{Messages: messages}
		subs, err := h.C2.Submit(h.ctx, env, api.SubmitOptions{})
		messages = messages[:0]
		if err != nil {
			slog.ErrorContext(h.ctx, "Submission failed", "error", err, "id", env.Messages[0].ID())
		}
		for _, sub := range subs {
			if sub.Success {
				slog.InfoContext(h.ctx, "Submission succeeded", "id", sub.Status.TxID)
			} else {
				slog.ErrorContext(h.ctx, "Submission failed", "message", sub, "status", sub.Status)
			}
		}
	}
}

// getAccount fetches the given account.
func getAccount[T protocol.Account](ctx context.Context, q api.Querier, u *url.URL) T {
	r, err := api.Querier2{Querier: q}.QueryAccount(ctx, u, nil)
	checkf(err, "get %v", u)

	if r.LastBlockTime == nil {
		fatalf("response for %v does not include a last block time", u)
	}

	age := time.Since(*r.LastBlockTime)
	if flagMaxResponseAge > 0 && age > flagMaxResponseAge {
		fatalf("response for %v is too old (%v)", u, age)
	}

	slog.InfoContext(ctx, "Got account", "url", u, "lastBlockAge", age.Round(time.Second))

	a := r.Account
	b, ok := a.(T)
	if !ok {
		fatalf("%v is a %T not a %v", u, a, reflect.TypeOf(new(T)).Elem())
	}
	return b
}

func (h *healer) tryEach() api.Querier2 {
	return api.Querier2{Querier: &tryEachQuerier{h, 10 * time.Second}}
}

type tryEachQuerier struct {
	*healer
	timeout time.Duration
}

func (q *tryEachQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	part, err := q.router.RouteAccount(scope)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for peer, info := range q.net.Peers[strings.ToLower(part)] {
		c := q.C2.ForPeer(peer)
		if len(info.Addresses) > 0 {
			c = c.ForAddress(info.Addresses[0])
		}

		ctx, cancel := context.WithTimeout(ctx, q.timeout)
		defer cancel()

		r, err := c.Query(ctx, scope, query)
		if err == nil {
			return r, nil
		}
		if errors.Code(err).IsClientError() {
			return nil, err
		}
		lastErr = err
		slog.ErrorContext(ctx, "Failed to query", "peer", peer, "scope", scope, "error", err)
	}
	return nil, lastErr
}
