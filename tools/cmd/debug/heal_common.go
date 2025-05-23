// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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

	"github.com/fatih/color"
	"github.com/lmittmann/tint"
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
	cmd.AddCommand(cmdHeal)

	healerFlags(cmdHeal)
	cmdHeal.PersistentFlags().BoolVarP(&pretend, "pretend", "n", false, "Do not submit envelopes, only scan")
	cmdHeal.PersistentFlags().BoolVar(&waitForTxn, "wait", false, "Wait for the message to finalize (defaults to true for heal synth)")
	cmdHeal.PersistentFlags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every minute")
	cmdHeal.PersistentFlags().StringVar(&pprof, "pprof", "", "Address to run net/http/pprof on")
	cmdHealSynth.PersistentFlags().StringVar(&lightDb, "light-db", lightDb, "Light client database for persisting chain data")

	_ = cmdHealSynth.MarkFlagFilename("light-db", ".db")
}

func healerFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	cmd.PersistentFlags().StringVar(&peerDb, "peer-db", "", "Track peers using a persistent database")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "Debug network requests")
	cmd.PersistentFlags().DurationVar(&flagMaxResponseAge, "max-response-age", flagMaxResponseAge, "Maximum age of a response before it is considered too stale to use")

	_ = cmd.MarkFlagFilename("cached-scan", ".json")
	_ = cmd.MarkFlagFilename("peer-db", ".json")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flag("peer-db").Changed {
			peerDb = filepath.Join(cacheDir, strings.ToLower(args[0])+"-peers.json")
		}
		if f := cmd.Flag("light-db"); f != nil && !f.Changed {
			lightDb = filepath.Join(cacheDir, strings.ToLower(args[0])+".db")
		}
	}
}

type healer struct {
	healing.Healer

	healSingle   func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID)
	healSequence func(h *healer, src, dst *protocol.PartitionInfo)

	network string
	ctx     context.Context
	C1      *jsonrpc.Client
	C2      *message.Client
	net     *healing.NetworkInfo
	light   *light.Client
	router  routing.Router

	submit chan []messaging.Message

	accounts map[[32]byte]protocol.Account
}

func (h *healer) Reset() {
	h.Healer.Reset()
	h.accounts = map[[32]byte]protocol.Account{}
}

func (h *healer) setup(ctx context.Context, network string) {
	// Color the logs
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{Level: slog.LevelInfo})))

	h.ctx = ctx
	h.network = network
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
	h.C1 = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	h.C1.Client.Timeout = time.Hour
	h.C1.Debug = debug

	ni, err := h.C1.NodeInfo(ctx, api.NodeInfoOptions{})
	check(err)

	node, err := p2p.New(p2p.Options{
		Network:           ni.Network,
		BootstrapPeers:    bootstrap,
		PeerDatabase:      peerDb,
		EnablePeerTracker: true,

		// Use the peer tracker, but don't update it between reboots
		PeerScanFrequency:    -1,
		PeerPersistFrequency: -1,
	})
	checkf(err, "start p2p node")
	go func() { <-ctx.Done(); _ = node.Close() }()

	fmt.Fprintf(os.Stderr, "We are %v\n", node.ID())

	h.router, err = apiutil.InitRouter(apiutil.RouterOptions{
		Context: ctx,
		Node:    node,
		Network: network,
	})
	check(err)

	ok := <-h.router.(*routing.RouterInstance).Ready()
	if !ok {
		fatalf("railed to initialize router")
	}

	// The client must be initialized before we load the network status since
	// that will query the network
	transport := &message.RoutedTransport{
		Network: ni.Network,
		Dialer:  node.DialNetwork(),
		Router:  routing.MessageRouter{Router: h.router},
		Debug:   debug,
	}
	h.C2 = &message.Client{
		Transport: transport,
	}

	h.loadNetworkStatus()

	// Use a hack dialer that uses the API for peer discovery if the peer
	// tracker is not persistent. This has to be done after loading the network
	// status since it requires that scan.
	if _, ok := node.Tracker().(*dial.PersistentTracker); !ok {
		transport.Dialer = &apiutil.StaticDialer{
			Scan:   h.net,
			Nodes:  h.C1,
			Dialer: transport.Dialer,
		}
	}

	if lightDb != "" {
		cv2, err := client.New(accumulate.ResolveWellKnownEndpoint(network, "v2"))
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
		go func() { <-ctx.Done(); _ = h.light.Close() }()
	}
}

