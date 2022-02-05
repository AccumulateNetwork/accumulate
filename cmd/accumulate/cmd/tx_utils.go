package cmd

import (
	"context"
	"encoding"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

func getTX(hash []byte, wait time.Duration) (*api.TransactionQueryResponse, error) {
	params := new(api.TxnQuery)
	params.Txid = hash
	params.Prove = TxProve

	if wait > 0 {
		params.Wait = wait
	}

	res, err := Client.QueryTx(context.Background(), params)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func prepareSigner(origin *url2.URL, args []string) ([]string, *transactions.Header, []byte, error) {
	var privKey []byte
	var err error

	ct := 0
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	hdr := transactions.Header{}
	hdr.Origin = origin
	hdr.KeyPageHeight = 1
	hdr.KeyPageIndex = 0

	if IsLiteAccount(origin.String()) {
		privKey, err = LookupByLabel(origin.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find private key for lite account %s %v", origin.String(), err)
		}
		return args, &hdr, privKey, nil
	}

	privKey, err = resolvePrivateKey(args[0])
	if err != nil {
		return nil, nil, nil, err
	}
	ct++

	if len(args) > 1 {
		if v, err := strconv.ParseInt(args[1], 10, 64); err == nil {
			ct++
			hdr.KeyPageIndex = uint64(v)
		}
	}

	keyInfo, err := getKey(origin.String(), privKey[32:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get key for %q : %v", origin, err)
	}

	resp := new(api.ChainQueryResponse)
	_, err = queryUrl(keyInfo.KeyPage, false, false, resp)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get %q : %v", keyInfo.KeyPage, err)
	}

	hdr.KeyPageIndex = keyInfo.Index
	hdr.KeyPageHeight = resp.MainChain.Height

	return args[ct:], &hdr, privKey, nil
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

// To prepare for Execute, leaveUnmarshalled = false.
// To prepare for ExecuteSpecificTxn, leaveUnmarshalled = true.
func prepareToExecute(payload encoding.BinaryMarshaler, leaveUnmarshalled bool, txHash []byte, header *transactions.Header, privKey []byte) (*api2.TxRequest, error) {
	header.Nonce = nonceFromTimeNow()

	dataToSign, err := payload.MarshalBinary()
	if err != nil {
		return nil, err
	}

	env := new(transactions.Envelope)
	env.TxHash = txHash
	env.Transaction = new(transactions.Transaction)
	env.Transaction.Body = dataToSign
	env.Transaction.TransactionHeader = *header

	ed := new(transactions.ED25519Sig)
	err = ed.Sign(header.Nonce, privKey, env.GetTxHash())
	if err != nil {
		return nil, err
	}

	params := &api2.TxRequest{}
	params.TxHash = txHash

	if TxPretend {
		params.CheckOnly = true
	}

	params.Signer.Nonce = header.Nonce
	params.Origin = header.Origin
	params.KeyPage.Height = header.KeyPageHeight
	params.KeyPage.Index = header.KeyPageIndex

	params.Signature = ed.GetSignature()
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Signer.PublicKey = ed.GetPublicKey()

	if leaveUnmarshalled {
		params.Payload = payload
	} else {
		params.Payload = hex.EncodeToString(dataToSign)
	}

	return params, err
}
