// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Analyze accounts",
}

var accountIdCmd = &cobra.Command{
	Use:   "id <url>",
	Short: "Show an account's ID",
	Args:  cobra.ExactArgs(1),
	Run:   accountId,
}

var accountRouteCmd = &cobra.Command{
	Use:   "route <network-endpoint> <url>",
	Short: "Calculate the route for an account",
	Args:  cobra.ExactArgs(2),
	Run:   accountId,
}

func init() {
	cmd.AddCommand(accountCmd)
	accountCmd.AddCommand(
		accountIdCmd,
		accountRouteCmd,
	)
}

func accountId(_ *cobra.Command, args []string) {
	var dclient *jsonrpc.Client
	if len(args) == 2 {
		dclient = jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
		args = args[1:]
	}

	u, err := url.Parse(args[0])
	check(err)
	fmt.Printf("Account:        %v\n", u)
	fmt.Printf("Account ID:     %X\n", u.AccountID())
	fmt.Printf("Identity ID:    %X\n", u.IdentityAccountID())
	fmt.Printf("Routing number: %X\n", u.Routing())

	// Do not route
	if dclient == nil {
		return
	}

	// Route using a routing table
	ns, err := dclient.NetworkStatus(context.Background(), api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)

	router := routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})
	partition, err := router.RouteAccount(u)
	check(err)
	fmt.Printf("Method:         prefix routing table\n")
	fmt.Printf("Routes to:      %s\n", partition)
}
