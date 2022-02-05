package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

// version represents the faucet command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "get version of the accumulate node",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetVersion()
		printOutput(cmd, out, err)
	},
}

func GetVersion() (string, error) {
	res, err := Client.Version(context.Background())
	if err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintJson(res.Data)
}
