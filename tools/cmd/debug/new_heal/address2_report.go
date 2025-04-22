// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

// GenerateAddressDirReport generates a comprehensive report of the AddressDir state
// and writes it to the specified writer. If writer is nil, os.Stdout is used.
func GenerateAddressDirReport(addressDir *AddressDir, writer io.Writer) {
	if writer == nil {
		writer = os.Stdout
	}

	// Get a read lock on the address directory
	addressDir.mu.RLock()
	defer addressDir.mu.RUnlock()

	// Print report header
	fmt.Fprintf(writer, "=== ACCUMULATE NETWORK ADDRESS DIRECTORY REPORT ===\n")
	fmt.Fprintf(writer, "Generated: %s\n\n", time.Now().Format(time.RFC1123))

	// Network Information
	printNetworkInfo(addressDir, writer)

	// Directory Network Validators
	printDNValidators(addressDir, writer)

	// BVN Validators by Partition
	printBVNValidators(addressDir, writer)

	// Network Peers
	printNetworkPeers(addressDir, writer)

	// Discovery Statistics
	printDiscoveryStats(addressDir, writer)

	// Print report footer
	fmt.Fprintf(writer, "\n=== END OF REPORT ===\n")
}

// printNetworkInfo prints basic network information
func printNetworkInfo(addressDir *AddressDir, writer io.Writer) {
	fmt.Fprintf(writer, "## Network Information\n\n")

	network := addressDir.GetNetwork()
	if network == nil {
		fmt.Fprintf(writer, "No network information available.\n\n")
		return
	}

	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Property", "Value"})
	table.SetBorder(false)
	table.SetColumnSeparator(" | ")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	table.Append([]string{"Network Name", network.Name})
	table.Append([]string{"Network ID", network.ID})
	table.Append([]string{"Is Mainnet", fmt.Sprintf("%v", network.IsMainnet)})
	table.Append([]string{"API Endpoint", network.APIEndpoint})
	table.Append([]string{"Partitions", fmt.Sprintf("%d", len(network.Partitions))})

	// Add partition details
	var partitionNames []string
	for _, p := range network.Partitions {
		partitionNames = append(partitionNames, p.ID)
	}
	table.Append([]string{"Partition List", strings.Join(partitionNames, ", ")})

	table.Render()
	fmt.Fprintln(writer)
}

// printDNValidators prints information about Directory Network validators
func printDNValidators(addressDir *AddressDir, writer io.Writer) {
	fmt.Fprintf(writer, "## Directory Network Validators (%d)\n\n", len(addressDir.DNValidators))

	if len(addressDir.DNValidators) == 0 {
		fmt.Fprintf(writer, "No Directory Network validators found.\n\n")
		return
	}

	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Name", "Status", "P2P Address", "RPC Address", "API Address", "Addresses"})
	table.SetBorder(true)
	table.SetColumnSeparator("|")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, validator := range addressDir.DNValidators {
		table.Append([]string{
			validator.Name,
			validator.Status,
			validator.P2PAddress,
			validator.RPCAddress,
			validator.APIAddress,
			fmt.Sprintf("%d", len(validator.Addresses)),
		})
	}

	table.Render()
	fmt.Fprintln(writer)

	// Print detailed address information for each validator
	for i, validator := range addressDir.DNValidators {
		if len(validator.Addresses) > 0 {
			fmt.Fprintf(writer, "### DN Validator %d: %s - Addresses\n\n", i+1, validator.Name)

			addrTable := tablewriter.NewWriter(writer)
			addrTable.SetHeader([]string{"Address", "Validated", "IP", "Port", "Peer ID"})
			addrTable.SetBorder(true)
			addrTable.SetColumnSeparator("|")
			addrTable.SetAlignment(tablewriter.ALIGN_LEFT)
			addrTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

			for _, addr := range validator.Addresses {
				addrTable.Append([]string{
					addr.Address,
					fmt.Sprintf("%v", addr.Validated),
					addr.IP,
					addr.Port,
					addr.PeerID,
				})
			}

			addrTable.Render()
			fmt.Fprintln(writer)
		}
	}
}

// printBVNValidators prints information about BVN validators by partition
func printBVNValidators(addressDir *AddressDir, writer io.Writer) {
	totalBVNValidators := 0
	for _, bvnGroup := range addressDir.BVNValidators {
		totalBVNValidators += len(bvnGroup)
	}

	fmt.Fprintf(writer, "## BVN Validators (%d groups, %d total)\n\n", len(addressDir.BVNValidators), totalBVNValidators)

	if len(addressDir.BVNValidators) == 0 {
		fmt.Fprintf(writer, "No BVN validators found.\n\n")
		return
	}

	// For each BVN partition
	for i, bvnGroup := range addressDir.BVNValidators {
		if len(bvnGroup) == 0 {
			continue
		}

		partitionID := bvnGroup[0].PartitionID
		fmt.Fprintf(writer, "### BVN Group %d: %s (%d validators)\n\n", i+1, partitionID, len(bvnGroup))

		table := tablewriter.NewWriter(writer)
		table.SetHeader([]string{"Name", "Status", "P2P Address", "RPC Address", "API Address", "Addresses"})
		table.SetBorder(true)
		table.SetColumnSeparator("|")
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

		for _, validator := range bvnGroup {
			table.Append([]string{
				validator.Name,
				validator.Status,
				validator.P2PAddress,
				validator.RPCAddress,
				validator.APIAddress,
				fmt.Sprintf("%d", len(validator.Addresses)),
			})
		}

		table.Render()
		fmt.Fprintln(writer)

		// Print detailed address information for each validator in this BVN
		for j, validator := range bvnGroup {
			if len(validator.Addresses) > 0 {
				fmt.Fprintf(writer, "#### BVN Validator %d.%d: %s - Addresses\n\n", i+1, j+1, validator.Name)

				addrTable := tablewriter.NewWriter(writer)
				addrTable.SetHeader([]string{"Address", "Validated", "IP", "Port", "Peer ID"})
				addrTable.SetBorder(true)
				addrTable.SetColumnSeparator("|")
				addrTable.SetAlignment(tablewriter.ALIGN_LEFT)
				addrTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

				for _, addr := range validator.Addresses {
					addrTable.Append([]string{
						addr.Address,
						fmt.Sprintf("%v", addr.Validated),
						addr.IP,
						addr.Port,
						addr.PeerID,
					})
				}

				addrTable.Render()
				fmt.Fprintln(writer)
			}
		}
	}
}

