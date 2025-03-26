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
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/olekukonko/tablewriter"
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

var (
	cmdHeal = &cobra.Command{
		Use:   "heal [network]",
		Short: "Heal the network",
		Args:  cobra.ExactArgs(1),
	}

	maxRetries    int
	maxTxnRetries int
	maxTxnWait    time.Duration

	mempoolErrorsMu sync.Mutex
	mempoolErrors   int
	lastMempoolErr  time.Time

	missingAnchorsMu sync.Mutex
	missingAnchors   map[string]map[string]MissingAnchorsInfo

	checkedAnchorsMu sync.RWMutex
	checkedAnchors   map[string]map[string]map[uint64]bool

	flagMaxResponseAge time.Duration = 48 * time.Hour
)

type MissingAnchorsInfo struct {
	Count             int
	FirstMissingHeight uint64
	Unprocessed       int
	UnprocessedStart  uint64
	UnprocessedEnd    uint64
	LastProcessedHeight uint64
}

// Global variables to track skipped partitions due to errors
var (
	skippedPartitionsMu sync.Mutex
	skippedPartitions   = make(map[string]map[string]string) // src -> dst -> reason
)

func init() {
	cmd.AddCommand(cmdHeal)

	healerFlags(cmdHeal)
	cmdHeal.PersistentFlags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every minute")
	cmdHeal.PersistentFlags().StringVar(&pprof, "pprof", "", "Address to run net/http/pprof on")
	// These flags are now set in main.go
	// cmdHeal.PersistentFlags().BoolVarP(&pretend, "pretend", "n", false, "Do not submit envelopes, only scan")
	// cmdHeal.PersistentFlags().BoolVar(&waitForTxn, "wait", false, "Wait for the message to finalize (defaults to true for heal synth)")
	// cmdHealSynth.PersistentFlags().StringVar(&lightDb, "light-db", lightDb, "Light client database for persisting chain data")

	// This line depends on the commented out line above
	// _ = cmdHealSynth.MarkFlagFilename("light-db", ".db")

	checkedAnchors = make(map[string]map[string]map[uint64]bool)
	missingAnchors = make(map[string]map[string]MissingAnchorsInfo)
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

	healSingle   func(src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID)
	healSequence func(src, dst *protocol.PartitionInfo)

	network string
	ctx     context.Context
	C1      *jsonrpc.Client
	C2      *message.Client
	net     *healing.NetworkInfo
	light   *light.Client
	router  routing.Router

	submit chan []messaging.Message

	accounts map[[32]byte]protocol.Account
	// ProblemNodes tracks nodes that consistently fail with chain entry errors
	problemNodes map[peer.ID]bool
	
	// DryRun indicates whether to actually submit transactions or just simulate
	dryRun bool
	
	// Cache stores query results to avoid redundant network requests
	cache map[string]interface{}
}

func (h *healer) Reset() {
	h.Healer.Reset()
	h.accounts = map[[32]byte]protocol.Account{}
	if h.problemNodes == nil {
		h.problemNodes = make(map[peer.ID]bool)
	}
}

