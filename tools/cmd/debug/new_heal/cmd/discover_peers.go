// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

var (
	networkFlag  string
	endpointFlag string
	timeoutFlag  int
	verboseFlag  bool
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&networkFlag, "network", "n", "mainnet", "Network to connect to (mainnet, testnet, or devnet)")
	rootCmd.PersistentFlags().StringVarP(&endpointFlag, "endpoint", "e", "", "Custom API endpoint (overrides network)")
	rootCmd.PersistentFlags().IntVarP(&timeoutFlag, "timeout", "t", 60, "Timeout in seconds for network operations")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable detailed logging")
}

var rootCmd = &cobra.Command{
	Use:   "discover-peers",
	Short: "Discover peer addresses from the Accumulate network",
	RunE:  discoverPeers,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func discoverPeers(cmd *cobra.Command, args []string) error {
	// Setup logging
	if verboseFlag {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	// Set up the API client
	var endpoint string
	if endpointFlag != "" {
		endpoint = endpointFlag
	} else {
		endpoint = accumulate.ResolveWellKnownEndpoint(networkFlag, "v3")
	}

	fmt.Printf("Connecting to %s (%s)...\n", networkFlag, endpoint)
	client := jsonrpc.NewClient(endpoint)

	// Create a context with timeout
	ctx := cmdutil.ContextForMainProcess(context.Background())
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutFlag)*time.Second)
	defer cancel()

	// Get the main network status
	fmt.Println("Fetching main network status...")
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return fmt.Errorf("failed to get network status: %v", err)
	}

	// Check for a cached scan
	var network *healing.NetworkInfo
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	cachedScanPath := filepath.Join(homeDir, ".accumulate", "cache", strings.ToLower(ns.Network.ID+".json"))
	
	data, err := os.ReadFile(cachedScanPath)
	switch {
	case err == nil:
		// Load the cached scan
		fmt.Printf("Loading cached network scan from %s\n", cachedScanPath)
		err = json.Unmarshal(data, &network)
		if err != nil {
			return fmt.Errorf("failed to parse cached network scan: %v", err)
		}

	case errors.Is(err, fs.ErrNotExist):
		// Scan the network
		fmt.Println("No cached scan found, scanning network...")
		network, err = healing.ScanNetwork(ctx, client)
		if err != nil {
			return fmt.Errorf("failed to scan network: %v", err)
		}

		// Save it
		fmt.Printf("Saving network scan to %s\n", cachedScanPath)
		err = os.MkdirAll(filepath.Dir(cachedScanPath), 0755)
		if err != nil {
			return fmt.Errorf("failed to create cache directory: %v", err)
		}
		data, err = json.Marshal(network)
		if err != nil {
			return fmt.Errorf("failed to marshal network scan: %v", err)
		}
		err = os.WriteFile(cachedScanPath, data, 0600)
		if err != nil {
			return fmt.Errorf("failed to write network scan: %v", err)
		}

	default:
		return fmt.Errorf("failed to read cached network scan: %v", err)
	}

	// Map keys to peers
	fmt.Println("\n=== Mapping Keys to Peers ===")
	peerByKeyHash := map[[32]byte]p2p.ID{}
	addrByKeyHash := map[[32]byte][]multiaddr.Multiaddr{}
	for _, part := range network.Peers {
		for _, peer := range part {
			kh := sha256.Sum256(peer.Key[:])
			peerID, err := p2p.IDFromPublicKey(peer.Key)
			if err != nil {
				fmt.Printf("Error creating peer ID from public key: %v\n", err)
				continue
			}
			peerByKeyHash[kh] = peerID
			fmt.Printf("Mapped key hash %x to peer ID %s\n", kh[:4], peerID)

			// Parse multiaddresses
			for _, addr := range peer.Addresses {
				maddr, err := multiaddr.NewMultiaddr(addr)
				if err != nil {
					fmt.Printf("Error parsing multiaddr %s: %v\n", addr, err)
					continue
				}
				addrByKeyHash[kh] = append(addrByKeyHash[kh], maddr)
				fmt.Printf("  Added multiaddr: %s\n", maddr)
			}
		}
	}

	// Process validators
	fmt.Println("\n=== Processing Validators ===")
	var mu sync.Mutex
	var wg sync.WaitGroup
	seenComet := map[string]bool{}
	var cometPeers []p2p.Peer

	// Process validators from network status
	for _, validator := range ns.Network.Validators {
		fmt.Printf("Processing validator: %x\n", validator.PublicKeyHash)
		kh := validator.PublicKeyHash
		peerID, ok := peerByKeyHash[kh]
		if !ok {
			fmt.Printf("  No peer ID found for validator %x\n", kh[:4])
			continue
		}
		fmt.Printf("  Found peer ID: %s\n", peerID)

		addrs := addrByKeyHash[kh]
		if len(addrs) == 0 {
			fmt.Printf("  No addresses found for validator %x\n", kh[:4])
			continue
		}

		// Try to connect to each address
		for _, addr := range addrs {
			fmt.Printf("  Trying address: %s\n", addr)
			
			// Extract host from multiaddr
			host := ""
			multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
				if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
					host = c.Value()
					return false
				}
				return true
			})
			
			if host == "" {
				fmt.Printf("  Could not extract host from multiaddr: %s\n", addr)
				continue
			}
			fmt.Printf("  Extracted host: %s\n", host)

			// Try both primary and secondary Tendermint RPC ports
			for _, port := range []int{16592, 16692} {
				fmt.Printf("  Trying port: %d\n", port)
				base := fmt.Sprintf("http://%s:%d", host, port)
				c, err := http.New(base, base+"/ws")
				if err != nil {
					fmt.Printf("  Error creating Tendermint client: %v\n", err)
					continue
				}

				// Query the node status
				queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				status, err := c.Status(queryCtx)
				cancel()
				if err != nil {
					fmt.Printf("  Error querying status: %v\n", err)
					continue
				}

				fmt.Printf("  Successfully connected to %s:%d\n", host, port)
				fmt.Printf("  Node info: %s\n", status.NodeInfo.Moniker)
				fmt.Printf("  Network: %s\n", status.NodeInfo.Network)
				fmt.Printf("  Latest block height: %d\n", status.SyncInfo.LatestBlockHeight)

				// Query network info to discover more peers
				wg.Add(1)
				go func() {
					defer wg.Done()
					queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					netInfo, err := c.NetInfo(queryCtx)
					if err != nil {
						fmt.Printf("  Error querying net info: %v\n", err)
						return
					}

					mu.Lock()
					for _, peer := range netInfo.Peers {
						if !seenComet[peer.NodeInfo.ID()] {
							cometPeers = append(cometPeers, peer)
							seenComet[peer.NodeInfo.ID()] = true
							fmt.Printf("  Discovered new peer: %s (%s)\n", peer.NodeInfo.Moniker, peer.RemoteIP)
						}
					}
					mu.Unlock()
				}()

				break // Successfully connected, no need to try other ports
			}
		}
	}

	// Use consensus peers to find hosts
	fmt.Println("\n=== Checking Consensus Peers ===")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for len(cometPeers) > 0 {
			scan := cometPeers
			cometPeers = nil
			for _, peer := range scan {
				if seenComet[peer.NodeInfo.ID()] {
					continue
				}
				seenComet[peer.NodeInfo.ID()] = true
				fmt.Printf("Checking peer: %s (%s)\n", peer.NodeInfo.Moniker, peer.RemoteIP)

				// Create a client for this peer
				host := peer.RemoteIP
				for _, port := range []int{16592, 16692} {
					fmt.Printf("  Trying port: %d\n", port)
					base := fmt.Sprintf("http://%s:%d", host, port)
					c, err := http.New(base, base+"/ws")
					if err != nil {
						fmt.Printf("  Error creating Tendermint client: %v\n", err)
						continue
					}

					// Query the node status
					queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					status, err := c.Status(queryCtx)
					cancel()
					if err != nil {
						fmt.Printf("  Error querying status: %v\n", err)
						continue
					}

					fmt.Printf("  Successfully connected to %s:%d\n", host, port)
					fmt.Printf("  Node info: %s\n", status.NodeInfo.Moniker)
					fmt.Printf("  Network: %s\n", status.NodeInfo.Network)
					fmt.Printf("  Latest block height: %d\n", status.SyncInfo.LatestBlockHeight)

					// Query network info to discover more peers
					wg.Add(1)
					go func() {
						defer wg.Done()
						queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
						defer cancel()

						netInfo, err := c.NetInfo(queryCtx)
						if err != nil {
							fmt.Printf("  Error querying net info: %v\n", err)
							return
						}

						mu.Lock()
						for _, p := range netInfo.Peers {
							if !seenComet[p.NodeInfo.ID()] {
								cometPeers = append(cometPeers, p)
								fmt.Printf("  Discovered new peer: %s (%s)\n", p.NodeInfo.Moniker, p.RemoteIP)
							}
						}
						mu.Unlock()
					}()

					break // Successfully connected, no need to try other ports
				}
			}
		}
	}()

	// Wait for all goroutines to complete
	wg.Wait()

	// Summarize findings
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total validators found: %d\n", len(ns.Network.Validators))
	fmt.Printf("Total peers found: %d\n", len(seenComet))

	return nil
}

