package cmd

import (
	//	"context"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"time"

	"github.com/tendermint/tendermint/crypto/ed25519"

	apiv2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	tapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/spf13/cobra"
)

// WORK IN PROGRESS

var stakingCmd = &cobra.Command{
	Use:   "staking",
	Short: "Staking and Validator commands",
	Run: func(cmd *cobra.Command, args []string) {
		var out string

		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get-validator":
				if len(args) > 1 {
					fmt.Println(out)

				} else {
					fmt.Println("Usage:")
					PrintValGet()
				}
			case "create-validator":
				if len(args) > 5 {
					out, err = CreateVal(args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10])
					if err != nil {
						fmt.Print(err)
					}
				} else {
					fmt.Println("Usage:")
					PrintCreateValidator()
				}
			default:
				fmt.Printf("\nTo create a Validator, you need to pass in moniker, publicKey, amountStaking, commission, and validatorAddress\n\n")
			}
		} else {
			fmt.Println("Usage")
			PrintValidators()

		}
		//PrintCreateValidator()

		fmt.Println("\n****************************************************************** CREATING VALIDATOR ***********************************************************************************\n",
		 string(out),
		  "\n*************************************************************************************************************************************************************************")
		},
}

func PrintCreateValidator() {
	fmt.Println("  accumulate staking create-validator [moniker] [identity] [website] [details] [commission] [validator-address] [amount] [pubkey]")
	fmt.Println("\t\t example usage: accumulate staking create-validator myValName...")
}

func PrintValGet() {
	fmt.Println("  accumulate staking get-validator [moniker]						Get validator by URL")
}

func PrintValidators() {
	PrintCreateValidator()
	PrintValGet()
}



// CreateValidator create a new validator
func CreateVal(valAddr,  pubKey, moniker, identity, website, details, amount , rate, maxR , maxCr string) (string, error) {
	kp := types.CreateKeyPair()
	adiUrl := "acc://Mohammed"
	var res apiv2.TxResponse

 	number, _ := strconv.ParseUint(rate, 10, 64)
	numberr, _ := strconv.ParseUint(maxR, 10, 64)
 	numberrr, _ := strconv.ParseUint(maxCr, 10, 64)
	r := number
	rr := numberr
	rrr := numberrr
	py, _ := pubKeyFromString(pubKey)

	message, err := createVal(valAddr , py, moniker, identity, website, details, r, rr, rrr)
	if err != nil {
		log.Fatal(err)
	}

	params := createRequest(adiUrl, &kp, message)
	req := &tapi.APIRequestRaw{}
	if err = json.Unmarshal(params, &req); err != nil {
		log.Fatal(err)
	}
	
	data := &protocol.CreateValidator{}
	
	err = json.Unmarshal(*req.Tx.Data, data)
	if err != nil {
		log.Fatal(err)
	}
	
	qr := new(apiv2.TxRequest)
	qr.Signer.PublicKey = req.Tx.Signer.PublicKey.Bytes()
	qr.Signer.Nonce = req.Tx.Signer.Nonce
	qr.Signature = req.Tx.Sig[:]
	qr.KeyPage.Height = req.Tx.KeyPage.Height
	qr.Payload = req.Tx.Data
	
	pu, _ := url2.Parse(string(req.Tx.Origin))
	qr.Origin = pu
	
	if err = json.Unmarshal(params, &qr); err != nil {
		log.Fatal(err)
	}
	
	err = json.Unmarshal(params, &qr)
	if err != nil {
		log.Fatal(err)
	}
	rez := &tapi.APIDataResponse{}
	if err = json.Unmarshal(params, &rez); err != nil {
		log.Fatal(err)
	}
	
	err = Client.Request(context.Background(), "create-validator", &qr, &res)
	
	str, err := json.Marshal(&qr)
	if err != nil {
		log.Fatal(err)
	}

	 return string(str), err
	}

	
func createVal(operator string, pubkey []byte, moniker string, identity string, website string, details string, rate uint64, maxR uint64, maxCr uint64) (string, error) {
		data := &protocol.CreateValidator{}
	
		data.PubKey = pubkey
	//	data.Validator{}
		data.Moniker = moniker
		data.Identity = identity
		data.Website = website 
		data.Details = details
		data.Commission.CommissionRates.Rate = rate
		data.Commission.CommissionRates.MaxRate = maxR
		data.Commission.CommissionRates.MaxChangeRate = maxCr
	
		ret, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
	
		return string(ret), nil
	}

func createRequest(adiUrl string, kp *ed25519.PrivKey, message string) []byte {
		req := &tapi.APIRequestRaw{}
	
		req.Tx = &tapi.APIRequestRawTx{}
		// make a raw json message.
		raw := json.RawMessage(message)
	
		//Set the message data. Making it a json.RawMessage will prevent go from unmarshalling it which
		//allows us to verify the signature against it.
		req.Tx.Data = &raw
		req.Tx.Signer = &tapi.Signer{}
		req.Tx.Signer.Nonce = uint64(time.Now().Unix())
		req.Tx.Origin = types.String(adiUrl)
		req.Tx.KeyPage = &tapi.APIRequestKeyPage{}
		req.Tx.KeyPage.Height = 1
		copy(req.Tx.Signer.PublicKey[:], kp.PubKey().Bytes())
	
		//form the ledger for signing
		ledger := types.MarshalBinaryLedgerAdiChainPath(adiUrl, []byte(message), int64(req.Tx.Signer.Nonce))
	
		//sign it...
		sig, err := kp.Sign(ledger)
	
		//store the signature
		copy(req.Tx.Sig[:], sig)
	
		//make the json for submission to the jsonrpc
		params, err := json.Marshal(&req)
		if err != nil {
		 log.Fatal(err)
		}
	
		return params
	}