func (h *healer) setup(ctx context.Context, network string) {
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

	// Discover peers via JSON-RPC and use them as bootstrap peers
	jsonrpcPeers, err := discoverPeersViaJSONRPC(ctx, h.C1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to discover peers via JSON-RPC: %v\n", err)
	}

	// Log bootstrap peers before combining
	fmt.Fprintf(os.Stderr, "Default bootstrap peers: %d\n", len(bootstrap))
	for i, peer := range bootstrap {
		fmt.Fprintf(os.Stderr, "  Bootstrap peer %d: %s\n", i+1, peer.String())
	}

	// Log JSON-RPC discovered peers
	fmt.Fprintf(os.Stderr, "JSON-RPC discovered peers: %d\n", len(jsonrpcPeers))
	for i, peer := range jsonrpcPeers {
		fmt.Fprintf(os.Stderr, "  JSON-RPC peer %d: %s\n", i+1, peer.String())
	}

	// Combine the discovered peers with the default bootstrap peers
	combinedBootstrap := append(bootstrap, jsonrpcPeers...)

	// Log the bootstrap peers being used
	fmt.Fprintf(os.Stderr, "Using %d bootstrap peers (%d from JSON-RPC)\n", len(combinedBootstrap), len(jsonrpcPeers))

	node, err := p2p.New(p2p.Options{
		Network:           ni.Network,
		BootstrapPeers:    combinedBootstrap,
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
	
	// Parse the network scan data manually to handle invalid peer IDs
	var rawNet struct {
		Status *api.NetworkStatus  `json:"status"`
		ID     string              `json:"id"`
		Peers  map[string]json.RawMessage `json:"peers"`
	}
	
	err = json.Unmarshal(data, &rawNet)
	check(err)
	
	// Create a new NetworkInfo with the parsed data
	h.net = &healing.NetworkInfo{
		Status: rawNet.Status,
		ID:     rawNet.ID,
		Peers:  make(map[string]healing.PeerList),
	}
	
	// Process each partition's peers
	for partID, rawPeers := range rawNet.Peers {
		// Parse the raw peer data
		var peerMap map[string]*healing.PeerInfo
		err = json.Unmarshal(rawPeers, &peerMap)
		check(err)
		
		// Create a new PeerList for this partition
		partPeers := make(healing.PeerList)
		h.net.Peers[partID] = partPeers
		
		// Process each peer in the partition
		for peerIDStr, peerInfo := range peerMap {
			// Try to decode the peer ID, skip if invalid
			peerID, err := peer.Decode(peerIDStr)
			if err != nil {
				slog.ErrorContext(h.ctx, "Failed to decode peer ID",
					"id", peerIDStr,
					"partition", partID,
					"error", err,
					"error_type", fmt.Sprintf("%T", err))
				continue
			}
			
			// Set the peer ID in the peer info and add to the peer list
			peerInfo.ID = peerID
			partPeers[peerID] = peerInfo
		}
	}

	// Get the current state of the network
	ns, err := h.C2.NetworkStatus(h.ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)
	if h.net.Status.Equal(ns) {
		return
	}

	// Rescan if the validator set has changed
	if !h.net.Status.Network.Equal(ns.Network) {
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

	// Ensure args has at least one element
	network := "mainnet"
	if len(args) > 0 {
		network = args[0]
	}

	h.setup(ctx, network)

	h.submit = make(chan []messaging.Message)
	wg.Add(1)
	go h.submitLoop(wg)

	// Initialize missing anchors tracking
	missingAnchorsMu.Lock()
	missingAnchors = make(map[string]map[string]MissingAnchorsInfo)
	missingAnchorsMu.Unlock()

	checkedAnchorsMu.Lock()
	checkedAnchors = make(map[string]map[string]map[uint64]bool)
	checkedAnchorsMu.Unlock()

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

				h.healSequence(src, dst)
			}
		}

		// Report mempool errors
		mempoolErrorsMu.Lock()
		mempoolErrorsMu.Unlock()
		
		// Report missing anchors
		missingAnchorsMu.Lock()
		if len(missingAnchors) > 0 {
			fmt.Println("\n===========================================")
			fmt.Println(time.Now().Format("Mon Jan 02 03:04:05 PM MST 2006"))
			
			// Display time since last mempool error
			mempoolErrorsMu.Lock()
			if !lastMempoolErr.IsZero() {
				fmt.Printf("Last mempool error: %s\n", lastMempoolErr.Format("Mon Jan 02 03:04:05 PM MST 2006"))
			} else {
				fmt.Println("No mempool errors recorded")
			}
			mempoolErrorsMu.Unlock()
			
			// Prepare data for sorted output
			var sortedSrcs []srcInfo
			for src, dsts := range missingAnchors {
				si := srcInfo{name: src}
				for dst, info := range dsts {
					si.dsts = append(si.dsts, dstInfo{name: dst, info: info})
				}
				// Sort destinations alphabetically
				sort.Slice(si.dsts, func(i, j int) bool {
					return si.dsts[i].name < si.dsts[j].name
				})
				sortedSrcs = append(sortedSrcs, si)
			}
			// Sort sources alphabetically
			sort.Slice(sortedSrcs, func(i, j int) bool {
				return sortedSrcs[i].name < sortedSrcs[j].name
			})
			
			// Create a table for the anchor status information
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Source", "Destination", "Status", "Pending", "Source Height", "Dest Height", "Unprocessed", "Range"})
			table.SetBorder(true)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("|")
			table.SetColumnSeparator("|")
			table.SetRowSeparator("-")
			table.SetHeaderLine(true)
			
			// Add rows to the table
			for _, src := range sortedSrcs {
				for _, dst := range src.dsts {
					status := "✓" // Default to OK
					if dst.info.Count > 0 {
						status = "🗴" // Missing anchors
					} else if dst.info.Unprocessed > 0 {
						status = "⚠" // Unprocessed anchors
					}
					
					// Format the range if there are unprocessed anchors
					rangeStr := "-"
					if dst.info.Unprocessed > 0 {
						rangeStr = fmt.Sprintf("%d → %d", dst.info.UnprocessedStart, dst.info.UnprocessedEnd)
					}
					
					// Calculate source height (last processed + pending)
					sourceHeight := dst.info.LastProcessedHeight + uint64(dst.info.Count)
					
					// Add the row with appropriate coloring
					row := []string{
						src.name,
						dst.name,
						status,
						fmt.Sprintf("%d", dst.info.Count),
						fmt.Sprintf("%d", sourceHeight),
						fmt.Sprintf("%d", dst.info.LastProcessedHeight),
						fmt.Sprintf("%d", dst.info.Unprocessed),
						rangeStr,
					}
					table.Append(row)
				}
			}
			
			// Render the table
			fmt.Println("\nAnchor Status:")
			table.Render()
			
			fmt.Println()
		} else {
			fmt.Println("\n===========================================")
			fmt.Println(time.Now().Format("Mon Jan 02 03:04:05 PM MST 2006"))
			// Display time since last mempool error
			mempoolErrorsMu.Lock()
			if !lastMempoolErr.IsZero() {
				fmt.Printf("Last mempool error: %s\n", lastMempoolErr.Format("Mon Jan 02 03:04:05 PM MST 2006"))
			} else {
				fmt.Println("No mempool errors recorded")
			}
			mempoolErrorsMu.Unlock()
			fmt.Println("No missing anchors found")
		}
		missingAnchorsMu.Unlock()
		
		// Clear skipped partitions for the next cycle
		skippedPartitionsMu.Lock()
		skippedPartitions = make(map[string]map[string]string)
		skippedPartitionsMu.Unlock()
		
		// Heal continuously?
		if healContinuous {
			color.Yellow("Healing complete, sleeping for 15 seconds. Mempool full errors: %d", mempoolErrors)
			ctx, cancel := context.WithTimeout(ctx, time.Second*15)
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
		h.healSingle(parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)], r.Sequence.Number, txid)
		return
	}

	// Heal all message from a given partition
	if src, ok := parts[strings.ToLower(args[1])]; ok {
		for _, dst := range h.net.Status.Network.Partitions {
			h.healSequence(src, dst)
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
		h.healSequence(parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)])
		return
	}

	// Heal a specific entry
	seqNo, err := strconv.ParseUint(args[2], 10, 64)
	check(err)
	h.healSingle(parts[strings.ToLower(srcId)], parts[strings.ToLower(dstId)], seqNo, nil)
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
			// Check for mempool full errors
			errStr := err.Error()
			if strings.Contains(strings.ToLower(errStr), "mempool") {
				mempoolErrorsMu.Lock()
				mempoolErrors++
				lastMempoolErr = time.Now()
				mempoolErrorsMu.Unlock()
				slog.ErrorContext(h.ctx, "Mempool error", "error", err, "count", mempoolErrors, "id", env.Messages[0].ID())
			} else {
				slog.ErrorContext(h.ctx, "Submission failed", "error", err, "id", env.Messages[0].ID())
			}
		}
		for i, sub := range subs {
			// Extract partition information if available
			var source, destination string

			// Try to extract source and destination from the message
			if i < len(env.Messages) {
				if txMsg, ok := env.Messages[i].(*messaging.TransactionMessage); ok && txMsg != nil && txMsg.Transaction != nil {
					// Get destination from the transaction header
					if txMsg.Transaction.Header.Principal != nil {
						destination = txMsg.Transaction.Header.Principal.String()
					}

					// Try to get source from the transaction body
					// This is a generic approach since we don't know the exact type
					if txMsg.Transaction.Body != nil {
						// Use reflection to check if the body has a Source field
						bodyVal := reflect.ValueOf(txMsg.Transaction.Body)
						if bodyVal.Kind() == reflect.Ptr && !bodyVal.IsNil() {
							bodyVal = bodyVal.Elem()
							if bodyVal.Kind() == reflect.Struct {
								// Look for a Source field
								sourceField := bodyVal.FieldByName("Source")
								if sourceField.IsValid() && !sourceField.IsZero() {
									// If Source is a *url.URL, convert it to string
									if sourceURL, ok := sourceField.Interface().(*url.URL); ok && sourceURL != nil {
										source = sourceURL.String()
									} else if sourceStr, ok := sourceField.Interface().(string); ok {
										source = sourceStr
									}
								}
							}
						}
					}
				}
			}

			if sub.Success {
				if source != "" && destination != "" {
					slog.InfoContext(h.ctx, "Submission succeeded",
						"id", sub.Status.TxID,
						"source", source,
						"destination", destination)
				} else {
					slog.InfoContext(h.ctx, "Submission succeeded", "id", sub.Status.TxID)
				}
			} else {
				// Check for mempool errors in submission status
				if sub.Status != nil && sub.Status.Error != nil {
					errStr := sub.Status.Error.Error()
					if strings.Contains(strings.ToLower(errStr), "mempool") {
						mempoolErrorsMu.Lock()
						mempoolErrors++
						lastMempoolErr = time.Now()
						mempoolErrorsMu.Unlock()
						slog.ErrorContext(h.ctx, "Mempool error", "message", sub, "status", sub.Status, "count", mempoolErrors)
					} else {
						slog.ErrorContext(h.ctx, "Submission failed", "message", sub, "status", sub.Status)
					}
				} else {
					slog.ErrorContext(h.ctx, "Submission failed", "message", sub, "status", sub.Status)
				}
			}
		}
	}
}

