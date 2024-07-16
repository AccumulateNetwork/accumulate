// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// EthChainID returns the Ethereum chain ID for an Accumulate network name.
func EthChainID(name string) *big.Int {
	// This exists purely for documentation purposes
	// const MetaMask_MaxSafeChainID = 0xFFFFFFFFFFFEC

	if strings.EqualFold(name, "mainnet") {
		return big.NewInt(281) // 0x119
	}

	// Use the last 2 bytes of the hash
	name = strings.ToLower(name)
	hash := sha256.Sum256([]byte(name))
	id := binary.BigEndian.Uint16(hash[len(hash)-2:])

	// This will generate something like 0xHASH0119
	return big.NewInt(281 | int64(id)<<16)
}

var eip712Transaction *encoding.TypeDefinition

func init() {
	//need to handle the edge cases for Key page operations and data entries
	encoding.RegisterUnion(NewKeyPageOperation)
	encoding.RegisterUnion(NewAccountAuthOperation)
	encoding.RegisterUnion(NewDataEntry)

	txnFields := []*encoding.TypeField{
		encoding.NewTypeField("header", "TransactionHeader"),
		encoding.NewTypeField("signature", "SignatureMetadata"),

		// The loop below dynamically adds a field for each user transaction
		// types. These fields look like:
		//
		//    encoding.NewTypeField("addCredits", "AddCredits")
		//    encoding.NewTypeField("sendTokens", "SendTokens")
	}
	for i := TransactionType(0); i < TransactionType(TransactionMaxUser); i++ {
		v, err := NewTransactionBody(i)
		if err != nil {
			continue
		}
		name := v.Type().String()
		typ := reflect.TypeOf(v).Elem().Name()
		txnFields = append(txnFields, encoding.NewTypeField(name, typ))
	}
	eip712Transaction = &encoding.TypeDefinition{
		Name:   "Transaction",
		Fields: &txnFields,
	}

	encoding.RegisterTypeDefinition(&[]*encoding.TypeField{
		encoding.NewTypeField("type", "string"),
		encoding.NewTypeField("publicKey", "bytes"),
		encoding.NewTypeField("signer", "string"),
		encoding.NewTypeField("signerVersion", "uint64"),
		encoding.NewTypeField("timestamp", "uint64"),
		encoding.NewTypeField("vote", "string"),
		encoding.NewTypeField("memo", "string"),
		encoding.NewTypeField("data", "bytes"),
		encoding.NewTypeField("delegators", "string[]"),
	}, "SignatureMetadata", "signatureMetadata")
}

// MarshalEip712 creates the EIP-712 json message needed to submit a transaction
// and signature to an Ethereum wallet.
func MarshalEip712(txn *Transaction, sig Signature) (ret []byte, err error) {
	call, err := NewEIP712Call(txn, sig)
	if err != nil {
		return nil, err
	}
	return json.Marshal(call)
}

func EIP712Hash(txn *Transaction, sig Signature) ([]byte, error) {
	call, err := NewEIP712Call(txn, sig)
	if err != nil {
		return nil, err
	}
	return call.Hash()
}

// NewEIP712Call converts the transaction and signature to an EIP-712 message.
func NewEIP712Call(txn *Transaction, sig Signature) (*encoding.EIP712Call, error) {
	var delegators []any
	var inner *TypedDataSignature
	for inner == nil {
		switch s := sig.(type) {
		case *DelegatedSignature:
			delegators = append(delegators, s.Delegator.String())
			sig = s.Signature
		case *TypedDataSignature:
			inner = s
		default:
			return nil, fmt.Errorf("unsupported signature type %v", s.Type())
		}
	}

	j, err := json.Marshal(inner.Metadata())
	if err != nil {
		return nil, err
	}
	var jsig map[string]any
	err = json.Unmarshal(j, &jsig)
	if err != nil {
		return nil, err
	}
	if len(delegators) > 0 {
		jsig["delegators"] = delegators
	}

	j, err = txn.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var jtx map[string]any
	err = json.Unmarshal(j, &jtx)
	if err != nil {
		return nil, err
	}
	jtx["signature"] = jsig

	body := jtx["body"]
	delete(jtx, "body")
	delete(body.(map[string]any), "type")
	jtx[txn.Body.Type().String()] = body

	return encoding.NewEIP712Call(jtx, inner.ChainID, eip712Transaction)
}
