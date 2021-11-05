package cmd

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"log"
	"strconv"
	"time"
)

func prepareSigner(actor *url2.URL, args []string) ([]string, *transactions.SignatureInfo, []byte, error) {
	//adiActor labelOrPubKeyHex height index
	var privKey []byte
	var err error

	ct := 0
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	ed := transactions.SignatureInfo{}
	ed.URL = actor.String()
	ed.MSHeight = 1
	ed.PriorityIdx = 0

	if IsLiteAccount(actor.String()) == true {
		privKey, err = LookupByLabel(actor.String()) //LookupByAnon(actor.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find private key for lite account %s %v", actor.String(), err)
		}
		return args, &ed, privKey, nil
	}

	if len(args) > 1 {
		b, err := pubKeyFromString(args[0])
		if err != nil {
			privKey, err = LookupByLabel(args[0])
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid public key or wallet label specified on command line")
			}

		} else {
			privKey, err = LookupByPubKey(b)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid public key, cannot resolve signing key")
			}
		}
		ct++
	} else {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	if len(args) > 2 {
		if v, err := strconv.ParseInt(args[1], 10, 64); err == nil {
			ct++
			ed.PriorityIdx = uint64(v)
			if len(args) > 3 {
				if v, err := strconv.ParseInt(args[2], 10, 64); err == nil {
					ct++
					ed.MSHeight = uint64(v)
				}
			}
		}
	}

	return args[ct:], &ed, privKey, nil
}

func prepareGenTx(jsonPayload []byte, binaryPayload []byte, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*acmeapi.APIRequestRaw, error) {

	params := &acmeapi.APIRequestRaw{}
	params.Tx = &acmeapi.APIRequestRawTx{}

	params.Tx.Data = &json.RawMessage{}
	*params.Tx.Data = jsonPayload
	params.Tx.Signer = &acmeapi.Signer{}
	params.Tx.Signer.PublicKey.FromBytes(privKey[32:])
	params.Tx.Signer.Nonce = nonce
	params.Tx.Sponsor = types.String(actor.String())
	params.Tx.KeyPage = &acmeapi.APIRequestKeyPage{}
	params.Tx.KeyPage.Height = si.MSHeight
	params.Tx.KeyPage.Index = si.PriorityIdx

	params.Tx.Sig = types.Bytes64{}

	gtx := new(transactions.GenTransaction)
	gtx.Transaction = binaryPayload

	gtx.ChainID = actor.ResourceChain()
	gtx.Routing = actor.Routing()

	si.Nonce = nonce
	gtx.SigInfo = si

	ed := new(transactions.ED25519Sig)
	err := ed.Sign(nonce, privKey, gtx.TransactionHash())
	if err != nil {
		return nil, err
	}
	params.Tx.Sig.FromBytes(ed.GetSignature())
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Tx.Signer.PublicKey.FromBytes(ed.GetPublicKey())

	gtx.Signature = append(gtx.Signature, ed)

	return params, err
}

func IsLiteAccount(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	u2, err := url2.Parse(u.Authority)
	if err != nil {
		log.Fatal(err)
	}
	return protocol.IsValidAdiUrl(u2) != nil
}

func GetUrl(url string, method string) ([]byte, error) {

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

	return str, nil
}

type KeyPageStore struct {
	PrivKeys []types.Bytes `json:"privKeys"`
}

type KeyBookStore struct {
	KeyPageList []string `json:"keyPages"`
}

type AccountKeyBookStore struct {
	KeyBook KeyBookStore `json:"keyBook"`
}

//
//func (a *AdiStore) MarshalBinary() ([]byte, error) {
//	var buf bytes.Buffer
//
//	buf.Write(common.Uint64Bytes(uint64(len(a.tokenAccounts))))
//	for i := range a.tokenAccounts {
//		buf.Write(common.SliceBytes([]byte(a.tokenAccounts[i])))
//	}
//
//	buf.Write(common.Uint64Bytes(uint64(len(a.keyBooks))))
//	for i := range a.keyBooks {
//		buf.Write(common.SliceBytes([]byte(a.keyBooks[i])))
//	}
//
//	return buf.Bytes(), nil
//}
//
//func (a *AdiStore) UnmarshalBinary(data []byte) (err error) {
//	defer func() {
//		if rErr := recover(); rErr != nil {
//			err = fmt.Errorf("insufficent data to unmarshal AdiStore %v", rErr)
//		}
//	}()
//
//	var s []byte
//	l, data := common.BytesUint64(data)
//	for i := uint64(0); i < l; i++ {
//		s, data = common.BytesSlice(data)
//		a.tokenAccounts[i] = string(s)
//	}
//
//	l, data = common.BytesUint64(data)
//	for i := uint64(0); i < l; i++ {
//		s, data = common.BytesSlice(data)
//		a.keyBooks[i] = string(s)
//	}
//
//	return nil
//}

func dispatchRequest(action string, payload interface{}, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte) (interface{}, error) {
	json.Marshal(payload)

	data, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := payload.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
	}

	var res interface{}
	if err := Client.Request(context.Background(), "create-sig-spec-group", params, &res); err != nil {
		return nil, err
	}

	return res, nil
}
