package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/boltdb/bolt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var adiCmd = &cobra.Command{
	Use:   "adi",
	Short: "Create and manage ADI",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					GetADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADIGet()
				}
			case "list":
				ListADIs()
			case "create":
				if len(args) > 3 {
					NewADI(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintADICreate()
				}
			default:
				fmt.Println("Usage:")
				PrintADI()
			}
		} else {
			fmt.Println("Usage:")
			PrintADI()
		}

	},
}

func init() {
	rootCmd.AddCommand(adiCmd)
}

func PrintADIGet() {
	fmt.Println("  accumulate adi get [URL]			Get existing ADI by URL")
}

func PrintADICreate() {
	fmt.Println("  accumulate adi create [actor-lite-account] [adi url to create] [public-key or wallet key label] [key-book-name (optional)] [key-page-name (optional)]  Create new ADI from lite account")
	fmt.Println("  accumulate adi create [actor-adi-url] [wallet signing key label] [key index (optional)] [key height (optional)] [adi url to create] [public key or wallet key label] [key book url (optional)] [key page url (optional)] Create new ADI for another ADI")
}

func PrintADIImport() {
	fmt.Println("  accumulate adi import [adi-url] [private-key]	Import Existing ADI")
}

func PrintADI() {
	PrintADIGet()
	PrintADICreate()
	PrintADIImport()
}

func GetADI(url string) {

	var res interface{}
	var str []byte

	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "adi", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func NewADIFromADISigner(actor *url2.URL, args []string) {
	var si *transactions.SignatureInfo
	var privKey []byte
	var err error

	args, si, privKey, err = prepareSigner(actor, args)
	if err != nil {
		log.Fatal(err)
	}

	var adiUrl string
	var book string
	var page string

	if len(args) == 0 {
		log.Fatal("insufficient number of command line arguments")
	}

	if len(args) > 1 {
		adiUrl = args[0]
	}
	if len(args) < 2 {
		log.Fatalf("invalid number of arguments")
	}

	pubKey, err := getPublicKey(args[1])
	if err != nil {
		log.Fatal("invalid public key")
	}

	if len(args) > 3 {
		book = args[3]
	}

	if len(args) > 4 {
		page = args[4]
	}

	u, err := url2.Parse(adiUrl)
	if err != nil {
		log.Fatalf("invalid adi url %s, %v", adiUrl, err)
	}

	idc := &protocol.IdentityCreate{}
	idc.Url = u.Authority
	idc.PublicKey = pubKey
	idc.KeyBookName = book
	idc.KeyPageName = page

	data, err := json.Marshal(idc)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := idc.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "adi-create", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func NewADIFromLiteAccount(actor *url2.URL, adiUrl string, pubKeyOrLabel string, book string, page string) {

	pubKey, err := getPublicKey(pubKeyOrLabel)
	if err != nil {
		log.Fatal("invalid public key")
	}

	privKey, err := LookupByAnon(actor.String())
	if err != nil {
		log.Fatal("cannot resolve private key in wallet")
	}

	u, err := url2.Parse(adiUrl)
	if err != nil {
		log.Fatal(err)
	}

	idc := &protocol.IdentityCreate{}
	idc.Url = u.Authority
	idc.PublicKey = pubKey[:]
	idc.KeyBookName = book
	idc.KeyPageName = page

	data, err := json.Marshal(idc)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := idc.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	si := transactions.SignatureInfo{}
	si.URL = actor.String()
	si.PriorityIdx = 0
	si.MSHeight = 1
	nonce := uint64(time.Now().Unix())

	//the key book is under the anon account.
	params, err := prepareGenTx(data, dataBinary, actor, &si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}
	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "adi-create", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))
}

// NewADI create a new ADI from a sponsored account.
func NewADI(actor string, params []string) {

	u, err := url2.Parse(actor)
	if err != nil {
		log.Fatal(err)
	}

	if IsLiteAccount(u.String()) == true {
		var book string
		var page string
		if len(params) > 2 {
			book = params[2]
		}
		if len(params) > 3 {
			page = params[3]
		}
		NewADIFromLiteAccount(u, params[0], params[1], book, page)
	} else {
	NewADIFromADISigner(u, params[:])
	}
}

func ListADIs() {

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("adi"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, _ = c.Next() {
			fmt.Printf("%s : %s \n", k, string(v))
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

}
