// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type consensusDoc struct {
	fieldsSet  []bool
	ChainID    string              `json:"chainID,omitempty" form:"chainID" query:"chainID" validate:"required"`
	Params     *consensusParams    `json:"params,omitempty" form:"params" query:"params" validate:"required"`
	Validators []*genesisValidator `json:"validators,omitempty" form:"validators" query:"validators" validate:"required"`
	extraData  []byte
}

var machine_consensusDoc = &encoding.Machine[*consensusDoc]{
	ExtraData: func(v *consensusDoc) *[]byte { return &v.extraData },
	Seen:      func(v *consensusDoc) *[]bool { return &v.fieldsSet },
	Fields: []*encoding.Field[*consensusDoc]{
		{Name: "ChainID", Number: 1, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StringField[*consensusDoc](func(v *consensusDoc) *string { return &v.ChainID })},
		{Name: "Params", Number: 2, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StructPtrField[*consensusDoc, *consensusParams, consensusParams](func(v *consensusDoc) **consensusParams { return &v.Params })},
		{Name: "Validators", Number: 3, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.SliceField[*consensusDoc, *genesisValidator, encoding.StructPtrField[encoding.SliceIndex[*genesisValidator], *genesisValidator, genesisValidator]](func(v *consensusDoc) *[]*genesisValidator { return &v.Validators })},
	},
}

func (v *consensusDoc) IsValid() error                 { return machine_consensusDoc.IsValid(v) }
func (v *consensusDoc) Copy() *consensusDoc            { return encoding.Copy(machine_consensusDoc, v) }
func (v *consensusDoc) CopyAsInterface() interface{}   { return v.Copy() }
func (v *consensusDoc) Equal(u *consensusDoc) bool     { return machine_consensusDoc.Equal(v, u) }
func (v *consensusDoc) MarshalBinary() ([]byte, error) { return machine_consensusDoc.MarshalBinary(v) }
func (v *consensusDoc) UnmarshalBinary(data []byte) error {
	return machine_consensusDoc.Unmarshal(data, v)
}
func (v *consensusDoc) UnmarshalBinaryFrom(rd io.Reader) error {
	return machine_consensusDoc.UnmarshalFrom(rd, v)
}
func (v *consensusDoc) MarshalJSON() ([]byte, error) { return machine_consensusDoc.JSONMarshal(v) }
func (v *consensusDoc) UnmarshalJSON(b []byte) error { return machine_consensusDoc.JSONUnmarshal(b, v) }

type genesisValidator struct {
	fieldsSet []bool
	Address   []byte                 `json:"address,omitempty" form:"address" query:"address" validate:"required"`
	Type      protocol.SignatureType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	PubKey    []byte                 `json:"pubKey,omitempty" form:"pubKey" query:"pubKey" validate:"required"`
	Power     int64                  `json:"power,omitempty" form:"power" query:"power" validate:"required"`
	Name      string                 `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	extraData []byte
}

var machine_genesisValidator = &encoding.Machine[*genesisValidator]{
	ExtraData: func(v *genesisValidator) *[]byte { return &v.extraData },
	Seen:      func(v *genesisValidator) *[]bool { return &v.fieldsSet },
	Fields: []*encoding.Field[*genesisValidator]{
		{Name: "Address", Number: 1, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.BytesField[*genesisValidator](func(v *genesisValidator) *[]byte { return &v.Address })},
		{Name: "Type", Number: 2, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.EnumField[*genesisValidator, *protocol.SignatureType, protocol.SignatureType](func(v *genesisValidator) *protocol.SignatureType { return &v.Type })},
		{Name: "PubKey", Number: 3, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.BytesField[*genesisValidator](func(v *genesisValidator) *[]byte { return &v.PubKey })},
		{Name: "Power", Number: 4, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.IntField[*genesisValidator](func(v *genesisValidator) *int64 { return &v.Power })},
		{Name: "Name", Number: 5, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StringField[*genesisValidator](func(v *genesisValidator) *string { return &v.Name })},
	},
}

func (v *genesisValidator) IsValid() error { return machine_genesisValidator.IsValid(v) }
func (v *genesisValidator) Copy() *genesisValidator {
	return encoding.Copy(machine_genesisValidator, v)
}
func (v *genesisValidator) CopyAsInterface() interface{} { return v.Copy() }
func (v *genesisValidator) Equal(u *genesisValidator) bool {
	return machine_genesisValidator.Equal(v, u)
}
func (v *genesisValidator) MarshalBinary() ([]byte, error) {
	return machine_genesisValidator.MarshalBinary(v)
}
func (v *genesisValidator) UnmarshalBinary(data []byte) error {
	return machine_genesisValidator.Unmarshal(data, v)
}
func (v *genesisValidator) UnmarshalBinaryFrom(rd io.Reader) error {
	return machine_genesisValidator.UnmarshalFrom(rd, v)
}
func (v *genesisValidator) MarshalJSON() ([]byte, error) {
	return machine_genesisValidator.JSONMarshal(v)
}
func (v *genesisValidator) UnmarshalJSON(b []byte) error {
	return machine_genesisValidator.JSONUnmarshal(b, v)
}
