// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type CallSite struct {
	fieldsSet []bool
	FuncName  string `json:"funcName,omitempty" form:"funcName" query:"funcName" validate:"required"`
	File      string `json:"file,omitempty" form:"file" query:"file" validate:"required"`
	Line      int64  `json:"line,omitempty" form:"line" query:"line" validate:"required"`
	extraData []byte
}

type ErrorBase[Status statusType] struct {
	fieldsSet []bool
	Message   string             `json:"message,omitempty" form:"message" query:"message" validate:"required"`
	Code      Status             `json:"code,omitempty" form:"code" query:"code" validate:"required"`
	Cause     *ErrorBase[Status] `json:"cause,omitempty" form:"cause" query:"cause" validate:"required"`
	CallStack []*CallSite        `json:"callStack,omitempty" form:"callStack" query:"callStack" validate:"required"`
	Data      json.RawMessage    `json:"data,omitempty" form:"data" query:"data" validate:"required"`
	extraData []byte
}

func (v *CallSite) Copy() *CallSite {
	u := new(CallSite)

	u.FuncName = v.FuncName
	u.File = v.File
	u.Line = v.Line
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *CallSite) CopyAsInterface() interface{} { return v.Copy() }

func (v *ErrorBase[Status]) Copy() *ErrorBase[Status] {
	u := new(ErrorBase[Status])

	u.Message = v.Message
	u.Code = v.Code
	if v.Cause != nil {
		u.Cause = (v.Cause).Copy()
	}
	u.CallStack = make([]*CallSite, len(v.CallStack))
	for i, v := range v.CallStack {
		v := v
		if v != nil {
			u.CallStack[i] = (v).Copy()
		}
	}
	u.Data = encoding.BytesCopy(v.Data)
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *ErrorBase[Status]) CopyAsInterface() interface{} { return v.Copy() }

func (v *CallSite) Equal(u *CallSite) bool {
	if !(v.FuncName == u.FuncName) {
		return false
	}
	if !(v.File == u.File) {
		return false
	}
	if !(v.Line == u.Line) {
		return false
	}

	return true
}

func (v *ErrorBase[Status]) Equal(u *ErrorBase[Status]) bool {
	if !(v.Message == u.Message) {
		return false
	}
	if !(v.Code == u.Code) {
		return false
	}
	switch {
	case v.Cause == u.Cause:
		// equal
	case v.Cause == nil || u.Cause == nil:
		return false
	case !((v.Cause).Equal(u.Cause)):
		return false
	}
	if len(v.CallStack) != len(u.CallStack) {
		return false
	}
	for i := range v.CallStack {
		if !((v.CallStack[i]).Equal(u.CallStack[i])) {
			return false
		}
	}
	if !(bytes.Equal(v.Data, u.Data)) {
		return false
	}

	return true
}

var fieldNames_CallSite = []string{
	1: "FuncName",
	2: "File",
	3: "Line",
}

