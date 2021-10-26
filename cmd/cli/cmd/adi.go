package cmd

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/boltdb/bolt"
	"log"

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
			case "public":
				if len(args) > 1 {
					PublicADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADIPublic()
				}
			case "create":
				if len(args) == 3 {
					NewADI(args[1], args[2], "", "", "")
				} else if len(args) == 4 {
					NewADI(args[1], args[2], args[3], "", "")
				} else if len(args) == 6 {
					NewADI(args[1], args[2], args[3], args[4], args[5])
				} else {
					fmt.Println("Usage:")
					PrintADICreate()
				}
			case "import":
				if len(args) > 2 {
					ImportADI(args[1], args[2])
				} else {
					fmt.Println("Usage:")
					PrintADIImport()
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

func PrintADIPublic() {
	fmt.Println("  accumulate adi public [URL]			Print public keys hashes for chosen ADI")
}

func PrintADICreate() {
	fmt.Println("  accumulate adi create [signer-url] [adi-url] [public-key (optional)] [key-book-name (optional)] [key-page-name (optional)] Create new ADI")
}

func PrintADIImport() {
	fmt.Println("  accumulate adi import [adi-url] [private-key]	Import Existing ADI")
}

func PrintADI() {
	PrintADIGet()
	PrintADIPublic()
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

func PublicADI(url string) {

	fmt.Println("ADI functionality is not available on Testnet")

}

// NewADI create a new ADI from a sponsored account.
func NewADI(sender string, adiUrl string, pubKeyHex string, book string, page string) {
	var pubKey []byte

	u, err := url2.Parse(adiUrl)
	if err != nil {
		log.Fatal(err)
	}

	i, err := hex.Decode(pubKey, []byte(pubKeyHex))

	if i != 64 && i != 0 {
		log.Fatalf("invalid public key")
	}

	var privKey ed25519.PrivateKey
	if i == 0 {
		pubKey, privKey, err = ed25519.GenerateKey(nil)
		fmt.Printf("Created Initial Key %x\n", pubKey)
	}

	if book == "" {
		book = "ssg0"
		u.JoinPath(book)
	}
	if page == "" {
		page = "sigspec0"
		u.JoinPath(page)
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

	params, err := prepareGenTx(data, dataBinary, sender)
	if err != nil {
		log.Fatal(err)
	}

	//Store the new adi in case things go bad
	if len(privKey) != 0 {
		//as := AdiStore{}

		//var asData []byte
		//err = Db.View(func(tx *bolt.Tx) error {
		//	b := tx.Bucket([]byte("adi"))
		//	asData = b.Get([]byte(u.Authority))
		//	return err
		//})
		//
		//if asData != nil {
		//	err = json.Unmarshal(asData, &as)
		//	log.Fatal(err)
		//}
		//as.KeyBooks = make(map[string]KeyBookStore)
		//if b, ok := as.KeyBooks[book]; !ok {
		//	if b.KeyPages == nil {
		//		b.KeyPages = make(map[string]KeyPageStore)
		//	}
		//	as.KeyBooks[page] = b
		//	if p, ok := b.KeyPages[page]; !ok {
		//		p.PrivKeys = append(p.PrivKeys, types.Bytes(privKey))
		//		b.KeyPages[page] = p
		//	}
		//}

		err = Db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("anon"))
			err := b.Put([]byte(adiUrl), privKey)
			return err
		})

		if err != nil {
			log.Fatal(err)
		}
	}
	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "adi-create", params, &res); err != nil {
		//todo: if we fail, then we need to remove the adi from storage or keep it and try again later...
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func ImportADI(url string, pk string) {

	fmt.Println("ADI functionality is not available on Testnet")

}
