// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Signature struct {
	fieldsSet []bool
	Txid      *url.TxID          `json:"txid,omitempty" form:"txid" query:"txid" validate:"required"`
	Signature protocol.Signature `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
	extraData []byte
}

type sigSection struct {
	fieldsSet  []bool
	Signatures []*Signature `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
	extraData  []byte
}

func (v *Signature) Copy() *Signature {
	u := new(Signature)

	if v.Txid != nil {
		u.Txid = v.Txid
	}
	if v.Signature != nil {
		u.Signature = protocol.CopySignature(v.Signature)
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *Signature) CopyAsInterface() interface{} { return v.Copy() }

func (v *sigSection) Copy() *sigSection {
	u := new(sigSection)

	u.Signatures = make([]*Signature, len(v.Signatures))
	for i, v := range v.Signatures {
		v := v
		if v != nil {
			u.Signatures[i] = (v).Copy()
		}
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *sigSection) CopyAsInterface() interface{} { return v.Copy() }

func (v *Signature) Equal(u *Signature) bool {
	switch {
	case v.Txid == u.Txid:
		// equal
	case v.Txid == nil || u.Txid == nil:
		return false
	case !((v.Txid).Equal(u.Txid)):
		return false
	}
	if !(protocol.EqualSignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

func (v *sigSection) Equal(u *sigSection) bool {
	if len(v.Signatures) != len(u.Signatures) {
		return false
	}
	for i := range v.Signatures {
		if !((v.Signatures[i]).Equal(u.Signatures[i])) {
			return false
		}
	}

	return true
}

var fieldNames_Signature = []string{
	1: "Txid",
	2: "Signature",
}

func (v *Signature) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Txid == nil) {
		writer.WriteTxid(1, v.Txid)
	}
	if !(protocol.EqualSignature(v.Signature, nil)) {
		writer.WriteValue(2, v.Signature.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_Signature)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Signature) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Txid is missing")
	} else if v.Txid == nil {
		errs = append(errs, "field Txid is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Signature is missing")
	} else if protocol.EqualSignature(v.Signature, nil) {
		errs = append(errs, "field Signature is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_sigSection = []string{
	1: "Signatures",
}

func (v *sigSection) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Signatures) == 0) {
		for _, v := range v.Signatures {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_sigSection)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *sigSection) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Signatures is missing")
	} else if len(v.Signatures) == 0 {
		errs = append(errs, "field Signatures is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

func (v *Signature) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Signature) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadTxid(1); ok {
		v.Txid = x
	}
	reader.ReadValue(2, func(r io.Reader) error {
		x, err := protocol.UnmarshalSignatureFrom(r)
		if err == nil {
			v.Signature = x
		}
		return err
	})

	seen, err := reader.Reset(fieldNames_Signature)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *sigSection) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *sigSection) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x := new(Signature); reader.ReadValue(1, x.UnmarshalBinaryFrom) {
			v.Signatures = append(v.Signatures, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_sigSection)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func init() {

	encoding.RegisterTypeDefinition(&[]*encoding.TypeField{
		encoding.NewTypeField("txid", "string"),
		encoding.NewTypeField("signature", "protocol.Signature"),
	}, "Signature", "signature")

	encoding.RegisterTypeDefinition(&[]*encoding.TypeField{
		encoding.NewTypeField("signatures", "Signature[]"),
	}, "sigSection", "sigSection")

}

func (v *Signature) MarshalJSON() ([]byte, error) {
	u := struct {
		Txid      *url.TxID                                       `json:"txid,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
		ExtraData *string                                         `json:"$epilogue,omitempty"`
	}{}
	if !(v.Txid == nil) {
		u.Txid = v.Txid
	}
	if !(protocol.EqualSignature(v.Signature, nil)) {
		u.Signature = &encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *sigSection) MarshalJSON() ([]byte, error) {
	u := struct {
		Signatures encoding.JsonList[*Signature] `json:"signatures,omitempty"`
		ExtraData  *string                       `json:"$epilogue,omitempty"`
	}{}
	if !(len(v.Signatures) == 0) {
		u.Signatures = v.Signatures
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *Signature) UnmarshalJSON(data []byte) error {
	u := struct {
		Txid      *url.TxID                                       `json:"txid,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
		ExtraData *string                                         `json:"$epilogue,omitempty"`
	}{}
	u.Txid = v.Txid
	u.Signature = &encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.Txid = u.Txid
	if u.Signature != nil {
		v.Signature = u.Signature.Value
	}

	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}

func (v *sigSection) UnmarshalJSON(data []byte) error {
	u := struct {
		Signatures encoding.JsonList[*Signature] `json:"signatures,omitempty"`
		ExtraData  *string                       `json:"$epilogue,omitempty"`
	}{}
	u.Signatures = v.Signatures
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.Signatures = u.Signatures
	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}
