package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/spf13/cobra"
	"log"
	"time"

	"github.com/AccumulateNetwork/accumulated/protocol"
)

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "Create and manage Keys, Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 2 {
			if args[0] == "get" {
				GetAndPrintKeyPage(args[1])
			} else {
				fmt.Println("Usage:")
				PrintKeyPageGet()
				PrintKeyPageCreate()
				PrintKeyUpdate()
			}
		} else if len(args) > 3 {
			if args[0] == "create" {
				CreateKeyPage(args[1], args[2:])
			} else if args[0] == "key" {
				switch arg := args[1]; arg {
				case "update":
					KeyPageUpdate(args[2], protocol.UpdateKey, args[3:])
				case "add":
					KeyPageUpdate(args[2], protocol.AddKey, args[3:])
				case "remove":
					KeyPageUpdate(args[2], protocol.RemoveKey, args[3:])
				default:
					fmt.Println("Usage:")
					PrintKeyPageCreate()
					PrintKeyUpdate()
				}
			} else {
				PrintPage()
			}
		} else {
			PrintPage()
		}
	},
}

func init() {
	rootCmd.AddCommand(pageCmd)
}

func PrintKeyPageGet() {
	fmt.Println("  accumulate page get [URL]			Get existing Key Page by URL")
}

func PrintKeyPageCreate() {
	fmt.Println("  accumulate page create [actor adi url] [signing key name] [key index (optional)] [key height (optional)] [new key page url] [public key 1] ... [public key hex or name n + 1] Create new key page with 1 to N+1 public keys")
	fmt.Println("\t\t example usage: accumulate key page create acc://RedWagon redKey5 acc://RedWagon/RedPage1 redKey1 redKey2 redKey3")
}
func PrintKeyUpdate() {
	fmt.Println("  accumulate page key update [key page url] [signing key name] [key index (optional)] [key height (optional)] [old key name] [new public key or name] Update key in a key page with a new public key")
	fmt.Println("\t\t example usage: accumulate key update page  acc://RedWagon redKey5 acc://RedWagon/RedPage1 redKey1 redKey2 redKey3")
	fmt.Println("  accumulate page key add [key page url] [signing key name] [key index (optional)] [key height (optional)] [new key name] Add key to a key page")
	fmt.Println("\t\t example usage: accumulate key add page acc://RedWagon redKey5 acc://RedWagon/RedPage1 redKey1 redKey2 redKey3")
	fmt.Println("  accumulate page key remove [key page url] [signing key name] [key index (optional)] [key height (optional)] [old key name] Remove key from a key page")
	fmt.Println("\t\t example usage: accumulate key add page acc://RedWagon redKey5 acc://RedWagon/RedPage1 redKey1 redKey2 redKey3")
}

func PrintPage() {
	PrintKeyPageCreate()
	PrintKeyPageGet()
	PrintKeyUpdate()
}

func GetAndPrintKeyPage(url string) {
	str, _, err := GetKeyPage(url)
	if err != nil {
		log.Fatal(fmt.Errorf("error retrieving key book for %s", url))
	}

	fmt.Println(string(str))
}

func GetKeyPage(url string) ([]byte, *protocol.SigSpecGroup, error) {
	s, err := GetUrl(url, "sig-spec")
	if err != nil {
		log.Fatal(err)
	}

	ssg := protocol.SigSpecGroup{}
	err = json.Unmarshal([]byte(s), &ssg)
	if err != nil {
		log.Fatal(err)
	}

	return s, &ssg, nil
}

// CreateKeyPage create a new key page
func CreateKeyPage(page string, args []string) {

	pageUrl, err := url2.Parse(page)
	if err != nil {
		PrintKeyPageCreate()
		log.Fatal(err)
	}

	args, si, privKey, err := prepareSigner(pageUrl, args)
	if err != nil {
		PrintKeyBookCreate()
		log.Fatal(err)
	}
	if len(args) < 2 {
		PrintKeyPageCreate()
		log.Fatal(fmt.Errorf("invalid number of arguments"))
	}
	newUrl, err := url2.Parse(args[0])
	keyLabels := args[1:]
	//when creating a key page you need to have the keys already generated and labeled.
	if newUrl.Authority != pageUrl.Authority {
		PrintKeyPageCreate()
		log.Fatalf("page url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, pageUrl.Authority)
	}

	css := protocol.CreateSigSpec{}
	ksp := make([]*protocol.KeySpecParams, len(keyLabels))
	css.Url = newUrl.String()
	css.Keys = ksp
	for i := range keyLabels {
		ksp := protocol.KeySpecParams{}

		pk, err := LookupByLabel(keyLabels[i])
		if err != nil {
			//now check to see if it is a valid key hex, if so we can assume that is the public key.
			ksp.PublicKey, err = pubKeyFromString(keyLabels[i])
			if err != nil {
				PrintKeyPageCreate()
				log.Fatal(fmt.Errorf("key name %s, does not exist in wallet, nor is it a valid public key", keyLabels[i]))
			}
		} else {
			ksp.PublicKey = pk[32:]
		}

		css.Keys[i] = &ksp
	}

	data, err := json.Marshal(css)
	if err != nil {
		PrintKeyPageCreate()
		log.Fatal(err)
	}

	dataBinary, err := css.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, pageUrl, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "create-sig-spec", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func resolveKey(key string) ([]byte, error) {
	ret, err := getPublicKey(key)
	if err != nil {
		ret, err = pubKeyFromString(key)
		if err != nil {
			PrintKeyUpdate()
			return nil, fmt.Errorf("key %s, does not exist in wallet, nor is it a valid public key", key)
		}
	}
	return ret, err
}

func KeyPageUpdate(actorUrl string, op protocol.KeyPageOperation, args []string) {

	u, err := url2.Parse(actorUrl)
	if err != nil {
		log.Fatal(err)
	}

	args, si, privKey, err := prepareSigner(u, args)
	if err != nil {
		PrintKeyUpdate()
		log.Fatal(err)
	}

	var newKey []byte
	var oldKey []byte

	ukp := protocol.UpdateKeyPage{}
	ukp.Operation = op

	switch op {
	case protocol.UpdateKey:
		if len(args) < 2 {
			PrintKeyUpdate()
			log.Fatal(fmt.Errorf("invalid number of arguments"))
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			log.Fatal(err)
		}
		newKey, err = resolveKey(args[1])
		if err != nil {
			PrintKeyUpdate()
			log.Fatal(err)
		}
	case protocol.AddKey:
		if len(args) < 1 {
			PrintKeyUpdate()
			log.Fatal(fmt.Errorf("invalid number of arguments"))
		}
		newKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			log.Fatal(err)
		}
	case protocol.RemoveKey:
		if len(args) < 1 {
			PrintKeyUpdate()
			log.Fatal(fmt.Errorf("invalid number of arguments"))
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			log.Fatal(err)
		}
	}

	ukp.Key = oldKey[:]
	ukp.NewKey = newKey[:]
	data, err := json.Marshal(&ukp)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := ukp.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, u, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	var str []byte
	if err := Client.Request(context.Background(), "update-key-page", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))
}