// printNetworkPeers prints information about non-validator network peers
func printNetworkPeers(addressDir *AddressDir, writer io.Writer) {
	// Count non-validator peers
	nonValidatorCount := 0
	for _, peer := range addressDir.NetworkPeers {
		if !peer.IsValidator {
			nonValidatorCount++
		}
	}

	fmt.Fprintf(writer, "## Network Peers (%d total, %d non-validators)\n\n", len(addressDir.NetworkPeers), nonValidatorCount)

	if len(addressDir.NetworkPeers) == 0 {
		fmt.Fprintf(writer, "No network peers found.\n\n")
		return
	}

	// Sort peers by ID for consistent output
	var peerIDs []string
	for id := range addressDir.NetworkPeers {
		peerIDs = append(peerIDs, id)
	}
	sort.Strings(peerIDs)

	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"ID", "Validator", "Partition", "Status", "Addresses", "First Seen", "Last Seen"})
	table.SetBorder(true)
	table.SetColumnSeparator("|")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, id := range peerIDs {
		peer := addressDir.NetworkPeers[id]
		table.Append([]string{
			id,
			fmt.Sprintf("%v", peer.IsValidator),
			peer.PartitionID,
			peer.Status,
			fmt.Sprintf("%d", len(peer.Addresses)),
			formatTime(peer.FirstSeen),
			formatTime(peer.LastSeen),
		})
	}

	table.Render()
	fmt.Fprintln(writer)

	// Print detailed address information for each peer
	for _, id := range peerIDs {
		peer := addressDir.NetworkPeers[id]
		if len(peer.Addresses) > 0 {
			fmt.Fprintf(writer, "### Network Peer: %s - Addresses\n\n", id)

			addrTable := tablewriter.NewWriter(writer)
			addrTable.SetHeader([]string{"Address", "Validated", "IP", "Port", "Last Success"})
			addrTable.SetBorder(true)
			addrTable.SetColumnSeparator("|")
			addrTable.SetAlignment(tablewriter.ALIGN_LEFT)
			addrTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

			for _, addr := range peer.Addresses {
				addrTable.Append([]string{
					addr.Address,
					fmt.Sprintf("%v", addr.Validated),
					addr.IP,
					addr.Port,
					formatTime(addr.LastSuccess),
				})
			}

			addrTable.Render()
			fmt.Fprintln(writer)
		}
	}
}

// printDiscoveryStats prints statistics about the discovery process
func printDiscoveryStats(addressDir *AddressDir, writer io.Writer) {
	fmt.Fprintf(writer, "## Discovery Statistics\n\n")

	stats := addressDir.DiscoveryStats
	if stats == nil {
		fmt.Fprintf(writer, "No discovery statistics available.\n\n")
		return
	}

	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Statistic", "Value"})
	table.SetBorder(false)
	table.SetColumnSeparator(" | ")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	table.Append([]string{"Last Discovery", formatTime(stats.LastDiscovery)})
	table.Append([]string{"Discovery Attempts", fmt.Sprintf("%d", stats.DiscoveryAttempts)})
	table.Append([]string{"Successful Discoveries", fmt.Sprintf("%d", stats.SuccessfulDiscoveries)})
	table.Append([]string{"Failed Discoveries", fmt.Sprintf("%d", stats.FailedDiscoveries)})
	
	// Calculate success rate
	successRate := 0.0
	if stats.DiscoveryAttempts > 0 {
		successRate = float64(stats.SuccessfulDiscoveries) / float64(stats.DiscoveryAttempts) * 100.0
	}
	table.Append([]string{"Success Rate", fmt.Sprintf("%.1f%%", successRate)})
	
	// Format total discovery time
	table.Append([]string{"Total Discovery Time", formatDuration(stats.TotalDiscoveryTime)})
	
	// Calculate average discovery time
	avgTime := time.Duration(0)
	if stats.DiscoveryAttempts > 0 {
		avgTime = stats.TotalDiscoveryTime / time.Duration(stats.DiscoveryAttempts)
	}
	table.Append([]string{"Average Discovery Time", formatDuration(avgTime)})

	table.Render()
	fmt.Fprintln(writer)
}

// Helper function to format time
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	return t.Format("2006-01-02 15:04:05")
}

// Helper function to format duration
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	
	// Format duration in a human-readable way
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	} else if d < time.Hour {
		m := d / time.Minute
		s := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm %ds", m, s)
	} else {
		h := d / time.Hour
		m := (d % time.Hour) / time.Minute
		s := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
}
