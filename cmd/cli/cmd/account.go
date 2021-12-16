package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/mdp/qrterminal"
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Create and get token accounts",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					out, err = GetAccount(args[1])
				} else {
					fmt.Println("Usage:")
					PrintAccountGet()
				}
			case "create":
				if len(args) > 4 {
					switch args[1] {
					case "token":
						out, err = CreateAccount(args[2], args[3:])
					case "data":
						out, err = CreateDataAccount(args[2], args[3:])
					default:
						fmt.Printf("Deprecation Warning!\nTo create a token account, in future please specify either \"token\" or \"data\"\n\n")
						//this will be removed in future release and replaced with usage: PrintAccountCreate()
						out, err = CreateAccount(args[1], args[2:])
					}
				} else {
					fmt.Println("Usage:")
					PrintAccountCreate()
				}
			case "qr":
				if len(args) > 1 {
					out, err = QrAccount(args[1])
				} else {
					fmt.Println("Usage:")
					PrintAccountQr()
				}
			case "generate":
				out, err = GenerateAccount()
			case "list":
				out, err = ListAccounts()
			case "restore":
				out, err = RestoreAccounts()
			default:
				fmt.Println("Usage:")
				PrintAccount()
			}
		} else {
			fmt.Println("Usage:")
			PrintAccount()
		}
		printOutput(cmd, out, err)
	},
}

func PrintAccountGet() {
	fmt.Println("  accumulate account get [url]			Get lite token account by URL")
}

func PrintAccountQr() {
	fmt.Println("  accumulate account qr [url]			Display QR code for lite account URL")
}

func PrintAccountGenerate() {
	fmt.Println("  accumulate account generate			Generate random lite token account")
}

func PrintAccountList() {
	fmt.Println("  accumulate account list			Display all lite token accounts")
}

func PrintAccountRestore() {
	fmt.Println("  accumulate account restore			Restore old lite token accounts")
}

func PrintAccountCreate() {
	fmt.Println("  accumulate account create token [actor adi] [signing key name] [key index (optional)] [key height (optional)] [new token account url] [tokenUrl] [keyBookUrl]	Create a token account for an ADI")
	fmt.Println("  accumulate account create data [actor adi] [signing key name] [key index (optional)] [key height (optional)] [new data account url]  [keyBookUrl]	Create a data account under an ADI")
}

func PrintAccountImport() {
	fmt.Println("  accumulate account import [private-key]	Import lite token account from private key hex")
}

func PrintAccountExport() {
	fmt.Println("  accumulate account export [url]		Export private key hex of lite token account")
}

func PrintAccount() {
	PrintAccountGet()
	PrintAccountQr()
	PrintAccountGenerate()
	PrintAccountList()
	PrintAccountRestore()
	PrintAccountCreate()
	PrintAccountImport()
	PrintAccountExport()
}

func GetAccount(url string) (string, error) {
	res, err := GetUrl(url)
	if err != nil {
		return "", err
	}

	return PrintQueryResponseV2(res)
}

func QrAccount(s string) (string, error) {
	u, err := url2.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid Accumulate URL: %v\n", s, err)
	}

	b := bytes.NewBufferString("")
	qrterminal.GenerateWithConfig(u.String(), qrterminal.Config{
		Level:          qrterminal.M,
		Writer:         b,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteChar:      qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      2,
	})

	r, err := ioutil.ReadAll(b)
	return string(r), err
}

//CreateAccount account create url labelOrPubKeyHex height index tokenUrl keyBookUrl
func CreateAccount(url string, args []string) (string, error) {
	actor, err := url2.Parse(url)
	if err != nil {
		PrintAccountCreate()
		return "", err
	}

	args, si, privKey, err := prepareSigner(actor, args)
	if len(args) < 3 {
		PrintAccountCreate()
		return "", fmt.Errorf("insufficient number of command line arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		PrintAccountCreate()
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if actor.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, actor.Authority)
	}
	tok, err := url2.Parse(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid token url")
	}

	var keybook string
	if len(args) > 2 {
		kbu, err := url2.Parse(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid key book url")
		}
		keybook = kbu.String()
	}

	//make sure this is a valid token account
	tokenJson, err := Get(tok.String())
	if err != nil {
		return "", err
	}
	token := protocol.TokenIssuer{}
	err = json.Unmarshal([]byte(tokenJson), &token)
	if err != nil {
		PrintAccountCreate()
		return "", fmt.Errorf("invalid token type %v", err)
	}

	tac := &protocol.TokenAccountCreate{}
	tac.Url = accountUrl.String()
	tac.TokenUrl = tok.String()
	tac.KeyBookUrl = keybook

	binaryData, err := tac.MarshalBinary()
	if err != nil {
		return "", err
	}

	jsonData, err := json.Marshal(&tac)
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()

	params, err := prepareGenTxV2(jsonData, binaryData, actor, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.TxResponse
	if err := Client.RequestV2(context.Background(), "create-token-account", params, &res); err != nil {
		//todo: if we fail, then we need to remove the adi from storage or keep it and try again later...
		return "", err
	}

	ar := ActionResponseFrom(&res)
	return ar.Print()
}

func GenerateAccount() (string, error) {
	return GenerateKey("")
}

func ListAccounts() (string, error) {

	b, err := Db.GetBucket(BucketLabel)
	if err != nil {
		//no accounts so nothing to do...
		return "", err
	}
	var out string
	for _, v := range b.KeyValueList {
		lt, err := protocol.LiteAddress(v.Value, protocol.AcmeUrl().String())
		if err != nil {
			continue
		}
		if lt.String() == string(v.Key) {
			out += fmt.Sprintf("%s\n", v.Key)
		}
	}
	//TODO: this probably should also list out adi accounts as well
	return out, nil
}

func RestoreAccounts() (out string, err error) {
	anon, err := Db.GetBucket(BucketAnon)
	if err != nil {
		//no anon accounts so nothing to do...
		return
	}
	for _, v := range anon.KeyValueList {
		u, err := url2.Parse(string(v.Key))
		if err != nil {
			out += fmt.Sprintf("%q is not a valid URL\n", v.Key)
		}
		key, _, err := protocol.ParseLiteAddress(u)
		if err != nil {
			out += fmt.Sprintf("%q is not a valid lite account: %v\n", v.Key, err)
		} else if key == nil {
			out += fmt.Sprintf("%q is not a lite account\n", v.Key)
		}

		privKey := ed25519.PrivateKey(v.Value)
		pubKey := privKey.Public().(ed25519.PublicKey)
		out += fmt.Sprintf("Converting %s : %x\n", v.Key, pubKey)

		err = Db.Put(BucketLabel, v.Key, pubKey)
		if err != nil {
			log.Fatal(err)
		}
		err = Db.Put(BucketKeys, pubKey, privKey)
		if err != nil {
			return "", err
		}
		err = Db.DeleteBucket(BucketAnon)
		if err != nil {
			return "", err
		}
	}
	return out, nil
}
