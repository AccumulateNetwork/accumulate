package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
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
	TxWaitSynth time.Duration
)

func init() {
	txCmd.Flags().DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	txCmd.Flags().DurationVar(&TxWaitSynth, "wait-synth", 0, "Wait for synthetic transactions to complete")
}

func PrintTXGet() {
	fmt.Println("  accumulate tx get [txid]			Get token transaction by txid")
}

func PrintTXCreate() {
	fmt.Println("  accumulate tx create [from] [to] [amount]	Create new token tx")
}

func PrintTXHistoryGet() {
	fmt.Println("  accumulate tx history [url] [starting transaction number] [ending transaction number]	Get transaction history")
}

func PrintTX() {
	PrintTXGet()
	PrintTXCreate()
	PrintTXHistoryGet()
}

func getTX(hash []byte, wait time.Duration) (*api2.QueryResponse, error) {
	var res api2.QueryResponse
	var err error

	params := new(api2.TxnQuery)
	params.Txid = hash

	if wait > 0 {
		params.Wait = wait
	}

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		return nil, err
	}

	err = Client.RequestV2(context.Background(), "query-tx", jsondata, &res)
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

	t := Client.Timeout
	defer func() { Client.Timeout = t }()

	if TxWait > 0 {
		Client.Timeout = TxWait * 2
	}

	res, err := getTX(txid, TxWait)
	if err != nil {
		var rpcErr jsonrpc2.Error
		if errors.As(err, &rpcErr) {
			return PrintJsonRpcError(err)
		}
		return "", err
	}

	out, err := PrintQueryResponseV2(res)
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

			o, err := PrintQueryResponseV2(res)
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

	var res acmeapi.APIDataResponsePagination

	params := new(acmeapi.APIRequestURLPagination)
	params.URL = types.String(u.String())
	params.Start = int64(start)
	params.Limit = int64(end)

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		return "", err
	}

	if err := Client.Request(context.Background(), "token-account-history", jsondata, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	if WantJsonOutput {
		data, err := json.Marshal(res)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var out string
	for i := range res.Data {
		s, err := PrintQueryResponse(res.Data[i])
		if err != nil {
			return "", err
		}
		out += s
	}
	return out, err
}

func CreateTX(sender string, args []string) (string, error) {
	//sender string, receiver string, amount string
	var res api2.TxResponse
	var err error
	u, err := url.Parse(sender)
	if err != nil {
		return "", err
	}

	args, si, pk, err := prepareSigner(u, args)

	if len(args) < 2 {
		PrintTXCreate()
		return "", fmt.Errorf("invalid number of arguments for tx create")
	}

	u2, err := url.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid receiver url %s, %v", args[0], err)
	}
	amount := args[1]

	//fmt.Println(hex.EncodeToString(pk))
	tokentx := new(acmeapi.SendTokens)
	tokentx.From = types.UrlChain{String: types.String(u.String())}

	to := []*acmeapi.TokenRecipient{}
	r := &acmeapi.TokenRecipient{}

	amt, err := strconv.ParseFloat(amount, 64)
	r.Amount = uint64(amt * 1e8)
	r.URL.String = types.String(u2.String())
	to = append(to, r)
	tokentx.To = to

	data, err := json.Marshal(tokentx)
	if err != nil {
		return "", err
	}

	dataBinary, err := tokentx.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, u, si, pk, nonce)
	if err != nil {
		return "", err
	}

	if err := Client.RequestV2(context.Background(), "send-tokens", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFrom(&res).Print()
}
