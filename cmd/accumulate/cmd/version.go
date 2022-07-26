package cmd

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "get version of the accumulate node",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out, err := GetVersion()
		printOutput(cmd, out, err)
	},
}

func GetVersion() (string, error) {
	var res interface{}

	if err := Client.RequestAPIv2(context.Background(), "version", nil, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "describe the accumulate node",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out, err := describe()
		printOutput(cmd, out, err)
	},
}

func describe() (string, error) {
	var res interface{}

	if err := Client.RequestAPIv2(context.Background(), "describe", nil, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}