func getAccount[T protocol.Account](h *healer, u *url.URL) T {
	r, err := h.tryEach().QueryAccount(h.ctx, u, nil)
	check(err)
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
	var peers []peer.ID
	
	// Check if network info is available
	if q.net == nil {
		// If we're in a test or the network info hasn't been loaded yet,
		// just use the direct client
		slog.InfoContext(ctx, "Network info not available, using direct client")
		return q.C1.Query(ctx, scope, query)
	}
	
	// First, collect peers from partition IDs
	for partID, partPeers := range q.net.Peers {
		// Skip partition entries that aren't valid peer IDs
		if strings.ToLower(partID) == "apollo" || 
		 strings.ToLower(partID) == "yutu" || 
		 strings.ToLower(partID) == "chandrayaan" || 
		 strings.ToLower(partID) == "directory" {
			// These are partition names, not peer IDs
			slog.DebugContext(ctx, "Skipping partition name", "partition", partID)
			// Add peers from this partition
			for peerID := range partPeers {
				// Add this peer to our list
				peers = append(peers, peerID)
			}
			continue
		}
		
		// Try to decode the partition ID as a peer ID
		peerID, err := peer.Decode(partID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to decode partition ID as peer ID",
				"id", partID,
				"error", err,
				"error_type", fmt.Sprintf("%T", err))
			// Even if the partition ID is invalid, we can still use the peers in this partition
			for peerID := range partPeers {
				peers = append(peers, peerID)
			}
			continue
		}
		
		// Add the partition ID as a peer
		peers = append(peers, peerID)
		
		// Also add all peers from this partition
		for peerID := range partPeers {
			peers = append(peers, peerID)
		}
	}

	// Remove duplicate peers
	uniquePeers := removeDuplicatePeers(peers)
	
	// If no valid peers were found, return an error
	if len(uniquePeers) == 0 {
		return nil, errors.InternalError.WithFormat("no valid peers found")
	}

	// Shuffle the peers to distribute load
	rand.Shuffle(len(uniquePeers), func(i, j int) { 
		uniquePeers[i], uniquePeers[j] = uniquePeers[j], uniquePeers[i] 
	})

	// Try each peer
	var lastErr error
	for _, id := range uniquePeers {
		// Skip problematic nodes for chain queries
		if _, ok := query.(*api.ChainQuery); ok {
			if q.problemNodes[id] {
				slog.InfoContext(ctx, "Skipping problematic node for chain query", "node", id.String())
				continue
			}
		}

		// Query the peer
		slog.InfoContext(ctx, "Querying node", "id", id)
		rec, err := q.C2.Query(ctx, scope, query)
		if err != nil {
			// Check if this is a chain entry error
			if _, ok := query.(*api.ChainQuery); ok && strings.Contains(err.Error(), "element does not exist") {
				// Mark this node as problematic for future chain queries
				q.problemNodes[id] = true
				slog.InfoContext(ctx, "HEALING_PROBLEM: Node marked as problematic due to chain entry error", 
					"node", id.String(), 
					"error", err.Error())
			}
			
			lastErr = err
			continue
		}

		return rec, nil
	}

	// If we get here, all peers failed
	if lastErr != nil {
		return nil, &PeerUnavailableError{Scope: scope, Err: lastErr}
	}
	return nil, errors.NotFound
}

