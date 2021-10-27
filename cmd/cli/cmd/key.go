package cmd

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Create and manage Keys, Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 2 {
					if args[1] == "page" {
						GetKey(args[2], "sig-spec")
					} else if args[1] == "book" {
						GetKey(args[2], "sig-spec-group")
					} else {
						fmt.Println("Usage:")
						PrintKeyGet()
					}
				} else {
					fmt.Println("Usage:")
					PrintKeyGet()
				}
			case "create":
				if len(args) > 3 {
					if args[1] == "page" {
						CreateKeyPage(args[2], args[3:])
					} else if args[1] == "book" {
						CreateKeyBook(args[2], args[3:])
					} else {
						fmt.Println("Usage:")
						PrintKeyCreate()
					}
				} else {
					fmt.Println("Usage:")
					PrintKeyCreate()
				}
			case "update":
				if len(args) > 3 {
					UpdateKeyPage(args[1], args[2], args[3], args[4])
				} else {
					fmt.Println("Usage:")
					PrintKeyCreate()
				}
			case "list":
				ListKeyPublic()
			case "generate":
				if len(args) > 1 {
					GenerateKey(args[1])
				}
				PrintKeyGenerate()
			default:
				fmt.Println("Usage:")
				PrintKey()
			}
		} else {
			fmt.Println("Usage:")
			PrintKey()
		}

	},
}

func init() {
	rootCmd.AddCommand(keyCmd)
}

func PrintKeyPublic() {
	fmt.Println("  accumulate key list			List generated keys associated with the wallet")
}
func PrintKeyGet() {
	fmt.Println("  accumulate key get book [URL]			Get existing Key Book by URL")
	fmt.Println("  accumulate key get page [URL]			Get existing Key Page by URL")
}

func PrintKeyCreate() {
	fmt.Println("  accumulate key create [page] [URL] [public-key-hex-1] ... [public-key-hex-n] Create new key page with 1 to N public keys")
	fmt.Println("  accumulate key create [book] [URL] [public-key-hex-1] ... [public-key-hex-n] Create new key page with 1 to N public keys")
}

func PrintKeyGenerate() {
	fmt.Println("  accumulate key generate label Generate a new key and give it a label in the wallet")
}

func PrintKey() {
	PrintKeyGet()
	PrintKeyCreate()
	PrintKeyGenerate()
	PrintKeyPublic()
}

func GetKey(url string, method string) {

	var res interface{}
	var str []byte

	u, err := url2.Parse(url)
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(u.String())

	if err := Client.Request(context.Background(), method, params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

// CreateKeyPage create a new key page
func CreateKeyPage(pageUrl string, pubKeyHex []string) {
	//todo, might be nice to have the pubkeyhex be either a public key or a label of internal key
	var pubKeys []types.Bytes32
	pubKeys = make([]types.Bytes32, len(pubKeyHex))

	u, err := url2.Parse(pageUrl)
	if err != nil {
		log.Fatal(err)
	}
	css := protocol.CreateSigSpec{}
	ksp := make([]*protocol.KeySpecParams, len(pubKeyHex))
	css.Url = u.String()
	css.Keys = ksp
	for i := range pubKeyHex {
		ksp := protocol.KeySpecParams{}

		j, err := hex.Decode(pubKeys[i][:], []byte(pubKeyHex[i]))

		if j != 64 || err != nil {
			log.Fatalf("invalid public key %s", pubKeyHex[i])
		}

		ksp.PublicKey = pubKeys[i][:]
		css.Keys[i] = &ksp
	}

	data, err := json.Marshal(css)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := css.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	params, err := prepareGenTx(data, dataBinary, css.Url, u.Authority, "adi")
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "key-page-create", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func UpdateKeyPage(pageUrl string, op string, keyHex string, newKeyHex string) {
	key := types.Bytes32{}
	newKey := types.Bytes32{}
	operation := protocol.KeyPageOperationByName(op)

	u, err := url2.Parse(pageUrl)
	if err != nil {
		log.Fatal(err)
	}

	i, err := hex.Decode(key[:], []byte(keyHex))

	if err != nil {
		log.Fatal(err)
	}

	if i != 64 {
		log.Fatal("invalid old key")
	}

	i, err = hex.Decode(newKey[:], []byte(newKeyHex))
	if i != 64 {
		log.Fatal("invalid new key")
	}

	ukp := protocol.UpdateKeyPage{}

	ukp.Key = key[:]
	ukp.NewKey = newKey[:]
	ukp.Operation = operation

	data, err := json.Marshal(ukp)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := ukp.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	params, err := prepareGenTx(data, dataBinary, u.String(), u.Authority, "adi")
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "key-page-update", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

// CreateKeyBook create a new key page
func CreateKeyBook(bookUrl string, pageUrls []string) {
	u, err := url2.Parse(bookUrl)
	if err != nil {
		log.Fatal(err)
	}

	ssg := protocol.CreateSigSpecGroup{}
	ssg.Url = u.String()

	var chainId types.Bytes32
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			log.Fatalf("invalid page url %s, %v", pageUrls[i], err)
		}
		chainId.FromBytes(u2.ResourceChain())
		ssg.SigSpecs = append(ssg.SigSpecs, chainId)
	}

	data, err := json.Marshal(ssg)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := ssg.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	params, err := prepareGenTx(data, dataBinary, ssg.Url, u.Authority, "adi")
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "key-book-create", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func GenerateKey(label string) {
	fmt.Println("Generation of key functionality is not currently available")
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal(err)
	}

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		err := b.Put([]byte(label), privKey)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Public Key %s : %s", label, pubKey)
}

func ListKeyPublic() {

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, _ = c.Next() {
			fmt.Printf("%s %x\n", k, v[32:])
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

func ImportKey(label string, pkhex string) {
	var pk ed25519.PrivateKey

	token, err := hex.DecodeString(pkhex)
	if err != nil {
		log.Fatal(err)
	}

	if len(token) == 32 {
		pk = ed25519.NewKeyFromSeed(token)
	}
	pk = token

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		err := b.Put([]byte(label), pk)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("{\"label\":\"%s\",\"publicKey\":\"%x\"}\n", label, token[32:])
}

func ExportKey(label string) {
	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		pk := b.Get([]byte(label))
		fmt.Println(hex.EncodeToString(pk))
		fmt.Printf("{\"label\":\"%s\",\"privateKey\":\"%x\",\"publicKey\":\"%x\"}\n", label, pk[:32], pk[32:])
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
