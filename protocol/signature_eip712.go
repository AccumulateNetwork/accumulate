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
	txnSchema := &[]*encoding.TypeField{
		encoding.NewTypeField("header", "TransactionHeader"),
		encoding.NewTypeField("body", txn.Body.Type().String()),
		encoding.NewTypeField("signature", "SignatureMetadata"),
	}

	return &encoding.TypeDefinition{Fields: txnSchema}
}

// MarshalEip712 This will create an EIP-712 json message needed to submit to a
// wallet.
func MarshalEip712(txn *Transaction, sig Signature) (ret []byte, err error) {
	// Convert the transaction and signature to an EIP-712 message
	jtx, err := makeEIP712Message(txn, sig)
	if err != nil {
		return nil, err
	}
	r, err := NewEip712TransactionDefinition(txn).Resolve(jtx, "Transaction")
	if err != nil {
		return nil, err
	}

	// Construct the wallet RPC call
	type eip712 struct {
		Types       map[string][]*encoding.TypeField `json:"types"`
		PrimaryType string                           `json:"primaryType"`
		Domain      encoding.EIP712Domain            `json:"domain"`
		Message     json.RawMessage                  `json:"message"`
	}
	e := eip712{}
	e.PrimaryType = "Transaction"
	e.Domain = encoding.Eip712Domain

	e.Message, err = r.MarshalJSON()
	if err != nil {
		return nil, err
	}

	e.Types = map[string][]*encoding.TypeField{}
	r.Types(e.Types)
	encoding.EIP712DomainValue.Types(e.Types)

	return json.Marshal(e)
}

func makeEIP712Message(txn *Transaction, sig Signature) (map[string]any, error) {
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

	return jtx, nil
}

func EIP712Hash(txn *Transaction, sig Signature) ([]byte, error) {
	jtx, err := makeEIP712Message(txn, sig)
	if err != nil {
		return nil, err
	}

	h, err := encoding.Eip712Hash(jtx, "Transaction", NewEip712TransactionDefinition(txn))
	if err != nil {
		return nil, err
	}

	return h, nil
}