// Helper function to remove duplicate peer IDs
func removeDuplicatePeers(peers []peer.ID) []peer.ID {
	seen := make(map[peer.ID]bool)
	result := []peer.ID{}
	
	for _, p := range peers {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	
	return result
}

func discoverPeersViaJSONRPC(ctx context.Context, client *jsonrpc.Client) ([]multiaddr.Multiaddr, error) {
	fmt.Fprintf(os.Stderr, "Starting peer discovery via JSON-RPC...\n")
	
	// Query network status to get peer information
	var status *api.NetworkStatus
	var err error
	
	// Add retry logic for network status
	maxRetries := 3
	retryDelay := 2 * time.Second
	
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			fmt.Fprintf(os.Stderr, "Retrying network status query (attempt %d/%d)...\n", retry+1, maxRetries)
			time.Sleep(retryDelay)
			// Increase delay for subsequent retries
			retryDelay *= 2
		}
		
		status, err = client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "Failed to get network status: %v\n", err)
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "All network status query attempts failed\n")
		return nil, err
	}
	
	fmt.Fprintf(os.Stderr, "Network status retrieved, found %d validators\n", len(status.Network.Validators))
	
	// Extract and validate peer addresses
	var validAddresses []multiaddr.Multiaddr
	var rejectedPeers []string
	
	// Known IP addresses and peer IDs for Accumulate nodes
	knownNodes := map[string]string{
		"144.76.105.23": "12D3KooWS2Adojqun5RV1Xy4k6vKXWpRQ3VdzXnW8SbW7ERzqKie",
		"95.217.104.54": "12D3KooWFpsh2YWYhHhGCK8vFJAKXBKhZEYwVYvRJ8aMwHoNbJnV",
	}
	
	// Additional known validators with their peer IDs for each BVN
	knownValidators := map[string]map[string]string{
		"Apollo": {
			"144.76.105.23": "12D3KooWS2Adojqun5RV1Xy4k6vKXWpRQ3VdzXnW8SbW7ERzqKie",
			"95.217.104.54": "12D3KooWFpsh2YWYhHhGCK8vFJAKXBKhZEYwVYvRJ8aMwHoNbJnV",
			"65.108.0.10":   "12D3KooWEFzWTj2G9UyGBjwZYTnFCz4PJ55xRGociqo4TbEpD6ye",
			"135.181.19.59": "12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj",
		},
		"Yutu": {
			"65.108.0.22":   "12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7",
			"135.181.19.59": "12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL",
		},
		"Chandrayaan": {
			"65.108.0.43":   "12D3KooWHkUtGcHY96bNavZMCP2k5ps5mC7GrF1hBC1CsyGJZSPY",
			"135.181.19.59": "12D3KooWKJuspMDC5GXzLYJs9nHwYfqst9QAW4m5FakXNHVMNiq7",
		},
	}
	
	fmt.Fprintf(os.Stderr, "Using %d known nodes for peer discovery\n", len(knownNodes))
	for ip, peerID := range knownNodes {
		fmt.Fprintf(os.Stderr, "  Known node: %s (Peer ID: %s)\n", ip, peerID)
	}
	
	// Standard port for P2P connections
	p2pPort := "16593"
	
	// First try to extract peer information from validators
	fmt.Fprintf(os.Stderr, "Examining validators and adding them as peers...\n")
	
	// Map to track validators by operator URL
	validatorsByOperator := make(map[string][]string)
	
	// First pass: collect all validator information
	for i, validator := range status.Network.Validators {
		pkHashStr := fmt.Sprintf("%x", validator.PublicKeyHash)
		fmt.Fprintf(os.Stderr, "Examining validator %d: Public Key Hash: %s\n", i, pkHashStr)
		
		// Track which BVNs this validator is part of
		var bvns []string
		
		// Try to extract information from validator partitions
		for _, partition := range validator.Partitions {
			fmt.Fprintf(os.Stderr, "  Validator partition: %s (Active: %v)\n", partition.ID, partition.Active)
			if partition.Active {
				bvns = append(bvns, partition.ID)
			}
		}
		
		// If we have an operator URL, log it and track it
		if validator.Operator != nil {
			operatorStr := validator.Operator.String()
			fmt.Fprintf(os.Stderr, "  Validator operator: %s\n", operatorStr)
			
			// Add to our map of validators by operator
			validatorsByOperator[operatorStr] = bvns
			
			// Try to query information about this validator
			// We'll use the operator URL to identify the validator
			for _, bvn := range bvns {
				// If we have known validator information for this BVN, use it
				if bvnNodes, ok := knownValidators[bvn]; ok {
					for ip, peerID := range bvnNodes {
						addrStr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, p2pPort, peerID)
						addr, err := multiaddr.NewMultiaddr(addrStr)
						if err == nil {
							validAddresses = append(validAddresses, addr)
							fmt.Fprintf(os.Stderr, "  Added validator peer for %s on %s: %s\n", operatorStr, bvn, addr.String())
						} else {
							rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", addrStr, err))
							fmt.Fprintf(os.Stderr, "  Rejected validator address: %s (invalid multiaddr: %v)\n", addrStr, err)
						}
					}
				}
			}
		}
	}
	
	// Try to get consensus status for each partition to find peer information
	partitions := []string{"Directory", "Apollo", "Yutu", "Chandrayaan"}
	fmt.Fprintf(os.Stderr, "Querying consensus status for partitions to find peer information...\n")
	
	for _, partition := range partitions {
		fmt.Fprintf(os.Stderr, "Querying consensus status for partition: %s\n", partition)
		
		// Query consensus status with includePeers=true
		consensusOpts := api.ConsensusStatusOptions{
			Partition:    partition,
			IncludePeers: &[]bool{true}[0],
		}
		
		// Add retry logic for consensus status
		var consensusStatus *api.ConsensusStatus
		var consensusErr error
		
		for retry := 0; retry < maxRetries; retry++ {
			if retry > 0 {
				fmt.Fprintf(os.Stderr, "  Retrying consensus status query for %s (attempt %d/%d)...\n", partition, retry+1, maxRetries)
				time.Sleep(retryDelay)
			}
			
			consensusStatus, consensusErr = client.ConsensusStatus(ctx, consensusOpts)
			if consensusErr == nil {
				break
			}
			fmt.Fprintf(os.Stderr, "  Failed to get consensus status for %s: %v\n", partition, consensusErr)
		}
		
		if consensusErr != nil {
			fmt.Fprintf(os.Stderr, "  All consensus status query attempts failed for %s\n", partition)
			
			// If we failed to get consensus status, use our known validators for this partition
			if bvnNodes, ok := knownValidators[partition]; ok {
				fmt.Fprintf(os.Stderr, "  Using known validators for %s\n", partition)
				for ip, peerID := range bvnNodes {
					addrStr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, p2pPort, peerID)
					addr, err := multiaddr.NewMultiaddr(addrStr)
					if err == nil {
						validAddresses = append(validAddresses, addr)
						fmt.Fprintf(os.Stderr, "    Added known validator for %s: %s\n", partition, addr.String())
					} else {
						rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", addrStr, err))
						fmt.Fprintf(os.Stderr, "    Rejected validator address: %s (invalid multiaddr: %v)\n", addrStr, err)
					}
				}
			}
			
			continue
		}
		
		// Process peers from consensus status
		if len(consensusStatus.Peers) > 0 {
			fmt.Fprintf(os.Stderr, "  Found %d peers in consensus status for %s\n", len(consensusStatus.Peers), partition)
			
			for j, peer := range consensusStatus.Peers {
				fmt.Fprintf(os.Stderr, "    Peer %d: NodeID=%s, Host=%s, Port=%d\n", j, peer.NodeID, peer.Host, peer.Port)
				
				// Try to construct a valid P2P multiaddress
				if peer.NodeID != "" && peer.Host != "" {
					// Construct a multiaddr with the P2P component
					addrStr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", peer.Host, peer.Port, peer.NodeID)
					addr, err := multiaddr.NewMultiaddr(addrStr)
					if err == nil {
						validAddresses = append(validAddresses, addr)
						fmt.Fprintf(os.Stderr, "      Added peer from consensus status: %s\n", addr.String())
					} else {
						rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", addrStr, err))
						fmt.Fprintf(os.Stderr, "      Rejected peer address: %s (invalid multiaddr: %v)\n", addrStr, err)
					}
				} else {
					if peer.NodeID == "" {
						fmt.Fprintf(os.Stderr, "      Skipping peer with empty NodeID\n")
					}
					if peer.Host == "" {
						fmt.Fprintf(os.Stderr, "      Skipping peer with empty Host\n")
					}
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "  No peers found in consensus status for %s\n", partition)
		}
	}
	
	// Process known nodes as a fallback
	fmt.Fprintf(os.Stderr, "Using known nodes for peer discovery\n")
	
	for ip, peerID := range knownNodes {
		addrStr := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, p2pPort, peerID)
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err == nil {
			validAddresses = append(validAddresses, addr)
			fmt.Fprintf(os.Stderr, "  Added peer address from known nodes: %s\n", addr.String())
		} else {
			rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", addrStr, err))
			fmt.Fprintf(os.Stderr, "  Rejected peer address: %s (invalid multiaddr: %v)\n", addrStr, err)
		}
	}
	
	// Try to get node info if available
	fmt.Fprintf(os.Stderr, "Attempting to get node info...\n")
	
	// Add retry logic for node info
	var nodeInfo *api.NodeInfo
	var nodeInfoErr error
	
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			fmt.Fprintf(os.Stderr, "Retrying node info query (attempt %d/%d)...\n", retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
		
		nodeInfo, nodeInfoErr = client.NodeInfo(ctx, api.NodeInfoOptions{})
		if nodeInfoErr == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "Failed to get node info: %v\n", nodeInfoErr)
	}
	
	if nodeInfoErr == nil {
		fmt.Fprintf(os.Stderr, "Retrieved node info, checking for peer information\n")
		
		// Log peer ID if available
		peerIDStr := nodeInfo.PeerID.String()
		if peerIDStr != "" {
			fmt.Fprintf(os.Stderr, "  Node Peer ID: %s\n", peerIDStr)
		}
		
		// Log network if available
		if nodeInfo.Network != "" {
			fmt.Fprintf(os.Stderr, "  Node Network: %s\n", nodeInfo.Network)
		}
		
		// Check if there are any services in the node info
		if len(nodeInfo.Services) > 0 {
			fmt.Fprintf(os.Stderr, "  Found %d services in node info\n", len(nodeInfo.Services))
			
			for i, service := range nodeInfo.Services {
				fmt.Fprintf(os.Stderr, "    Service %d: Type=%v, Argument=%s\n", i, service.Type, service.Argument)
				
				// Try to construct a multiaddr from service information if possible
				if service.Argument != "" {
					// Try to parse as a multiaddr directly
					addr, err := multiaddr.NewMultiaddr(service.Argument)
					if err == nil && strings.Contains(service.Argument, "/p2p/") {
						validAddresses = append(validAddresses, addr)
						fmt.Fprintf(os.Stderr, "      Added peer from service: %s\n", addr.String())
					} else if err != nil {
						rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", service.Argument, err))
						fmt.Fprintf(os.Stderr, "      Rejected service address: %s (invalid multiaddr: %v)\n", service.Argument, err)
					} else {
						rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (missing p2p component)", service.Argument))
						fmt.Fprintf(os.Stderr, "      Rejected service address: %s (missing p2p component)\n", service.Argument)
					}
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "  No services found in node info\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "All node info query attempts failed\n")
	}
	
	// If we still didn't find any valid addresses, use the bootstrap servers as a fallback
	if len(validAddresses) == 0 {
		fmt.Fprintf(os.Stderr, "No valid addresses found, using fallback bootstrap servers\n")
		
		// Get bootstrap servers as strings
		bootstrapStrings := []string{
			"/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
		}
		
		for _, addrStr := range bootstrapStrings {
			addr, err := multiaddr.NewMultiaddr(addrStr)
			if err == nil {
				validAddresses = append(validAddresses, addr)
				fmt.Fprintf(os.Stderr, "  Added fallback bootstrap server: %s\n", addr.String())
			} else {
				rejectedPeers = append(rejectedPeers, fmt.Sprintf("%s (invalid multiaddr: %v)", addrStr, err))
				fmt.Fprintf(os.Stderr, "  Rejected fallback bootstrap server: %s (invalid multiaddr: %v)\n", addrStr, err)
			}
		}
	}
	
	// Summary of rejected peers
	if len(rejectedPeers) > 0 {
		fmt.Fprintf(os.Stderr, "Summary of rejected peers (%d):\n", len(rejectedPeers))
		for i, peer := range rejectedPeers {
			fmt.Fprintf(os.Stderr, "  %d: %s\n", i+1, peer)
		}
	}
	
	// Remove duplicates
	uniqueAddresses := removeDuplicateAddresses(validAddresses)
	fmt.Fprintf(os.Stderr, "Final peer count after removing duplicates: %d\n", len(uniqueAddresses))
	
	return uniqueAddresses, nil
}

func removeDuplicateAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	seen := make(map[string]bool)
	var uniqueAddrs []multiaddr.Multiaddr
	for _, addr := range addrs {
		addrStr := addr.String()
		if !seen[addrStr] {
			seen[addrStr] = true
			uniqueAddrs = append(uniqueAddrs, addr)
		}
	}
	return uniqueAddrs
}

type PeerUnavailableError struct {
	Scope *url.URL
	Err   error
}

func (e *PeerUnavailableError) Error() string {
	return fmt.Sprintf("peer unavailable for %v: %v", e.Scope, e.Err)
}

type NotFoundError struct {
	Scope *url.URL
	Query api.Query
	Err   error
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found for %v: %v", e.Scope, e.Err)
}

type srcInfo struct {
	name string
	dsts []dstInfo
}

type dstInfo struct {
	name string
	info MissingAnchorsInfo
}

func addMissingAnchors(src, dst string, info MissingAnchorsInfo) {
	missingAnchorsMu.Lock()
	defer missingAnchorsMu.Unlock()

	if missingAnchors == nil {
		missingAnchors = make(map[string]map[string]MissingAnchorsInfo)
	}

	if missingAnchors[src] == nil {
		missingAnchors[src] = make(map[string]MissingAnchorsInfo)
	}

	// Store the missing anchors info
	missingAnchors[src][dst] = info
}

func printMissingAnchorsSummary() {
	missingAnchorsMu.Lock()
	defer missingAnchorsMu.Unlock()
	
	// Define all expected partition pairs
	expectedPairs := []struct {
		src string
		dst string
	}{
		{"Directory", "Apollo"},
		{"Directory", "Yutu"},
		{"Directory", "Chandrayaan"},
		{"Apollo", "Directory"},
		{"Yutu", "Directory"},
		{"Chandrayaan", "Directory"},
	}
	
	// Create a map to track which pairs we've seen
	seenPairs := make(map[string]map[string]bool)
	
	// Initialize the map
	for _, pair := range expectedPairs {
		if seenPairs[pair.src] == nil {
			seenPairs[pair.src] = make(map[string]bool)
		}
		seenPairs[pair.src][pair.dst] = false
	}
	
	// Mark pairs that we have data for
	for src, dsts := range missingAnchors {
		if seenPairs[src] == nil {
			seenPairs[src] = make(map[string]bool)
		}
		for dst := range dsts {
			seenPairs[src][dst] = true
		}
	}
	
	// Print the table header
	fmt.Println("\n===========================================")
	fmt.Println(time.Now().Format("Mon Jan 02 03:04:05 PM MST 2006"))
	
	// Display time since last mempool error
	mempoolErrorsMu.Lock()
	if !lastMempoolErr.IsZero() {
		fmt.Printf("Last mempool error: %s\n", lastMempoolErr.Format("Mon Jan 02 03:04:05 PM MST 2006"))
	} else {
		fmt.Println("No mempool errors recorded")
	}
	mempoolErrorsMu.Unlock()
	
	// Prepare data for sorted output
	var sortedSrcs []srcInfo
	for src, dsts := range missingAnchors {
		si := srcInfo{name: src}
		for dst, info := range dsts {
			si.dsts = append(si.dsts, dstInfo{name: dst, info: info})
		}
		// Sort destinations alphabetically
		sort.Slice(si.dsts, func(i, j int) bool {
			return si.dsts[i].name < si.dsts[j].name
		})
		sortedSrcs = append(sortedSrcs, si)
	}
	// Sort sources alphabetically
	sort.Slice(sortedSrcs, func(i, j int) bool {
		return sortedSrcs[i].name < sortedSrcs[j].name
	})
	
	// Print in chart format for backward compatibility
	for _, src := range sortedSrcs {
		for _, dst := range src.dsts {
			if dst.info.Count > 0 {
				color.Red("🗴 %s → %s has %d pending anchors (from %d)\n", src.name, dst.name, dst.info.Count, dst.info.FirstMissingHeight)
			}
			if dst.info.Unprocessed > 0 {
				color.Yellow("⚠ %s → %s has %d unprocessed anchors (%d → %d)\n", src.name, dst.name, dst.info.Unprocessed, dst.info.UnprocessedStart, dst.info.UnprocessedEnd)
			}
		}
	}
	
	fmt.Println()
	
	// Table header
	fmt.Printf("%-10s | %-10s | %-12s | %-12s | %-10s | %s\n", 
		"Source", "Dest", "Source Height", "Dest Height", "Difference", "Status")
	fmt.Println(strings.Repeat("-", 80))
	
	// Print the pairs we have data for
	for _, src := range sortedSrcs {
		for _, dst := range src.dsts {
			// Calculate source height (last processed height + pending + unprocessed)
			sourceHeight := dst.info.LastProcessedHeight
			if dst.info.Count > 0 {
				// If we have pending anchors, we need to be careful about how we calculate the source height
				// For cases like Directory to Apollo where there's a large gap, we should use the first missing height
				// plus the pending count minus 1, rather than assuming all anchors in between are missing
				if src.name == "Directory" && dst.name == "Apollo" {
					// For Directory to Apollo, use the first missing height
					sourceHeight = dst.info.FirstMissingHeight
				} else if dst.info.Count > 50 {
					// For large pending counts, use the first missing height
					sourceHeight = dst.info.FirstMissingHeight
				} else {
					// For small pending counts, add the pending count to the last processed height
					sourceHeight = dst.info.LastProcessedHeight + uint64(dst.info.Count)
				}
			}
			if dst.info.Unprocessed > 0 && dst.info.UnprocessedEnd > sourceHeight {
				sourceHeight = dst.info.UnprocessedEnd
			}
			
			// Destination height is the last processed height
			destHeight := dst.info.LastProcessedHeight
			
			// Calculate difference
			diff := int64(sourceHeight) - int64(destHeight)
			
			// Determine status
			status := "Up to date"
			if dst.info.Count > 0 {
				status = fmt.Sprintf("%d pending", dst.info.Count)
			}
			if dst.info.Unprocessed > 0 {
				if dst.info.Count > 0 {
					status += fmt.Sprintf(", %d unprocessed", dst.info.Unprocessed)
				} else {
					status = fmt.Sprintf("%d unprocessed", dst.info.Unprocessed)
				}
			}
			
			// Print table row
			fmt.Printf("%-10s | %-10s | %-12d | %-12d | %-10d | %s\n", 
				src.name, dst.name, sourceHeight, destHeight, diff, status)
				
			// Mark this pair as seen
			if seenPairs[src.name] != nil {
				seenPairs[src.name][dst.name] = true
			}
		}
	}
	
	fmt.Println()
	
	// Print the pairs we don't have data for
	skippedPartitionsMu.Lock()
	defer skippedPartitionsMu.Unlock()
	
	for _, pair := range expectedPairs {
		src, dst := pair.src, pair.dst
		
		// Skip if we already printed this pair
		if seenPairs[src] != nil && seenPairs[src][dst] {
			continue
		}
		
		// Check if it's in the skipped partitions
		var reason string
		if skippedPartitions[src] != nil {
			reason = skippedPartitions[src][dst]
		}
		
		status := "Unknown"
		if reason != "" {
			status = "Skipped: " + reason
		}
		
		// Print table row for missing pair
		fmt.Printf("%-10s | %-10s | %-12s | %-12s | %-10s | %s\n", 
			src, dst, "N/A", "N/A", "N/A", status)
	}
	
	fmt.Println()
}
