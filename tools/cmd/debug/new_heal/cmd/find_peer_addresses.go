// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal"
)

var findPeerAddressesCmd = &cobra.Command{
	Use:   "find-peer-addresses",
	Short: "Find peer addresses from the Accumulate network",
	RunE:  findPeerAddresses,
}

func init() {
	rootCmd.AddCommand(findPeerAddressesCmd)
}

func findPeerAddresses(cmd *cobra.Command, args []string) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutFlag)*time.Second)
	defer cancel()

	// Get the main network status
	fmt.Println("Fetching main network status...")
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return fmt.Errorf("failed to get network status: %v", err)
	}

	fmt.Printf("Network: %s\n", ns.Network.ID)
	fmt.Printf("Found %d validators\n", len(ns.Network.Validators))

	// Create address directory
	addrDir := new_heal.NewAddressDir()

	// Process validators from network status
	fmt.Println("\n=== Processing Validators ===")
	for _, validator := range ns.Network.Validators {
		fmt.Printf("Processing validator: %x\n", validator.PublicKeyHash)
		fmt.Printf("  Validator ID: %s\n", validator.ID)
		fmt.Printf("  Partition: %s\n", validator.Partition)
		
		// Try to get RPC endpoint using our current method
		rpcEndpoint, err := addrDir.GetPeerRPCEndpoint(validator.ID, validator.Partition)
		if err != nil {
			fmt.Printf("  Error getting RPC endpoint: %v\n", err)
		} else {
			fmt.Printf("  RPC Endpoint (our method): %s\n", rpcEndpoint)
		}

		// Try to get known address for validator
		knownAddr := addrDir.getKnownAddressesForValidator(validator.ID)
		if knownAddr != "" {
			fmt.Printf("  Known address from hardcoded map: %s\n", knownAddr)
		}

		// Try to connect to the validator
		if rpcEndpoint != "" {
			// Try to connect to the RPC endpoint
			fmt.Printf("  Trying to connect to: %s\n", rpcEndpoint)
			c, err := http.New(rpcEndpoint, rpcEndpoint+"/ws")
			if err != nil {
				fmt.Printf("  Error creating Tendermint client: %v\n", err)
			} else {
				// Query the node status
				queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				status, err := c.Status(queryCtx)
				cancel()
				if err != nil {
					fmt.Printf("  Error querying status: %v\n", err)
				} else {
					fmt.Printf("  Successfully connected to %s\n", rpcEndpoint)
					fmt.Printf("  Node info: %s\n", status.NodeInfo.Moniker)
					fmt.Printf("  Network: %s\n", status.NodeInfo.Network)
					fmt.Printf("  Latest block height: %d\n", status.SyncInfo.LatestBlockHeight)
					
					// Query network info to discover more peers
					queryCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
					netInfo, err := c.NetInfo(queryCtx)
					cancel()
					if err != nil {
						fmt.Printf("  Error querying net info: %v\n", err)
					} else {
						fmt.Printf("  Found %d peers\n", len(netInfo.Peers))
						for i, peer := range netInfo.Peers {
							fmt.Printf("  Peer %d: %s (%s)\n", i+1, peer.NodeInfo.Moniker, peer.RemoteIP)
							
							// Try to parse the remote IP as a multiaddr
							maddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/16592", peer.RemoteIP))
							if err != nil {
								fmt.Printf("    Error creating multiaddr: %v\n", err)
							} else {
								fmt.Printf("    Multiaddr: %s\n", maddr)
								
								// Extract host from multiaddr
								host := ""
								multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
									if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
										host = c.Value()
										return false
									}
									return true
								})
								
								if host == "" {
									fmt.Printf("    Could not extract host from multiaddr\n")
								} else {
									fmt.Printf("    Extracted host: %s\n", host)
									
									// Try to connect to the peer
									for _, port := range []int{16592, 16692} {
										peerRPC := fmt.Sprintf("http://%s:%d", host, port)
										fmt.Printf("    Trying to connect to: %s\n", peerRPC)
										pc, err := http.New(peerRPC, peerRPC+"/ws")
										if err != nil {
											fmt.Printf("    Error creating Tendermint client: %v\n", err)
											continue
										}
										
										// Query the node status
										peerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
										peerStatus, err := pc.Status(peerCtx)
										cancel()
										if err != nil {
											fmt.Printf("    Error querying status: %v\n", err)
											continue
										}
										
										fmt.Printf("    Successfully connected to %s\n", peerRPC)
										fmt.Printf("    Node info: %s\n", peerStatus.NodeInfo.Moniker)
										fmt.Printf("    Network: %s\n", peerStatus.NodeInfo.Network)
										fmt.Printf("    Latest block height: %d\n", peerStatus.SyncInfo.LatestBlockHeight)
										break
									}
								}
							}
						}
					}
				}
			}
		}
		
		fmt.Println()
	}

	// Try to get addresses from network status directly
	fmt.Println("\n=== Checking Network Status for Addresses ===")
	for _, node := range ns.Nodes {
		fmt.Printf("Node: %s\n", node.Name)
		fmt.Printf("  Type: %s\n", node.Type)
		
		// Check if the node has any addresses
		if len(node.Addresses) > 0 {
			fmt.Printf("  Addresses (%d):\n", len(node.Addresses))
			for i, addr := range node.Addresses {
				fmt.Printf("    %d: %s\n", i+1, addr)
				
				// Try to parse as multiaddr
				maddr, err := multiaddr.NewMultiaddr(addr)
				if err != nil {
					fmt.Printf("      Error parsing multiaddr: %v\n", err)
					
					// Try to extract IP if it's in a different format
					if strings.Contains(addr, "://") {
						parts := strings.Split(addr, "://")
						if len(parts) > 1 {
							hostPort := strings.Split(parts[1], ":")
							if len(hostPort) > 0 {
								host := hostPort[0]
								fmt.Printf("      Extracted host from URL: %s\n", host)
							}
						}
					}
				} else {
					fmt.Printf("      Parsed multiaddr: %s\n", maddr)
					
					// Extract host from multiaddr
					host := ""
					multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
						if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
							host = c.Value()
							return false
						}
						return true
					})
					
					if host == "" {
						fmt.Printf("      Could not extract host from multiaddr\n")
					} else {
						fmt.Printf("      Extracted host: %s\n", host)
					}
				}
			}
		} else {
			fmt.Printf("  No addresses found\n")
		}
		
		fmt.Println()
	}

	return nil
}
