package cmd

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api <method> <params>",
	Short: "Send a JSON-RPC API request",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := SendApiRequest(args[0], args[1])
		printOutput(cmd, out, err)
	},
}

func SendApiRequest(method string, params string) (string, error) {
	var res json.RawMessage
	err := Client.RequestAPIv2(context.Background(), method, params, &res)
	if err == nil {
		return string(res), nil
	}

	return "", formatApiError(err)
}
