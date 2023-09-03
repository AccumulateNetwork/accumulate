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
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

var networkCmd = &cobra.Command{
	Use: "network",
}

var networkScanCmd = &cobra.Command{
	Use:   "scan [network]",
	Short: "Scan the network for nodes",
	Run:   scanNetwork,
}

func init() {
	cmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkScanCmd)

	networkScanCmd.Flags().BoolVarP(&outputJSON, "json", "j", false, "Output result as JSON")
}

func scanNetwork(_ *cobra.Command, args []string) {
	client := jsonrpc.NewClient(api.ResolveWellKnownEndpoint(args[0]))
	client.Client.Timeout = time.Hour
	net, err := healing.ScanNetwork(context.Background(), client)
	check(err)

	if outputJSON {
		check(json.NewEncoder(os.Stdout).Encode(net))
		return
	}

	for _, part := range net.Status.Network.Partitions {
		fmt.Println(part.ID)
		for _, peer := range net.Peers[part.ID] {
			fmt.Printf("  %v\n", peer)
		}
	}
}
