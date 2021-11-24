package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// version represents the faucet command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "get version of the accumulate node",
	Run: func(cmd *cobra.Command, args []string) {
		GetVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func GetVersion() {
	var res interface{}

	if err := Client.Request(context.Background(), "version", nil, &res); err != nil {
		log.Fatal(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}