func (h *healer) loadNetworkStatus() {
	if cachedScan == "" {
		f := filepath.Join(cacheDir, strings.ToLower(h.network)+".json")
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			slog.Info("Detected network scan", "file", f)
			cachedScan = f
		} else if !errors.Is(err, fs.ErrNotExist) {
			check(err)
		} else {
			h.net, err = healing.ScanNetwork(h.ctx, h.C2)
			check(err)
			return
		}
	}

	slog.Info("Loading cached network scan")
	data, err := os.ReadFile(cachedScan)
	check(err)
	check(json.Unmarshal(data, &h.net))

	// Get the current state of the network
	ns, err := h.C2.NetworkStatus(h.ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)
	if h.net.Status.Equal(ns) {
		return
	}

	// Rescan if the validator set has changed
	if false && !h.net.Status.Network.Equal(ns.Network) {
		slog.Info("Validator set has changed since last scan")
		h.net, err = healing.ScanNetwork(h.ctx, h.C2)
		check(err)
	} else {
		h.net.Status = ns
	}

	// Update the cached file if anything has changed
	data, err = json.MarshalIndent(&h.net, "", "  ")
	check(err)
	check(os.WriteFile(cachedScan, data, 0644))
}

func (h *healer) heal(args []string) {
	// Wait must be called after cancel (so the defer must happen first)
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	ctx := cmdutil.ContextForMainProcess(context.Background())
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	h.setup(ctx, args[0])

	h.submit = make(chan []messaging.Message)
	wg.Add(1)
	go h.submitLoop(wg)

	// Heal all partitions
	if len(args) < 2 {
	heal:
		for _, src := range h.net.Status.Network.Partitions {
			for _, dst := range h.net.Status.Network.Partitions {
				// Stopped?
				select {
				default:
				case <-ctx.Done():
					return
				}

				h.healSequence(h, src, dst)
			}
		}

		// Heal continuously?
		if healContinuous {
			color.Yellow("Healing complete, sleeping for a minute")
			ctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			<-ctx.Done()
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
			if len(messages) < 10 {
				continue
			}
		case <-t.C:
		}
		if len(messages) == 0 {
			continue
		}
		part := map[[32]byte]*url.URL{}
		block := map[[32]byte]time.Time{}
		for _, msg := range messages {
			var server = "http://apollo-mainnet.accumulate.defidevs.io:16595/v3"
			partUrl := protocol.DnUrl()
			if msg, ok := msg.(*messaging.BlockAnchor); ok {

				bvn, _ := protocol.ParsePartitionUrl(msg.Anchor.ID().Account())
				if !strings.EqualFold(bvn, protocol.Directory) {
					server = fmt.Sprintf("http://%s-mainnet.accumulate.defidevs.io:16695/v3", strings.ToLower(bvn))
					partUrl = msg.Anchor.ID().Account().RootIdentity()
				}
			}
			part[partUrl.AccountID32()] = partUrl

			fmt.Println("Sending to", server)
			c := jsonrpc.NewClient(server)

			// What block is this partition on?
			if _, ok := block[partUrl.AccountID32()]; !ok {
				r, err := h.C1.Query(context.Background(), partUrl, &api.DefaultQuery{})
				if err != nil {
					continue
				}
				a, ok := r.(*api.AccountRecord)
				if !ok || a.LastBlockTime == nil {
					continue
				}
				block[partUrl.AccountID32()] = *a.LastBlockTime
			}

			env := &messaging.Envelope{Messages: []messaging.Message{msg}}
			subs, err := c.Submit(context.Background(), env, api.SubmitOptions{})
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

		messages = messages[:0]

		// Wait for a new block
		for hash, last := range block {
			for {
				r, err := h.C1.Query(context.Background(), part[hash], &api.DefaultQuery{})
				if err != nil {
					break
				}
				a, ok := r.(*api.AccountRecord)
				if !ok || a.LastBlockTime == nil {
					break
				}
				if a.LastBlockTime.After(last) {
					break
				}
				slog.InfoContext(h.ctx, "Waiting for a new block", "partition", part[hash])
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// getAccount fetches the given account.
func getAccount[T protocol.Account](h *healer, u *url.URL) T {
	r, err := h.tryEach().QueryAccount(h.ctx, u, nil)
	checkf(err, "get %v", u)

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
		if err != nil {
			if errors.Code(err).IsClientError() {
				return nil, err
			}
			lastErr = err
			slog.ErrorContext(ctx, "Failed to query", "peer", peer, "scope", scope, "error", err)
			continue
		}

		r2, ok := r.(api.WithLastBlockTime)
		if !ok {
			return r, nil
		}

		if r2.GetLastBlockTime() == nil {
			cmdutil.Warnf("response for %v does not include a last block time", scope)
			continue
		}

		age := time.Since(*r2.GetLastBlockTime())
		if flagMaxResponseAge > 0 && age > flagMaxResponseAge {
			cmdutil.Warnf("response for %v is too old (%v)", scope, age)
			continue
		}

		return r, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("unable to query %v (ran out of peers to try)", scope)
	}
	return nil, lastErr
}
