package cmd

import (
	"fmt"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
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
	fmt.Println("  accumulate page create [origin adi url] [signing key name] [key index (optional)] [key height (optional)] [new key page url] [public key 1] ... [public key hex or name n + 1] Create new key page with 1 to N+1 public keys")
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
	res, _, err := GetKeyPage(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key page for %s, %v", url, err)
	}

	return PrintQueryResponseV2(res)
}

func GetKeyPage(url string) (*api2.QueryResponse, *protocol.KeyPage, error) {
	res, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	kp := protocol.KeyPage{}
	err = UnmarshalQuery(res.Data, &kp)
	if err != nil {
		return nil, nil, err
	}
	return res, &kp, nil
}

// CreateKeyPage create a new key page
func CreateKeyPage(page string, args []string) (string, error) {
	pageUrl, err := url2.Parse(page)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(pageUrl, args)
	if err != nil {
		return "", err
	}

	if len(args) < 2 {
		return "", fmt.Errorf("invalid number of arguments")
	}
	newUrl, err := url2.Parse(args[0])
	keyLabels := args[1:]
	//when creating a key page you need to have the keys already generated and labeled.
	if newUrl.Authority != pageUrl.Authority {
		return "", fmt.Errorf("page url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, pageUrl.Authority)
	}

	ckp := protocol.CreateKeyPage{}
	ksp := make([]*protocol.KeySpecParams, len(keyLabels))
	ckp.Url = newUrl.String()
	ckp.Keys = ksp
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

		ckp.Keys[i] = &ksp
	}

	res, err := dispatchTxRequest("create-key-page", &ckp, pageUrl, si, privKey)
	if err != nil {
		return "", err
	}

	return ActionResponseFrom(res).Print()

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

func KeyPageUpdate(origin string, op protocol.KeyPageOperation, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	var newKey []byte
	var oldKey []byte

	ukp := protocol.UpdateKeyPage{}
	ukp.Operation = op

	switch op {
	case protocol.UpdateKey:
		if len(args) < 2 {
			return "", fmt.Errorf("invalid number of arguments")
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			return "", err
		}
		newKey, err = resolveKey(args[1])
		if err != nil {
			return "", err
		}
	case protocol.AddKey:
		if len(args) < 1 {
			return "", fmt.Errorf("invalid number of arguments")
		}
		newKey, err = resolveKey(args[0])
		if err != nil {
			return "", err
		}
	case protocol.RemoveKey:
		if len(args) < 1 {
			return "", fmt.Errorf("invalid number of arguments")
		}
		oldKey, err = resolveKey(args[0])
		if err != nil {
			return "", err
		}
	}

	ukp.Key = oldKey[:]
	ukp.NewKey = newKey[:]

	res, err := dispatchTxRequest("update-key-page", &ukp, u, si, privKey)
	if err != nil {
		return "", err
	}

	return ActionResponseFrom(res).Print()
}
