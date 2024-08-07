// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type LastBlock struct {
	fieldsSet []bool
	Index     uint64    `json:"index,omitempty" form:"index" query:"index" validate:"required"`
	Time      time.Time `json:"time,omitempty" form:"time" query:"time" validate:"required"`
	extraData []byte
}

func (v *LastBlock) Copy() *LastBlock {
	u := new(LastBlock)

	u.Index = v.Index
	u.Time = v.Time
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *LastBlock) CopyAsInterface() interface{} { return v.Copy() }

func (v *LastBlock) Equal(u *LastBlock) bool {
	if !(v.Index == u.Index) {
		return false
	}
	if !((v.Time).Equal(u.Time)) {
		return false
	}

	return true
}

var fieldNames_LastBlock = []string{
	1: "Index",
	2: "Time",
}

func (v *LastBlock) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Index == 0) {
		writer.WriteUint(1, v.Index)
	}
	if !(v.Time == (time.Time{})) {
		writer.WriteTime(2, v.Time)
	}

	_, _, err := writer.Reset(fieldNames_LastBlock)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *LastBlock) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Index is missing")
	} else if v.Index == 0 {
		errs = append(errs, "field Index is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Time is missing")
	} else if v.Time == (time.Time{}) {
		errs = append(errs, "field Time is not set")
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

func (v *LastBlock) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *LastBlock) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Index = x
	}
	if x, ok := reader.ReadTime(2); ok {
		v.Time = x
	}

	seen, err := reader.Reset(fieldNames_LastBlock)
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
		encoding.NewTypeField("index", "uint64"),
		encoding.NewTypeField("time", "string"),
	}, "LastBlock", "lastBlock")

}
