package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

var blocksCmd = &cobra.Command{
	Use:   "blocks",
	Short: "Create and get blocks",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "minor": // TODO refine CLI syntax
				if len(args) > 3 {
					out, err = GetMinorBlocks(args[1], args[2], args[3])
				} else {
					fmt.Println("Usage:")
					PrintGetMinorBLocks()
				}
			default:
				fmt.Println("Usage:")
				PrintBlocks()
			}
		} else {
			fmt.Println("Usage:")
			PrintBlocks()
		}
		printOutput(cmd, out, err)
	},
}

var (
	BlocksWait      time.Duration
	BlocksNoWait    bool
	BlocksWaitSynth time.Duration
)

func init() {
	blocksCmd.Flags().DurationVarP(&BlocksWait, "wait", "w", 0, "Wait for the transaction to complete")
	blocksCmd.Flags().DurationVar(&BlocksWaitSynth, "wait-synth", 0, "Wait for synthetic transactions to complete")
}

func PrintGetMinorBLocks() {
	fmt.Println("  accumulate blocks minor [url] [start index] [count]	Get minor blocks")
}

func PrintBlocks() {
	PrintGetMinorBLocks()
}

func GetMinorBlocks(accountUrl string, s string, e string) (string, error) {
	var res api2.MultiResponse
	start, err := strconv.Atoi(s)
	if err != nil {
		return "", err
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	params := new(api2.MinorBlocksQuery)
	params.UrlQuery.Url = u
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)

	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}

	if err := Client.RequestAPIv2(context.Background(), "query-minor-blocks", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintMultiResponse(&res)
}
