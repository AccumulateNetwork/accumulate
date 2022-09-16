package cmd

import (
	"encoding/json"
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
	Short: "get list of wallets with their info",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := queryWalletsInfo(cmd, args)
		printOutput(cmd, out, err)
	},
}

func init() {
	initRunFlags(ledgerCmd, false)
	ledgerCmd.AddCommand(ledgerInfoCmd)
}

func queryWalletsInfo(cmd *cobra.Command, args []string) (string, error) {
	ledgerHub, err := walletd.NewLedgerHub()
	if err != nil {
		return "", err
	}
	ledgerInfos, err := ledgerHub.QueryLedgerWalletsInfo()
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		str, err := json.Marshal(ledgerInfos)
		if err != nil {
			return "", err
		}
		return string(str), nil
	} else {
		result := fmt.Sprintln("Wallets:")
		for i, ledgerInfo := range ledgerInfos {
			result += fmt.Sprintf("%d\tManufacturer:\t%s\n", i+1, ledgerInfo.Manufacturer)
			result += fmt.Sprintf("\tProduct:\t%s\n", ledgerInfo.Product)
			result += fmt.Sprintf("\tVendor ID:\t%d\n", ledgerInfo.VendorID)
			result += fmt.Sprintf("\tProduct ID:\t%d\n", ledgerInfo.ProductID)
			result += fmt.Sprintf("\tApp Version:\t%s\n", ledgerInfo.Version.Label)
			result += fmt.Sprintf("\tLedger URL:\t%s\n", ledgerInfo.Url)
		}
		return result, nil
	}
}
