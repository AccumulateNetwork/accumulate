package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/spf13/cobra"
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

func GetTX(hash string) (string, error) {

	var res acmeapi.APIDataResponse
	var hashbytes types.Bytes32

	params := new(acmeapi.TokenTxRequest)
	err := hashbytes.FromString(hash)
	if err != nil {
		return "", err
	}
	params.Hash = hashbytes

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		return "", err
	}

	if err := Client.Request(context.Background(), "token-tx", jsondata, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponse(&res)
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
	var res acmeapi.APIDataResponse
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
	tokentx := new(acmeapi.TokenTx)
	tokentx.From = types.UrlChain{types.String(u.String())}

	to := []*acmeapi.TokenTxOutput{}
	r := &acmeapi.TokenTxOutput{}

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

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, u, si, pk, nonce)
	if err != nil {
		return "", err
	}

	if err := Client.Request(context.Background(), "token-tx-create", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result")
	}
	return ar.Print()
}
