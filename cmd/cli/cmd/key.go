package cmd

import (
	"bytes"
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
	//"github.com/tyler-smith/go-bip32"
	//"github.com/tyler-smith/go-bip39"
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
			case "import":
				ImportMneumonic(args[1:])
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
				} else {
					PrintKeyGenerate()
				}
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
	fmt.Println("  accumulate key create [page] [URL] [key label 1] ... [key label n] Create new key page with 1 to N public keys within the wallet")
	fmt.Println("  accumulate key create [book] [URL] [key page url 1] ... [key page url n] Create new key page with 1 to N public keys")
}

func PrintKeyGenerate() {
	fmt.Println("  accumulate key generate [label]     Generate a new key and give it a label in the wallet")
}

func PrintKey() {
	PrintKeyGet()
	PrintKeyCreate()
	PrintKeyGenerate()
	PrintKeyPublic()
}

func GetKeyPage(book string, keyLabel string) (*protocol.SigSpec, int, error) {

	b, err := url2.Parse(book)
	if err != nil {
		log.Fatal(err)
	}

	privKey, err := LookupByLabel(keyLabel)
	if err != nil {
		log.Fatal(err)
	}

	kb, err := GetKeyBook(b.String())
	if err != nil {
		log.Fatal(err)
	}

	for i := range kb.SigSpecs {
		v := kb.SigSpecs[i]
		//we have a match so go fetch the ssg
		s, err := GetByChainId(v[:])
		if err != nil {
			log.Fatal(err)
		}
		if *s.Type.AsString() != types.ChainTypeSigSpec.Name() {
			log.Fatal(fmt.Errorf("expecting key page, received %s", s.Type))
		}
		ss := protocol.SigSpec{}
		err = ss.UnmarshalBinary(*s.Data)
		if err != nil {
			log.Fatal(err)
		}

		for j := range ss.Keys {
			_, err := LookupByPubKey(ss.Keys[j].PublicKey)
			if err == nil && bytes.Equal(privKey[32:], v[:]) {
				return &ss, j, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("key page not found in book %s for key label %s", book, keyLabel)
}

func GetKeyBook(url string) (*protocol.SigSpecGroup, error) {
	s, err := GetKey(url, "sig-spec-group")
	if err != nil {
		log.Fatal(err)
	}

	ssg := protocol.SigSpecGroup{}
	err = json.Unmarshal([]byte(s), &ssg)
	if err != nil {
		log.Fatal(err)
	}

	return &ssg, nil
}

func GetKey(url string, method string) ([]byte, error) {

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

	return str, nil
}

// CreateKeyPage create a new key page
func CreateKeyPage(pageUrl string, keyLabels []string) {
	//when creating a key page you need to have the keys already generated and labeled.
	u, err := url2.Parse(pageUrl)
	if err != nil {
		log.Fatal(err)
	}

	css := protocol.CreateSigSpec{}
	ksp := make([]*protocol.KeySpecParams, len(keyLabels))
	css.Url = u.String()
	css.Keys = ksp
	for i := range keyLabels {
		ksp := protocol.KeySpecParams{}

		pk, err := LookupByLabel(keyLabels[i])
		if err != nil {
			log.Fatal(fmt.Errorf("key label %s, does not exist in wallet", keyLabels[i]))
		}

		ksp.PublicKey = pk[32:]
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

func LookupByLabel(label string) (asData []byte, err error) {
	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		asData = b.Get([]byte(label))
		if len(asData) == 0 {
			err = fmt.Errorf("valid key not found for %s", label)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return LookupByPubKey(asData)
}

func LookupByPubKey(pubKey []byte) (asData []byte, err error) {
	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		asData = b.Get(pubKey)
		return err
	})
	return
}

func GenerateKey(label string) {
	_, err := LookupByLabel(label)
	if err == nil {
		log.Fatal(fmt.Errorf("key already exists for label %s", label))
	}
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal(err)
	}

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("keys"))
		err := b.Put(pubKey, privKey)
		return err
	})

	err = Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		err := b.Put([]byte(label), pubKey)
		return err
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Public Key %s : %x", label, pubKey)
}

func ListKeyPublic() {

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("label"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, _ = c.Next() {
			pk, err := LookupByPubKey(v)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s %x\n", k, pk[32:])
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

func ImportMneumonic(mnemonic []string) {
	//entropy, _ := bip39.NewEntropy(256)
	//mnemonic, _ := bip39.NewMnemonic(entropy)
	//
	//// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	//seed := bip39.NewSeed(mnemonic, "Secret Passphrase")
	//
	//masterKey, _ := bip32.NewMasterKey(seed)
	//publicKey := masterKey.PublicKey()
	//
	//// Display mnemonic and keys
	//fmt.Println("Mnemonic: ", mnemonic)
	//fmt.Println("Master private key: ", masterKey)
	//fmt.Println("Master public key: ", publicKey)
}