// Helper function to create a Tendermint HTTP client for a peer
func httpClientForPeer(peer p2p.Peer, port int) (*http.HTTP, error) {
	if port == 0 {
		port = 16592
	}
	base := fmt.Sprintf("http://%s:%d", peer.RemoteIP, port)
	return http.New(base, base+"/ws")
}

// Helper function to create a promise that may fail
func maybe[T any](f func() (T, bool)) func() (T, error) {
	return func() (T, error) {
		v, ok := f()
		if !ok {
			var zero T
			return zero, errors.New("failed")
		}
		return v, nil
	}
}

// Helper function to create a done callback
func done[T any](f func(T)) func(T, error) {
	return func(v T, err error) {
		if err != nil {
			return
		}
		f(v)
	}
}

// Helper function to wait for a promise
func waitFor(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

// Helper function to work with a mutex
func work(mu *sync.Mutex, wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock()
		defer mu.Unlock()
		f()
	}()
}

// Helper function to call a function with a promise
func promise[T any](f func() (T, error)) func() (T, error) {
	return f
}

// Helper function to sync a promise with a mutex
func syncPromise[T any](mu *sync.Mutex, f func() (T, error)) func() (T, error) {
	return func() (T, error) {
		mu.Lock()
		defer mu.Unlock()
		return f()
	}
}

// Helper function to sync a promise then call a function
func syncThen[T any](mu *sync.Mutex, p func() (T, error), f func(T, error)) func() {
	return func() {
		var v T
		var err error
		func() {
			mu.Lock()
			defer mu.Unlock()
			v, err = p()
		}()
		f(v, err)
	}
}

// Helper function to call a function
func call[T any](f func() (T, error)) func() (T, error) {
	return f
}
