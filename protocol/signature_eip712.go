// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func init() {
	//need to handle the edge cases for Key page operations and data entries
	encoding.RegisterEnumeratedTypeInterface(NewKeyPageOperation)
	encoding.RegisterEnumeratedTypeInterface(NewDataEntry)

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

func NewEip712TransactionDefinition(txn *Transaction) *encoding.TypeDefinition {
	body := txn.Body.Type().String()
	txnSchema := &[]*encoding.TypeField{
		encoding.NewTypeField("header", "TransactionHeader"),
		encoding.NewTypeField("body", strings.ToUpper(body[:1])+body[1:]),
		encoding.NewTypeField("signature", "SignatureMetadata"),
	}

	return &encoding.TypeDefinition{Name: "Transaction", Fields: txnSchema}
}

// MarshalEip712 creates the EIP-712 json message needed to submit a transaction
// and signature to an Ethereum wallet.
func MarshalEip712(txn *Transaction, sig Signature) (ret []byte, err error) {
	call, err := newEIP712Call(txn, sig)
	if err != nil {
		return nil, err
	}
	return json.Marshal(call)
}

func EIP712Hash(txn *Transaction, sig Signature) ([]byte, error) {
	call, err := newEIP712Call(txn, sig)
	if err != nil {
		return nil, err
	}
	return call.Hash()
}

// newEIP712Call converts the transaction and signature to an EIP-712 message.
func newEIP712Call(txn *Transaction, sig Signature) (*encoding.EIP712Call, error) {
	var delegators []any
	var inner *Eip712TypedDataSignature
	for inner == nil {
		switch s := sig.(type) {
		case *DelegatedSignature:
			delegators = append(delegators, s.Delegator.String())
			sig = s.Signature
		case *Eip712TypedDataSignature:
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

	var jtx map[string]interface{}
	err = json.Unmarshal(j, &jtx)
	if err != nil {
		return nil, err
	}
	jtx["signature"] = jsig

	td := NewEip712TransactionDefinition(txn)
	return encoding.NewEIP712Call(jtx, td)
}
