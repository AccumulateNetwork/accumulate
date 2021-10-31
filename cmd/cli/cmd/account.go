package cmd

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types/api/response"

	"github.com/AccumulateNetwork/accumulated/types"
	anonaddress "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Create and get token accounts",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					GetAccount(args[1])
				} else {
					fmt.Println("Usage:")
					PrintAccountGet()
				}
			case "create":
				if len(args) > 3 {
					CreateAccount(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintAccountCreate()
				}
			case "generate":
				GenerateAccount()
			case "list":
				ListAccounts()
			case "import":
				if len(args) > 1 {
					ImportAccount(args[1])
				} else {
					fmt.Println("Usage:")
					PrintAccountImport()
				}
			case "export":
				if len(args) > 1 {
					ExportAccount(args[1])
				} else {
					fmt.Println("Usage:")
					PrintAccountExport()
				}
			default:
				fmt.Println("Usage:")
				PrintAccount()
			}
		} else {
			fmt.Println("Usage:")
			PrintAccount()
		}

	},
}

func init() {
	rootCmd.AddCommand(accountCmd)
}

func PrintAccountGet() {
	fmt.Println("  accumulate account get [url]			Get anon token account by URL")
}

func PrintAccountGenerate() {
	fmt.Println("  accumulate account generate			Generate random anon token account")
}

func PrintAccountList() {
	fmt.Println("  accumulate account list			Display all anon token accounts")
}

func PrintAccountCreate() {
	fmt.Println("  accumulate account create [{actor adi}] [wallet key label] [key index (optional)] [key height (optional)] [token account url] [tokenUrl] [keyBook (optional)]	Create a token account for an ADI")
}

func PrintAccountImport() {
	fmt.Println("  accumulate account import [private-key]	Import anon token account from private key hex")
}

func PrintAccountExport() {
	fmt.Println("  accumulate account export [url]		Export private key hex of anon token account")
}

func PrintAccount() {
	PrintAccountGet()
	PrintAccountGenerate()
	PrintAccountList()
	PrintAccountCreate()
	PrintAccountImport()
	PrintAccountExport()
}

func GetAccount(url string) {

	var res interface{}
	var str []byte

	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "token-account", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

//account create adiActor labelOrPubKeyHex height index tokenUrl keyBookUrl
func CreateAccount(url string, args []string) {

	actor, err := url2.Parse(url)
	if err != nil {
		PrintAccountCreate()
		log.Fatal(err)
	}

	args, si, privKey, err := prepareSigner(actor, args)
	if len(args) < 3 {
		PrintAccountCreate()
		log.Fatal("insufficient number of command line arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		PrintAccountCreate()
		log.Fatalf("invalid account url %s", args[0])
	}
	if actor.Authority != accountUrl.Authority {
		log.Fatalf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, actor.Authority)
	}
	tok, err := url2.Parse(args[1])
	if err != nil {
		log.Fatal("invalid token url")
	}

	kbu, err := url2.Parse(args[2])
	if err != nil {
		log.Fatal("invalid key book url")
	}

	//make sure this is a valid token account
	tokenJson := Get(tok.String())
	token := response.Token{}
	err = json.Unmarshal([]byte(tokenJson), &token)
	if err != nil {
		PrintAccountCreate()
		log.Fatal(fmt.Errorf("invalid token type %v", err))
	}

	tac := &protocol.TokenAccountCreate{}
	tac.Url = accountUrl.String()
	tac.TokenUrl = tok.String()
	tac.KeyBookUrl = kbu.String()

	binaryData, err := tac.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	jsonData, err := json.Marshal(&tac)
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())

	params, err := prepareGenTx(jsonData, binaryData, actor, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "token-account-create", params, &res); err != nil {
		//todo: if we fail, then we need to remove the adi from storage or keep it and try again later...
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))
}

func GenerateAccount() {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal(err)
	}

	address := anonaddress.GenerateAcmeAddress(pubKey)

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		err := b.Put([]byte(address), privKey)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(address)
}

func ImportAccount(pkhex string) {

	var pk ed25519.PrivateKey

	token, err := hex.DecodeString(pkhex)
	if err != nil {
		log.Fatal(err)
	}

	pk = token
	address := anonaddress.GenerateAcmeAddress(pk.Public().(ed25519.PublicKey))

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		err := b.Put([]byte(address), token)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(address)

}

func ExportAccount(url string) {

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		pk := b.Get([]byte(url))
		fmt.Println(hex.EncodeToString(pk))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

}

func ListAccounts() {

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			fmt.Printf("%s\n", k)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

}
