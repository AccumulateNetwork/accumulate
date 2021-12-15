package goaccumulate

import (
	"context"
	"encoding/json"
	"fmt"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
)

func GetAndPrintKeyPage(url string) (string, error) {
	str, _, err := GetKeyPage(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key page for %s, %v", url, err)
	}

	res := api2.QueryResponse{}

	err = json.Unmarshal(str, &res)
	if err != nil {
		return "", err
	}
	return PrintQueryResponseV2(&res)
}

func GetKeyPage(url string) ([]byte, *protocol.KeyPage, error) {
	s, err := GetUrl(url, "query-key-index")
	if err != nil {
		return nil, nil, err
	}

	res := api2.QueryResponse{}
	err = json.Unmarshal(s, &res)
	if err != nil {
		return nil, nil, err
	}

	ss := protocol.KeyPage{}
	//err = json.Unmarshal(*res.Data, &ss)
	//if err != nil {
	//		return nil, nil, err
	//	}

	return s, &ss, nil
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
		return "", err
	}

	dataBinary, err := css.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, pageUrl, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.QueryResponse
	if err := Client.RequestV2(context.Background(), "create-key-page", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	//	err = json.Unmarshal(*res.Data, &ar)
	//	if err != nil {
	//		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	//	}
	return ar.Print()
}

func resolveKey(key string) ([]byte, error) {
	ret, err := getPublicKey(key)
	if err != nil {
		ret, err = pubKeyFromString(key)
		if err != nil {
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
	data, err := json.Marshal(&ukp)
	if err != nil {
		return "", err
	}

	dataBinary, err := ukp.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, u, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.QueryResponse
	if err := Client.RequestV2(context.Background(), "update-key-page", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	//	err = json.Unmarshal(*res.Data, &ar)
	//	if err != nil {
	//		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	//	}
	return ar.Print()
}
