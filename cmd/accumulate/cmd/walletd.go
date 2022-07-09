package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var walletdCmd = &cobra.Command{
	Use:   "walletd",
	Short: "start the wallet daemon",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting the wallet daemon")

		// Start the web services

		// Then wait for the user to stop the daemon
		for {
			time.Sleep(time.Second)
		}
	},
}
