package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
)

var ledgerCmd = &cobra.Command{
	Use:   "ledger",
	Short: "ledger command",
	Args:  cobra.MinimumNArgs(3),
}

var ledgerInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "get list of wallets info",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := queryLedgerInfo(cmd, args)
		printOutput(cmd, out, err)
	},
}

func init() {
	initRunFlags(ledgerCmd, false)
	ledgerCmd.AddCommand(ledgerInfoCmd)
}

func queryLedgerInfo(cmd *cobra.Command, args []string) (string, error) {
	ledgerState, err := walletd.NewLedgerHub()
	if err != nil {
		return "", err
	}
	ledgerInfos, err := ledgerState.GetLedgerInfos()
	if err != nil {
		return "", err
	}
	result := fmt.Sprintln("Ledgers:")
	for _, ledgerInfo := range ledgerInfos {
		result += fmt.Sprintln("\tVersion:\t", ledgerInfo.Version)
	}
	return result, nil
}
