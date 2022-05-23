package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

const minorBlockApiTimeout = 2 * time.Minute

var blocksCmd = &cobra.Command{
	Use:   "blocks",
	Short: "Create and get blocks",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		var txFetchMode query.TxFetchMode
		var blockFilterMode query.BlockFilterMode

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "minor":
				if len(args) > 3 {
					txFetchMode, err = parseFetchMode(args)
					if err != nil {
						printError(cmd, err)
						return
					}
					blockFilterMode, err = parseBlockFilterMode(args)
					if err != nil {
						printError(cmd, err)
						return
					}

					if strings.EqualFold(args[1], "all-networks") {
						err = GetMinorBlocksFromDN(cmd, args[2], args[3], txFetchMode, blockFilterMode)
						if err != nil {
							printError(cmd, err)
							return
						}
					} else {
						accUrl, err := url.Parse(args[1])
						if err != nil {
							printError(cmd, err)
							return
						}

						err = GetMinorBlocksByUrl(cmd, accUrl, args[2], args[3], txFetchMode, blockFilterMode)
						if err != nil {
							printError(cmd, err)
							return
						}
					}
				} else {
					fmt.Println("Usage:")
					PrintGetMinorBlocks()
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

func parseFetchMode(args []string) (query.TxFetchMode, error) {
	if len(args) > 4 {
		txFetchMode, ok := query.TxFetchModeByName(args[4])
		if ok {
			return txFetchMode, nil
		} else {
			return query.TxFetchModeOmit, fmt.Errorf("%s is not a valid fetch mode. Use expand|ids|countOnly|omit", args[4])
		}
	}
	return query.TxFetchModeExpand, nil
}

func parseBlockFilterMode(args []string) (query.BlockFilterMode, error) {
	if len(args) > 5 {
		blockFilterMode, ok := query.BlockFilterModeByName(args[5])
		if ok {
			return blockFilterMode, nil
		} else {
			return query.BlockFilterModeExcludeNone, fmt.Errorf("%s is not a block filter mode. Use excludenone|excludeempty", args[4])
		}
	}
	return query.BlockFilterModeExcludeNone, nil
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

func PrintGetMinorBlocks() {
	fmt.Println("  accumulate blocks minor [subnet-url] [start index] [count] [tx fetch mode expand|ids|countOnly|omit (optional)] [block filter mode excludenone|excludeempty (optional)] Get minor blocks")
	fmt.Println("  accumulate blocks minor all-networks [start index] [count] [tx fetch mode expand|ids|countOnly|omit (optional)] [block filter mode excludenone|excludeempty (optional)] Get minor blocks")
}

func PrintBlocks() {
	PrintGetMinorBlocks()
}

func GetMinorBlocksByUrl(cmd *cobra.Command, accountUrl *url.URL, s string, e string, txFetchMode query.TxFetchMode, blockFilterMode query.BlockFilterMode) error {
	start, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		return err
	}

	params := new(api2.MinorBlocksByUrlQuery)
	params.UrlQuery.Url = accountUrl
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)
	params.TxFetchMode = txFetchMode
	params.BlockFilterMode = blockFilterMode

	// Temporary increase timeout, we may get a large result set which takes a while to construct
	globalTimeout := Client.Timeout
	Client.Timeout = minorBlockApiTimeout
	defer func() {
		Client.Timeout = globalTimeout
	}()

	res, err := Client.QueryMinorBlocksByUrl(context.Background(), params)
	if err != nil {
		rpcError, err := PrintJsonRpcError(err)
		cmd.Println(rpcError)
		return err
	}

	out, err := PrintMultiResponse(res)
	if err != nil {
		return err
	}

	printOutput(cmd, out, nil)
	return nil
}

func GetMinorBlocksFromDN(cmd *cobra.Command, s string, e string, txFetchMode query.TxFetchMode, blockFilterMode query.BlockFilterMode) error {
	start, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		return err
	}

	params := new(api2.MinorBlocksQuery)
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)
	params.TxFetchMode = txFetchMode
	params.BlockFilterMode = blockFilterMode

	// Temporary increase timeout, we may get a large result set which takes a while to construct
	globalTimeout := Client.Timeout
	Client.Timeout = minorBlockApiTimeout
	defer func() {
		Client.Timeout = globalTimeout
	}()

	res, err := Client.QueryMinorBlocksFromDN(context.Background(), params)
	if err != nil {
		rpcError, err := PrintJsonRpcError(err)
		cmd.Println(rpcError)
		return err
	}

	out, err := PrintMultiResponse(res)
	if err != nil {
		return err
	}

	printOutput(cmd, out, nil)
	return nil
}
