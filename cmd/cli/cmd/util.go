package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/boltdb/bolt"
	"time"
)

func prepareGenTx(jsonPayload []byte, binaryPayload []byte, sender string) (*acmeapi.APIRequestRaw, error) {

	params := &acmeapi.APIRequestRaw{}
	params.Tx = &acmeapi.APIRequestRawTx{}

	err := Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		pk := b.Get([]byte(sender))
		fmt.Println(hex.EncodeToString(pk))

		params.Tx.Data = &json.RawMessage{}
		*params.Tx.Data = jsonPayload
		params.Tx.Signer = &acmeapi.Signer{}
		params.Tx.Signer.Nonce = uint64(time.Now().Unix())
		params.Tx.Sponsor = types.String(sender)
		params.Tx.KeyPage = &acmeapi.APIRequestKeyPage{}
		params.Tx.KeyPage.Height = 1
		params.Tx.KeyPage.Index = 0

		params.Tx.Sig = types.Bytes64{}

		gtx := new(transactions.GenTransaction)
		gtx.Transaction = binaryPayload
		u, err := url2.Parse(sender)
		if err != nil {
			return err
		}
		gtx.ChainID = u.ResourceChain()
		gtx.Routing = u.Routing()

		gtx.SigInfo = new(transactions.SignatureInfo)
		//the siginfo URL is the URL of the signer
		gtx.SigInfo.URL = sender
		//Provide a nonce, typically this will be queried from identity sig spec and incremented.
		//since SigGroups are not yet implemented, we will use the unix timestamp for now.
		gtx.SigInfo.Unused2 = params.Tx.Signer.Nonce
		//The following will be defined in the SigSpec Group for which key to use
		gtx.SigInfo.MSHeight = params.Tx.KeyPage.Height
		gtx.SigInfo.PriorityIdx = params.Tx.KeyPage.Index

		ed := new(transactions.ED25519Sig)
		err = ed.Sign(gtx.SigInfo.Unused2, pk, gtx.TransactionHash())
		if err != nil {
			return err
		}
		params.Tx.Sig.FromBytes(ed.GetSignature())
		//The public key needs to be used to verify the signature, however,
		//to pass verification, the validator will hash the key and check the
		//sig spec group to make sure this key belongs to the identity.
		params.Tx.Signer.PublicKey.FromBytes(ed.GetPublicKey())
		return nil
	})

	return params, err
}

type KeyPageStore struct {
	PrivKeys []types.Bytes `json:"privKeys"`
}

type KeyBookStore struct {
	KeyPages map[string]KeyPageStore `json:"privKey"`
}

type AdiStore struct {
	KeyBooks         map[string]KeyBookStore `json:"keyBooks"`
	tokenAccountUrls []string                `json:"tokenAccountUrls"`
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
