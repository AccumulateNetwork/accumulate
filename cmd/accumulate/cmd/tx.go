package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"golang.org/x/sync/errgroup"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Create and get token txs",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					out, err = GetTX(args[1])
				} else {
					fmt.Println("Usage:")
					PrintTXGet()
				}
			case "pending":
				if len(args) == 1 {
					out, err = GetPendingTx(args[1], []string{})
				} else {
					out, err = GetPendingTx(args[1], args[2:])
				}
			case "history":
				if len(args) > 3 {
					out, err = GetTXHistory(args[1], args[2], args[3])
				} else {
					fmt.Println("Usage:")
					PrintTXHistoryGet()
				}
			case "create":
				if len(args) > 3 {
					out, err = CreateTX(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintTXCreate()
				}
			case "execute":
				if len(args) > 2 {
					out, err = ExecuteTX(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintTXExecute()
				}
			case "sign":
				if len(args) > 2 {
					out, err = SignTX(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintTxSign()
				}
			default:
				fmt.Println("Usage:")
				PrintTX()
			}
		} else {
			fmt.Println("Usage:")
			PrintTX()
		}
		printOutput(cmd, out, err)
	},
}

var (
	TxWait      time.Duration
	TxNoWait    bool
	TxWaitSynth time.Duration
)

func init() {
	txCmd.Flags().DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	txCmd.Flags().DurationVar(&TxWaitSynth, "wait-synth", 0, "Wait for synthetic transactions to complete")
}

func PrintTXGet() {
	fmt.Println("  accumulate tx get [txid]			Get token transaction by txid")
}

func PrintTXPendingGet() {
	fmt.Println("  accumulate tx pending [txid]			Get token transaction by txid")
	fmt.Println("  accumulate tx pending [height]			Get token transaction by block height")
	fmt.Println("  accumulate tx pending [starting transaction number]	[ending transaction number]		Get token transaction by beginning and ending height")
}

func PrintTXCreate() {
	fmt.Println("  accumulate tx create [from] [to] [amount]	Create new token tx")
}

func PrintTXExecute() {
	fmt.Println("  accumulate tx execute [from] [payload]	Execute an arbitrary transaction")
}

func PrintTxSign() {
	fmt.Println("  accumulate tx sign [origin] [signing key name] [key index (optional)] [key height (optional)] [txid]	Sign a pending transaction")
}

func PrintTXHistoryGet() {
	fmt.Println("  accumulate tx history [url] [starting transaction number] [ending transaction number]	Get transaction history")
}

func PrintTX() {
	PrintTXGet()
	PrintTXCreate()
	PrintTXExecute()
	PrintTxSign()
	PrintTXHistoryGet()
	PrintTXPendingGet()
}
func GetPendingTx(origin string, args []string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	//<record>#pending/<hash> - fetch an envelope by hash
	//<record>#pending/<index> - fetch an envelope by index/height
	//<record>#pending/<start>:<end> - fetch a range of envelope by index/height
	//build the fragments:
	params := api2.UrlQuery{}

	var out string
	var perr error
	switch len(args) {
	case 0:
		//query with no parameters
		u.Fragment = "pending"
		params.Url = u
		res := api2.MultiResponse{}
		err = queryAs("query", &params, &res)
		if err != nil {
			return "", err
		}
		out, perr = PrintMultiResponse(&res)
	case 1:
		if len(args[0]) == 64 {
			//this looks like a transaction hash, now check it
			txid, err := hex.DecodeString(args[0])
			if err != nil {
				return "", fmt.Errorf("cannot decode transaction id")
			}
			u.Fragment = fmt.Sprintf("pending/%x", txid)
		} else {
			height, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return "", fmt.Errorf("expecting height, but could not convert argument, %v", err)
			}
			u.Fragment = fmt.Sprintf("pending/%d", height)
		}
		params.Url = u
		res := api2.TransactionQueryResponse{}
		err = queryAs("query", &params, &res)
		if err != nil {
			return "", err
		}
		out, perr = PrintTransactionQueryResponseV2(&res)
	case 2:
		//pagination
		start, err := strconv.Atoi(args[0])
		if err != nil {
			return "", fmt.Errorf("error converting start index %v", err)
		}
		count, err := strconv.Atoi(args[1])
		if err != nil {
			return "", fmt.Errorf("error converting count %v", err)
		}
		u.Fragment = fmt.Sprintf("pending/%d:%d", start, count)
		params.Url = u
		res := api2.MultiResponse{}
		err = queryAs("query", &params, &res)
		if err != nil {
			return "", err
		}
		out, perr = PrintMultiResponse(&res)
	default:
		return "", fmt.Errorf("invalid number of arguments")
	}

	return out, perr
}
func getTX(hash []byte, wait time.Duration) (*api2.TransactionQueryResponse, error) {
	var res api2.TransactionQueryResponse
	var err error

	params := new(api2.TxnQuery)
	params.Txid = hash
	params.Prove = TxProve

	if wait > 0 {
		params.Wait = wait

		t := Client.Timeout
		defer func() { Client.Timeout = t }()

		Client.Timeout = TxWait * 2
	}

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		return nil, err
	}

	err = Client.RequestAPIv2(context.Background(), "query-tx", jsondata, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func GetTX(hash string) (string, error) {
	txid, err := hex.DecodeString(hash)
	if err != nil {
		return "", err
	}

	res, err := getTX(txid, TxWait)
	if err != nil {
		var rpcErr jsonrpc2.Error
		if errors.As(err, &rpcErr) {
			return PrintJsonRpcError(err)
		}
		return "", err
	}

	out, err := PrintTransactionQueryResponseV2(res)
	if err != nil {
		return "", err
	}

	if TxWaitSynth == 0 || len(res.SyntheticTxids) == 0 {
		return out, nil
	}

	if TxWaitSynth > 0 {
		Client.Timeout = TxWaitSynth * 2
	}

	errg := new(errgroup.Group)
	for _, txid := range res.SyntheticTxids {
		txid := txid // Do not capture the loop variable in the closure
		errg.Go(func() error {
			res, err := getTX(txid[:], TxWaitSynth)
			if err != nil {
				return err
			}

			o, err := PrintTransactionQueryResponseV2(res)
			if err != nil {
				return err
			}

			if WantJsonOutput {
				out += "\n"
			}
			out += o
			return nil
		})
	}
	err = errg.Wait()
	if err != nil {
		var rpcErr jsonrpc2.Error
		if errors.As(err, &rpcErr) {
			return PrintJsonRpcError(err)
		}
		return PrintJsonRpcError(err)
	}

	return out, nil
}

func GetTXHistory(accountUrl string, s string, e string) (string, error) {
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

	params := new(api2.TxHistoryQuery)
	params.UrlQuery.Url = u
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)

	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}

	if err := Client.RequestAPIv2(context.Background(), "query-tx-history", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintMultiResponse(&res)
}

