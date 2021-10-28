package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/jsonrpc2/v15"

	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Create and get token txs",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					GetTX(args[1])
				} else {
					fmt.Println("Usage:")
					PrintTXGet()
				}
			case "history":
				if len(args) > 3 {
					GetTXHistory(args[1], args[2], args[3])
				} else {
					fmt.Println("Usage:")
					PrintAccountGet()
				}
			case "create":
				if len(args) > 3 {
					CreateTX(args[1], args[2], args[3])
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

	},
}

func init() {
	rootCmd.AddCommand(txCmd)
}

func PrintTXGet() {
	fmt.Println("  accumulate tx get [txid]			Get token transaction by txid")
}

func PrintTXCreate() {
	fmt.Println("  accumulate tx create [from] [to] [amount]	Create new token tx")
}

func PrintTXHistoryGet() {
	fmt.Println("  accumulate tx history [url] [start] [end]	Get token account history by URL given transaction start and end indices")
}

func PrintTX() {
	PrintTXGet()
	PrintTXHistoryGet()
	PrintTXCreate()
}

func GetTX(hash string) {

	var res interface{}
	var str []byte
	var hashbytes types.Bytes32

	params := new(acmeapi.TokenTxRequest)
	err := hashbytes.FromString(hash)
	if err != nil {
		log.Fatal(err)
	}

	params.Hash = hashbytes

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		log.Fatal(err)
	}

	if err := Client.Request(context.Background(), "token-tx", jsondata, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func GetTXHistory(accountUrl string, s string, e string) {

	start, err := strconv.Atoi(s)
	if err != nil {
		log.Fatal(err)
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		log.Fatal(err)
	}

	u, err := url.Parse(accountUrl)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte

	params := new(acmeapi.APIRequestURLPagination)
	params.URL = types.String(u.String())
	params.Start = int64(start)
	params.Limit = int64(end)

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		log.Fatal(err)
	}

	if err := Client.Request(context.Background(), "token-account-history", jsondata, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func CreateTX(sender string, receiver string, amount string) {

	var res interface{}
	var str []byte
	var err error

	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		pk := b.Get([]byte(sender))
		fmt.Println(hex.EncodeToString(pk))
		params := &acmeapi.APIRequestRaw{}
		params.Tx = &acmeapi.APIRequestRawTx{}

		tokentx := new(acmeapi.TokenTx)
		tokentx.From = types.UrlChain{types.String(sender)}

		to := []*acmeapi.TokenTxOutput{}
		r := &acmeapi.TokenTxOutput{}
		r.Amount, err = strconv.ParseUint(amount, 10, 64)
		r.URL = types.UrlChain{types.String(receiver)}
		to = append(to, r)
		tokentx.To = to

		data, err := json.Marshal(tokentx)
		if err != nil {
			log.Fatal(err)
		}

		datajson := json.RawMessage(data)
		params.Tx.Data = &datajson
		params.Tx.Signer = &acmeapi.Signer{}
		params.Tx.Signer.Nonce = uint64(time.Now().Unix())
		params.Tx.Sponsor = types.String(sender)
		params.Tx.KeyPage = &acmeapi.APIRequestKeyPage{}
		params.Tx.KeyPage.Height = 1
		params.Tx.KeyPage.Index = 0

		params.Tx.Sig = types.Bytes64{}

		dataBinary, err := tokentx.MarshalBinary()
		if err != nil {
			log.Fatal(err)
		}
		gtx := new(transactions.GenTransaction)
		gtx.Transaction = dataBinary //The transaction needs to be marshaled as binary for proper tx hash

		//route to the sender's account for processing
		u, err := url.Parse(sender)
		if err != nil {
			log.Fatal(err)
		}
		gtx.ChainID = u.ResourceChain()
		gtx.Routing = u.Routing()

		gtx.SigInfo = new(transactions.SignatureInfo)
		//the siginfo URL is the URL of the signer
		gtx.SigInfo.URL = sender
		//Provide a nonce, typically this will be queried from identity sig spec and incremented.
		//since SigGroups are not yet implemented, we will use the unix timestamp for now.
		gtx.SigInfo.Unused2 = params.Tx.Signer.Nonce
		//The following will be defined in the SigSpec Group for which key to use
		gtx.SigInfo.MSHeight = params.Tx.KeyPage.Height
		gtx.SigInfo.PriorityIdx = params.Tx.KeyPage.Index

		ed := new(transactions.ED25519Sig)
		err = ed.Sign(gtx.SigInfo.Unused2, pk, gtx.TransactionHash())
		if err != nil {
			return jsonrpc2.NewError(api.ErrCodeSubmission, "Submission Entry Error", err)
		}
		params.Tx.Sig.FromBytes(ed.GetSignature())
		//The public key needs to be used to verify the signature, however,
		//to pass verification, the validator will hash the key and check the
		//sig spec group to make sure this key belongs to the identity.
		params.Tx.Signer.PublicKey.FromBytes(ed.GetPublicKey())

		if err := Client.Request(context.Background(), "token-tx-create", params, &res); err != nil {
			log.Fatal(err)
		}

		str, err = json.Marshal(res)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(str))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

}
