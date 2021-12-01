package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/spf13/cobra"
)

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "Create and manage Keys, Books, and Pages",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) == 2 {
			if args[0] == "get" {
				out, err = GetAndPrintKeyPage(args[1])
			} else {
				fmt.Println("Usage:")
				PrintKeyPageGet()
				PrintKeyPageCreate()
				PrintKeyUpdate()
			}
		} else if len(args) > 3 {
			if args[0] == "create" {
				out, err = CreateKeyPage(args[1], args[2:])
			} else if args[0] == "key" {
				switch arg := args[1]; arg {
				case "update":
					out, err = KeyPageUpdate(args[2], protocol.UpdateKey, args[3:])
				case "add":
					out, err = KeyPageUpdate(args[2], protocol.AddKey, args[3:])
				case "remove":
					out, err = KeyPageUpdate(args[2], protocol.RemoveKey, args[3:])
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
		printOutput(cmd, out, err)
	},
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
	fmt.Println("\t\t example usage: accumulate page key update  acc://RedWagon/RedPage1 redKey1 redKey2 redKey3")
	fmt.Println("  accumulate page key add [key page url] [signing key name] [key index (optional)] [key height (optional)] [new key name] Add key to a key page")
	fmt.Println("\t\t example usage: accumulate page key add acc://RedWagon/RedPage1 redKey1 redKey2 ")
	fmt.Println("  accumulate page key remove [key page url] [signing key name] [key index (optional)] [key height (optional)] [old key name] Remove key from a key page")
	fmt.Println("\t\t example usage: accumulate page key remove acc://RedWagon/RedPage1 redKey1 redKey2")
}

func PrintPage() {
	PrintKeyPageCreate()
	PrintKeyPageGet()
	PrintKeyUpdate()
}

func GetAndPrintKeyPage(url string) (string, error) {
	str, _, err := GetKeyPage(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key page for %s, %v", url, err)
	}

	res := acmeapi.APIDataResponse{}
	err = json.Unmarshal(str, &res)
	if err != nil {
		return "", err
	}
	return PrintQueryResponse(&res)
}

func GetKeyPage(url string) ([]byte, *protocol.KeyPage, error) {
	s, err := GetUrl(url, "sig-spec")
	if err != nil {
		return nil, nil, err
	}

	res := acmeapi.APIDataResponse{}
	err = json.Unmarshal(s, &res)
	if err != nil {
		return nil, nil, err
	}

	ss := protocol.KeyPage{}
	err = json.Unmarshal(*res.Data, &ss)
	if err != nil {
		return nil, nil, err
	}

	return s, &ss, nil
}

// CreateKeyPage create a new key page
func CreateKeyPage(page string, args []string) (string, error) {
	pageUrl, err := url2.Parse(page)
	if err != nil {
		PrintKeyPageCreate()
		return "", err
	}

	args, si, privKey, err := prepareSigner(pageUrl, args)
	if err != nil {
		PrintKeyBookCreate()
		return "", err
	}
	if len(args) < 2 {
		PrintKeyPageCreate()
		return "", fmt.Errorf("invalid number of arguments")
	}
	newUrl, err := url2.Parse(args[0])
	keyLabels := args[1:]
	//when creating a key page you need to have the keys already generated and labeled.
	if newUrl.Authority != pageUrl.Authority {
		return "", fmt.Errorf("page url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, pageUrl.Authority)
	}

	css := protocol.CreateKeyPage{}
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
				return "", fmt.Errorf("key name %s, does not exist in wallet, nor is it a valid public key", keyLabels[i])
			}
		} else {
			ksp.PublicKey = pk[32:]
		}

		css.Keys[i] = &ksp
	}

	data, err := json.Marshal(css)
	if err != nil {
		PrintKeyPageCreate()
		return "", err
	}

	dataBinary, err := css.MarshalBinary()
	if err != nil {
		PrintKeyPageCreate()
		return "", err
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, pageUrl, si, privKey, nonce)
	if err != nil {
		PrintKeyPageCreate()
		return "", err
	}

	var res acmeapi.APIDataResponse
	if err := Client.Request(context.Background(), "create-sig-spec", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	}
	return ar.Print()
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

func KeyPageUpdate(actorUrl string, op protocol.KeyPageOperation, args []string) (string, error) {

	u, err := url2.Parse(actorUrl)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(u, args)
	if err != nil {
		PrintKeyUpdate()
		return "", err
	}

	var newKey []byte
	var oldKey []byte

	ukp := protocol.UpdateKeyPage{}
	ukp.Operation = op

	switch op {
	case protocol.UpdateKey:
		if len(args) < 2 {
			PrintKeyUpdate()
			return "", fmt.Errorf("invalid number of arguments")
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			return "", err
		}
		newKey, err = resolveKey(args[1])
		if err != nil {
			PrintKeyUpdate()
			return "", err
		}
	case protocol.AddKey:
		if len(args) < 1 {
			PrintKeyUpdate()
			return "", fmt.Errorf("invalid number of arguments")
		}
		newKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			return "", err
		}
	case protocol.RemoveKey:
		if len(args) < 1 {
			PrintKeyUpdate()
			return "", fmt.Errorf("invalid number of arguments")
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			PrintKeyUpdate()
			return "", err
		}
	}

	ukp.Key = oldKey[:]
	ukp.NewKey = newKey[:]
	data, err := json.Marshal(&ukp)
	if err != nil {
		return "", err
	}

	dataBinary, err := ukp.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, u, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res acmeapi.APIDataResponse
	if err := Client.Request(context.Background(), "update-key-page", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	}
	return ar.Print()
}
