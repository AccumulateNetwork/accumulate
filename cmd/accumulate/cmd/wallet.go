package cmd

import (
	"github.com/spf13/cobra"
)

var initWalletCmd = &cobra.Command{
	Use:   "wallet init [create/import]",
	Short: "Import secret factoid key from terminal input",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[1] {
		case "create":
			InitDBCreate(false)
		case "import":
			InitDBImport(cmd, false)
		default:
		}
	},
}
