// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type DbPatch struct {
	fieldsSet  []bool
	Operations []DbPatchOp `json:"operations,omitempty" form:"operations" query:"operations" validate:"required"`
	extraData  []byte
}

type DeleteDbPatchOp struct {
	fieldsSet []bool
	Key       *record.Key `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	extraData []byte
}

type PutDbPatchOp struct {
	fieldsSet []bool
	Key       *record.Key `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Value     []byte      `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	extraData []byte
}

func (*DeleteDbPatchOp) Type() DbPatchOpType { return DbPatchOpTypeDelete }

func (*PutDbPatchOp) Type() DbPatchOpType { return DbPatchOpTypePut }

func (v *DbPatch) Copy() *DbPatch {
	u := new(DbPatch)

	u.Operations = make([]DbPatchOp, len(v.Operations))
	for i, v := range v.Operations {
		if v != nil {
			u.Operations[i] = CopyDbPatchOp(v)
		}
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *DbPatch) CopyAsInterface() interface{} { return v.Copy() }

func (v *DeleteDbPatchOp) Copy() *DeleteDbPatchOp {
	u := new(DeleteDbPatchOp)

	if v.Key != nil {
		u.Key = (v.Key).Copy()
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *DeleteDbPatchOp) CopyAsInterface() interface{} { return v.Copy() }

func (v *PutDbPatchOp) Copy() *PutDbPatchOp {
	u := new(PutDbPatchOp)

	if v.Key != nil {
		u.Key = (v.Key).Copy()
	}
	u.Value = encoding.BytesCopy(v.Value)
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *PutDbPatchOp) CopyAsInterface() interface{} { return v.Copy() }

func (v *DbPatch) Equal(u *DbPatch) bool {
	if len(v.Operations) != len(u.Operations) {
		return false
	}
	for i := range v.Operations {
		if !(EqualDbPatchOp(v.Operations[i], u.Operations[i])) {
			return false
		}
	}

	return true
}

func (v *DeleteDbPatchOp) Equal(u *DeleteDbPatchOp) bool {
	switch {
	case v.Key == u.Key:
		// equal
	case v.Key == nil || u.Key == nil:
		return false
	case !((v.Key).Equal(u.Key)):
		return false
	}

	return true
}

func (v *PutDbPatchOp) Equal(u *PutDbPatchOp) bool {
	switch {
	case v.Key == u.Key:
		// equal
	case v.Key == nil || u.Key == nil:
		return false
	case !((v.Key).Equal(u.Key)):
		return false
	}
	if !(bytes.Equal(v.Value, u.Value)) {
		return false
	}

	return true
}

var fieldNames_DbPatch = []string{
	1: "Operations",
}

func (v *DbPatch) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Operations) == 0) {
		for _, v := range v.Operations {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_DbPatch)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *DbPatch) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Operations is missing")
	} else if len(v.Operations) == 0 {
		errs = append(errs, "field Operations is not set")
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

var fieldNames_DeleteDbPatchOp = []string{
	1: "Type",
	2: "Key",
}

func (v *DeleteDbPatchOp) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteEnum(1, v.Type())
	if !(v.Key == nil) {
		writer.WriteValue(2, v.Key.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_DeleteDbPatchOp)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *DeleteDbPatchOp) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Type is missing")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Key is missing")
	} else if v.Key == nil {
		errs = append(errs, "field Key is not set")
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

var fieldNames_PutDbPatchOp = []string{
	1: "Type",
	2: "Key",
	3: "Value",
}

func (v *PutDbPatchOp) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteEnum(1, v.Type())
	if !(v.Key == nil) {
		writer.WriteValue(2, v.Key.MarshalBinary)
	}
	if !(len(v.Value) == 0) {
		writer.WriteBytes(3, v.Value)
	}

	_, _, err := writer.Reset(fieldNames_PutDbPatchOp)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *PutDbPatchOp) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Type is missing")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Key is missing")
	} else if v.Key == nil {
		errs = append(errs, "field Key is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Value is missing")
	} else if len(v.Value) == 0 {
		errs = append(errs, "field Value is not set")
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

func (v *DbPatch) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *DbPatch) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		ok := reader.ReadValue(1, func(r io.Reader) error {
			x, err := UnmarshalDbPatchOpFrom(r)
			if err == nil {
				v.Operations = append(v.Operations, x)
			}
			return err
		})
		if !ok {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_DbPatch)
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

func (v *DeleteDbPatchOp) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *DeleteDbPatchOp) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	var vType DbPatchOpType
	if x := new(DbPatchOpType); reader.ReadEnum(1, x) {
		vType = *x
	}
	if !(v.Type() == vType) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), vType)
	}

	return v.UnmarshalFieldsFrom(reader)
}

func (v *DeleteDbPatchOp) UnmarshalFieldsFrom(reader *encoding.Reader) error {
	if x := new(record.Key); reader.ReadValue(2, x.UnmarshalBinaryFrom) {
		v.Key = x
	}

	seen, err := reader.Reset(fieldNames_DeleteDbPatchOp)
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

func (v *PutDbPatchOp) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *PutDbPatchOp) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	var vType DbPatchOpType
	if x := new(DbPatchOpType); reader.ReadEnum(1, x) {
		vType = *x
	}
	if !(v.Type() == vType) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), vType)
	}

	return v.UnmarshalFieldsFrom(reader)
}

func (v *PutDbPatchOp) UnmarshalFieldsFrom(reader *encoding.Reader) error {
	if x := new(record.Key); reader.ReadValue(2, x.UnmarshalBinaryFrom) {
		v.Key = x
	}
	if x, ok := reader.ReadBytes(3); ok {
		v.Value = x
	}

	seen, err := reader.Reset(fieldNames_PutDbPatchOp)
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

func (v *DbPatch) MarshalJSON() ([]byte, error) {
	u := struct {
		Operations *encoding.JsonUnmarshalListWith[DbPatchOp] `json:"operations,omitempty"`
	}{}
	if !(len(v.Operations) == 0) {
		u.Operations = &encoding.JsonUnmarshalListWith[DbPatchOp]{Value: v.Operations, Func: UnmarshalDbPatchOpJSON}
	}
	return json.Marshal(&u)
}

func (v *DeleteDbPatchOp) MarshalJSON() ([]byte, error) {
	u := struct {
		Type DbPatchOpType `json:"type"`
		Key  *record.Key   `json:"key,omitempty"`
	}{}
	u.Type = v.Type()
	if !(v.Key == nil) {
		u.Key = v.Key
	}
	return json.Marshal(&u)
}

func (v *PutDbPatchOp) MarshalJSON() ([]byte, error) {
	u := struct {
		Type  DbPatchOpType `json:"type"`
		Key   *record.Key   `json:"key,omitempty"`
		Value *string       `json:"value,omitempty"`
	}{}
	u.Type = v.Type()
	if !(v.Key == nil) {
		u.Key = v.Key
	}
	if !(len(v.Value) == 0) {
		u.Value = encoding.BytesToJSON(v.Value)
	}
	return json.Marshal(&u)
}

func (v *DbPatch) UnmarshalJSON(data []byte) error {
	u := struct {
		Operations *encoding.JsonUnmarshalListWith[DbPatchOp] `json:"operations,omitempty"`
	}{}
	u.Operations = &encoding.JsonUnmarshalListWith[DbPatchOp]{Value: v.Operations, Func: UnmarshalDbPatchOpJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if u.Operations != nil {
		v.Operations = make([]DbPatchOp, len(u.Operations.Value))
		for i, x := range u.Operations.Value {
			v.Operations[i] = x
		}
	}
	return nil
}

func (v *DeleteDbPatchOp) UnmarshalJSON(data []byte) error {
	u := struct {
		Type DbPatchOpType `json:"type"`
		Key  *record.Key   `json:"key,omitempty"`
	}{}
	u.Type = v.Type()
	u.Key = v.Key
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if !(v.Type() == u.Type) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), u.Type)
	}
	v.Key = u.Key
	return nil
}

func (v *PutDbPatchOp) UnmarshalJSON(data []byte) error {
	u := struct {
		Type  DbPatchOpType `json:"type"`
		Key   *record.Key   `json:"key,omitempty"`
		Value *string       `json:"value,omitempty"`
	}{}
	u.Type = v.Type()
	u.Key = v.Key
	u.Value = encoding.BytesToJSON(v.Value)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if !(v.Type() == u.Type) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), u.Type)
	}
	v.Key = u.Key
	if x, err := encoding.BytesFromJSON(u.Value); err != nil {
		return fmt.Errorf("error decoding Value: %w", err)
	} else {
		v.Value = x
	}
	return nil
}
