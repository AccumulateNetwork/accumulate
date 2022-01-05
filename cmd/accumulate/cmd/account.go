package cmd

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
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
	fmt.Println("  accumulate account create token [origin adi] [signing key name] [key index (optional)] [key height (optional)] [new token account url] [tokenUrl] [keyBookUrl]	Create a token account for an ADI")
	fmt.Println("  accumulate account create data [origin adi] [signing key name] [key index (optional)] [key height (optional)] [new data account url]  [keyBookUrl]	Create a data account under an ADI")
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

	if res.Type != types.ChainTypeTokenAccount.String() && res.Type != types.ChainTypeLiteTokenAccount.String() &&
		res.Type != types.ChainTypeDataAccount.String() && res.Type != types.ChainTypeLiteDataAccount.String() {
		return "", fmt.Errorf("expecting token account or data account but received %v", res.Type)
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
func CreateAccount(origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		PrintAccountCreate()
		return "", err
	}

	args, si, privKey, err := prepareSigner(u, args)
	if len(args) < 3 {
		return "", err
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		PrintAccountCreate()
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if u.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, u.Authority)
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

	tac := protocol.CreateTokenAccount{}
	tac.Url = accountUrl.String()
	tac.TokenUrl = tok.String()
	tac.KeyBookUrl = keybook

	res, err := dispatchTxRequest("create-token-account", &tac, u, si, privKey)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
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
		lt, err := protocol.LiteTokenAddress(v.Value, protocol.AcmeUrl().String())
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
		key, _, err := protocol.ParseLiteTokenAddress(u)
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