func CreateTX(sender string, args []string) (string, error) {
	//sender string, receiver string, amount string
	u, err := url.Parse(sender)
	if err != nil {
		return "", err
	}

	args, si, pk, err := prepareSigner(u, args)

	if len(args) < 2 {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}

	tokenUrl, err := GetTokenUrlFromAccount(u)
	if err != nil {
		return "", fmt.Errorf("invalid token url was obtained from %s, %v", u.String(), err)
	}

	u2, err := url.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid receiver url %s, %v", args[0], err)
	}

	amount := args[1]
	send := new(protocol.SendTokens)

	amt, err := amountToBigInt(tokenUrl.String(), amount)
	if err != nil {
		return "", err
	}

	send.AddRecipient(u2, amt)

	res, err := dispatchTxRequest("send-tokens", send, nil, u, si, pk)
	if err != nil {
		return "", err
	}
	if !TxNoWait && TxWait > 0 {
		_, err := waitForTxn(res.TransactionHash, TxWait)
		if err != nil {
			var rpcErr jsonrpc2.Error
			if errors.As(err, &rpcErr) {
				return PrintJsonRpcError(err)
			}
			return "", err
		}
	}
	return ActionResponseFrom(res).Print()
}

func waitForTxn(hash []byte, wait time.Duration) (*api2.TransactionQueryResponse, error) {
	queryRes, err := getTX(hash, wait)
	if err != nil {
		return nil, err
	}
	if queryRes.SyntheticTxids != nil {
		for _, txid := range queryRes.SyntheticTxids {
			_, err := waitForTxn(txid[:], wait)
			if err != nil {
				return nil, err
			}
		}
	}
	return queryRes, nil
}

func ExecuteTX(sender string, args []string) (string, error) {
	//sender string, receiver string, amount string
	u, err := url.Parse(sender)
	if err != nil {
		return "", err
	}

	args, si, pk, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}

	var typ struct {
		Type types.TransactionType
	}
	err = json.Unmarshal([]byte(args[0]), &typ)
	if err != nil {
		return "", fmt.Errorf("invalid payload 1: %v", err)
	}

	txn, err := protocol.NewTransaction(typ.Type)
	if err != nil {
		return "", fmt.Errorf("invalid payload 2: %v", err)
	}

	err = json.Unmarshal([]byte(args[0]), txn)
	if err != nil {
		return "", fmt.Errorf("invalid payload 3: %v", err)
	}

	res, err := dispatchTxRequest("execute", txn, nil, u, si, pk)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}

func SignTX(sender string, args []string) (string, error) {
	//sender string, receiver string, amount string
	u, err := url.Parse(sender)
	if err != nil {
		return "", err
	}

	args, si, pk, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}
	if len(args) != 1 {
		PrintTxSign()
		return "", nil
	}

	txHash, err := hex.DecodeString(args[0])
	if err != nil {
		return "", fmt.Errorf("unable to parse transaction hash: %v", err)
	}

	res, err := dispatchTxRequest("execute", nil, txHash, u, si, pk)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}
