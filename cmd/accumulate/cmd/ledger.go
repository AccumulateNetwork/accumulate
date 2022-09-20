package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
)

var walletID string
var debug bool

var ledgerCmd = &cobra.Command{
	Use:   "ledger",
	Short: "ledger commands",
	Args:  cobra.MinimumNArgs(3),
}

var ledgerInfoCmd = &cobra.Command{
	Use:   "info --walletID [wallet id] (optional) --debug",
	Short: "get list of wallets with their info",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := queryWalletsInfo(cmd, args)
		printOutput(cmd, out, err)
	},
}

var ledgerKeyGenerateCmd = &cobra.Command{
	Use:   "key generate [key name/label]",
	Short: "generate keypair on a ledger device and give it a name",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := legerGenerateKey(cmd, args)
		printOutput(cmd, out, err)
	},
}

func init() {
	initRunFlags(ledgerCmd, false)
	ledgerCmd.AddCommand(ledgerInfoCmd)
	ledgerCmd.AddCommand(ledgerKeyGenerateCmd)
	ledgerKeyGenerateCmd.Flags().StringVar(&walletID, "wallet-id", "", "specify the wallet id by \"ledger info\" index or ledger ID URL")
	ledgerKeyGenerateCmd.Flags().BoolVar(&debug, "debug", false, "set debug loggin gon")
}

func queryWalletsInfo(cmd *cobra.Command, args []string) (string, error) {
	ledgerApi, err := walletd.NewLedgerApi(debug)
	if err != nil {
		return "", err
	}
	ledgerInfos, err := ledgerApi.QueryLedgerWalletsInfo()
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
			result += fmt.Sprintf("\tWallet ID:\t%s\n", ledgerInfo.Url)
			if ledgerInfo.Status == "ok" {
				result += fmt.Sprintf("\tApp Version:\t%s\n", ledgerInfo.Version.Label)
			}
			result += fmt.Sprintf("\tStatus:\t\t%s\n", ledgerInfo.Status)
		}
		return result, nil
	}
}

func legerGenerateKey(cmd *cobra.Command, args []string) (string, error) {
	ledgerApi, err := walletd.NewLedgerApi(debug)
	if err != nil {
		return "", err
	}

	selWallet, err := ledgerApi.SelectWallet(walletID)
	if err != nil {
		return "", err
	}

	if !WantJsonOutput {
		cmd.Println("Please confirm the address on your Ledger device")
	}
	label := args[1]
	keyData, err := ledgerApi.GenerateKey(selWallet, label)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		str, err := json.Marshal(keyData)
		if err != nil {
			return "", err
		}

		return string(str), nil
	}

	return fmt.Sprintf("\tname\t\t:\t%s\n\twallet ID\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", label,
		keyData.WalletID, keyData.PublicKey, keyData.KeyType), nil
}