func (v *CallSite) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.FuncName) == 0) {
		writer.WriteString(1, v.FuncName)
	}
	if !(len(v.File) == 0) {
		writer.WriteString(2, v.File)
	}
	if !(v.Line == 0) {
		writer.WriteInt(3, v.Line)
	}

	_, _, err := writer.Reset(fieldNames_CallSite)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *CallSite) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field FuncName is missing")
	} else if len(v.FuncName) == 0 {
		errs = append(errs, "field FuncName is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field File is missing")
	} else if len(v.File) == 0 {
		errs = append(errs, "field File is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Line is missing")
	} else if v.Line == 0 {
		errs = append(errs, "field Line is not set")
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

var fieldNames_ErrorBase = []string{
	1: "Message",
	2: "Code",
	3: "Cause",
	4: "CallStack",
	5: "Data",
}

func (v *ErrorBase[Status]) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Message) == 0) {
		writer.WriteString(1, v.Message)
	}
	if !(v.Code == 0) {
		writer.WriteEnum(2, v.Code)
	}
	if !(v.Cause == nil) {
		writer.WriteValue(3, v.Cause.MarshalBinary)
	}
	if !(len(v.CallStack) == 0) {
		for _, v := range v.CallStack {
			writer.WriteValue(4, v.MarshalBinary)
		}
	}
	if !(len(v.Data) == 0) {
		writer.WriteBytes(5, v.Data)
	}

	_, _, err := writer.Reset(fieldNames_ErrorBase)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *ErrorBase[Status]) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Message is missing")
	} else if len(v.Message) == 0 {
		errs = append(errs, "field Message is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Code is missing")
	} else if v.Code == 0 {
		errs = append(errs, "field Code is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Cause is missing")
	} else if v.Cause == nil {
		errs = append(errs, "field Cause is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field CallStack is missing")
	} else if len(v.CallStack) == 0 {
		errs = append(errs, "field CallStack is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Data is missing")
	} else if len(v.Data) == 0 {
		errs = append(errs, "field Data is not set")
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

func (v *CallSite) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *CallSite) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.FuncName = x
	}
	if x, ok := reader.ReadString(2); ok {
		v.File = x
	}
	if x, ok := reader.ReadInt(3); ok {
		v.Line = x
	}

	seen, err := reader.Reset(fieldNames_CallSite)
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

func (v *ErrorBase[Status]) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *ErrorBase[Status]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Message = x
	}
	if x := new(Status); reader.ReadEnum(2, any(x).(encoding.EnumValueSetter)) {
		v.Code = *x
	}
	if x := new(ErrorBase[Status]); reader.ReadValue(3, x.UnmarshalBinaryFrom) {
		v.Cause = x
	}
	for {
		if x := new(CallSite); reader.ReadValue(4, x.UnmarshalBinaryFrom) {
			v.CallStack = append(v.CallStack, x)
		} else {
			break
		}
	}
	if x, ok := reader.ReadBytes(5); ok {
		v.Data = x
	}

	seen, err := reader.Reset(fieldNames_ErrorBase)
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

func (v *ErrorBase[Status]) MarshalJSON() ([]byte, error) {
	u := struct {
		Message   string                       `json:"message,omitempty"`
		Code      Status                       `json:"code,omitempty"`
		CodeID    uint64                       `json:"codeID,omitempty"`
		Cause     *ErrorBase[Status]           `json:"cause,omitempty"`
		CallStack encoding.JsonList[*CallSite] `json:"callStack,omitempty"`
		Data      json.RawMessage              `json:"data,omitempty"`
		ExtraData *string                      `json:"$epilogue,omitempty"`
	}{}
	if !(len(v.Message) == 0) {
		u.Message = v.Message
	}
	if !(v.Code == 0) {
		u.Code = v.Code
	}
	if !(v.CodeID() == 0) {
		u.CodeID = v.CodeID()
	}
	if !(v.Cause == nil) {
		u.Cause = v.Cause
	}
	if !(len(v.CallStack) == 0) {
		u.CallStack = v.CallStack
	}
	if !(len(v.Data) == 0) {
		u.Data = v.Data
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *ErrorBase[Status]) UnmarshalJSON(data []byte) error {
	u := struct {
		Message   string                       `json:"message,omitempty"`
		Code      Status                       `json:"code,omitempty"`
		CodeID    uint64                       `json:"codeID,omitempty"`
		Cause     *ErrorBase[Status]           `json:"cause,omitempty"`
		CallStack encoding.JsonList[*CallSite] `json:"callStack,omitempty"`
		Data      json.RawMessage              `json:"data,omitempty"`
		ExtraData *string                      `json:"$epilogue,omitempty"`
	}{}
	u.Message = v.Message
	u.Code = v.Code
	u.CodeID = v.CodeID()
	u.Cause = v.Cause
	u.CallStack = v.CallStack
	u.Data = v.Data
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.Message = u.Message
	v.Code = u.Code
	v.Cause = u.Cause
	v.CallStack = u.CallStack
	v.Data = u.Data
	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}
