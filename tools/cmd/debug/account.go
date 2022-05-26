package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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
	Use:   "route <bvn-count> <url>",
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
	var bvnCount int
	if len(args) == 2 {
		c, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v", err)
			os.Exit(1)
		}
		bvnCount = int(c)
		args = args[1:]
	}

	u, err := url.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Account       : %v\n", u)
	fmt.Printf("Account ID    : %X\n", u.AccountID())
	fmt.Printf("Identity ID   : %X\n", u.IdentityAccountID())
	fmt.Printf("Routing number: %X\n", u.Routing())
	if bvnCount != 0 {
		fmt.Printf("Routes to     : BVN %d\n", u.Routing()%uint64(bvnCount))
	}
}
