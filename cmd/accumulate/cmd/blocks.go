package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
		var err error
		var txFetchMode protocol.TxFetchMode
		var filterAnBlks bool
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "minor": // TODO refine CLI syntax
				if len(args) > 3 {
					txFetchMode, err = parseFetchMode(args)
					if err != nil {
						printError(cmd, err)
						return
					}
					filterAnBlks, err = parseFilterSynthAnchorOnlyBlocks(args)
					if err != nil {
						printError(cmd, err)
						return
					}
					err = GetMinorBlocks(cmd, args[1], args[2], args[3], txFetchMode, filterAnBlks)
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
	},
}

func parseFetchMode(args []string) (protocol.TxFetchMode, error) {
	if len(args) > 4 {
		txFetchMode, ok := protocol.TxFetchModeByName(args[4])
		if ok {
			return txFetchMode, nil
		} else {
			return protocol.TxFetchModeOmit, fmt.Errorf("%s is not a valid fetch mode. Use expand|ids|countOnly|omit", args[4])
		}
	}
	return protocol.TxFetchModeExpand, nil
}

func parseFilterSynthAnchorOnlyBlocks(args []string) (bool, error) {
	if len(args) > 5 {
		val, err := strconv.ParseBool(args[5])
		if err != nil {
			return false, err
		}
		return val, nil
	}
	return false, nil
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
	fmt.Println("  accumulate blocks minor [subnet-url] [start index] [count] [tx fetch mode expand|ids|countOnly|omit (optional)] [filter synth-anchors only blocks true|false (optional)] Get minor blocks")
}

func PrintBlocks() {
	PrintGetMinorBLocks()
}

func GetMinorBlocks(cmd *cobra.Command, accountUrl string, s string, e string, txFetchMode protocol.TxFetchMode, filterAnBlks bool) error {
	var res api2.MultiResponse
	start, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		return err
	}

	u, err := url.Parse(accountUrl)
	if err != nil {
		return err
	}

	params := new(api2.MinorBlocksQuery)
	params.UrlQuery.Url = u
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)
	params.TxFetchMode = txFetchMode
	params.FilterSynthAnchorsOnlyBlocks = filterAnBlks

	data, err := json.Marshal(params)
	if err != nil {
		return err
	}

	// Temporary increase timeout, we may get a large result set which takes a while to construct
	globalTimeout := Client.Timeout
	Client.Timeout = 2 * time.Minute // TODO configuration option or param?
	defer func() {
		Client.Timeout = globalTimeout
	}()

	if err := Client.RequestAPIv2(context.Background(), "query-minor-blocks", json.RawMessage(data), &res); err != nil {
		cmd.Println(PrintJsonRpcError(err))
		return err
	}

	FPrintMultiResponse(cmd.OutOrStderr(), &res)
	return nil
}
