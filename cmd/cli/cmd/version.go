package cmd

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"
)

// version represents the faucet command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "get version of the accumulate node",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetVersion()
		if err != nil {
			cmd.Print("Error: ")
			cmd.PrintErr(err)
		} else {
			cmd.Println(out)
		}
	},
}

func GetVersion() (string, error) {
	var res interface{}

	if err := Client.Request(context.Background(), "version", nil, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}
