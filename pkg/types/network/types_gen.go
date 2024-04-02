// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type GlobalValues struct {
	fieldsSet []bool
	memoize   globalValueMemos
	Oracle    *protocol.AcmeOracle        `json:"oracle,omitempty" form:"oracle" query:"oracle" validate:"required"`
	Globals   *protocol.NetworkGlobals    `json:"globals,omitempty" form:"globals" query:"globals" validate:"required"`
	Network   *protocol.NetworkDefinition `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Routing   *protocol.RoutingTable      `json:"routing,omitempty" form:"routing" query:"routing" validate:"required"`
	// ExecutorVersion is the active executor version.
	ExecutorVersion protocol.ExecutorVersion `json:"executorVersion,omitempty" form:"executorVersion" query:"executorVersion"`
	// BvnExecutorVersions is the active executor version of each BVN.
	BvnExecutorVersions []*protocol.PartitionExecutorVersion `json:"bvnExecutorVersions,omitempty" form:"bvnExecutorVersions" query:"bvnExecutorVersions" validate:"required"`
	extraData           []byte
}

var machine_GlobalValues = &encoding.Machine[*GlobalValues]{
	ExtraData: func(v *GlobalValues) *[]byte { return &v.extraData },
	Seen:      func(v *GlobalValues) *[]bool { return &v.fieldsSet },
	Fields: []*encoding.Field[*GlobalValues]{
		{Name: "Oracle", Number: 1, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StructPtrField[*GlobalValues, *protocol.AcmeOracle, protocol.AcmeOracle](func(v *GlobalValues) **protocol.AcmeOracle { return &v.Oracle })},
		{Name: "Globals", Number: 2, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StructPtrField[*GlobalValues, *protocol.NetworkGlobals, protocol.NetworkGlobals](func(v *GlobalValues) **protocol.NetworkGlobals { return &v.Globals })},
		{Name: "Network", Number: 3, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StructPtrField[*GlobalValues, *protocol.NetworkDefinition, protocol.NetworkDefinition](func(v *GlobalValues) **protocol.NetworkDefinition { return &v.Network })},
		{Name: "Routing", Number: 4, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.StructPtrField[*GlobalValues, *protocol.RoutingTable, protocol.RoutingTable](func(v *GlobalValues) **protocol.RoutingTable { return &v.Routing })},
		{Name: "ExecutorVersion", Number: 5, Binary: true, OmitEmpty: true, Accessor: encoding.EnumField[*GlobalValues, *protocol.ExecutorVersion, protocol.ExecutorVersion](func(v *GlobalValues) *protocol.ExecutorVersion { return &v.ExecutorVersion })},
		{Name: "BvnExecutorVersions", Number: 6, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.SliceField[*GlobalValues, *protocol.PartitionExecutorVersion, encoding.StructPtrField[encoding.SliceIndex[*protocol.PartitionExecutorVersion], *protocol.PartitionExecutorVersion, protocol.PartitionExecutorVersion]](func(v *GlobalValues) *[]*protocol.PartitionExecutorVersion { return &v.BvnExecutorVersions })},
	},
}

func (v *GlobalValues) IsValid() error                 { return machine_GlobalValues.IsValid(v) }
func (v *GlobalValues) Copy() *GlobalValues            { return encoding.Copy(machine_GlobalValues, v) }
func (v *GlobalValues) CopyAsInterface() interface{}   { return v.Copy() }
func (v *GlobalValues) Equal(u *GlobalValues) bool     { return machine_GlobalValues.Equal(v, u) }
func (v *GlobalValues) MarshalBinary() ([]byte, error) { return machine_GlobalValues.MarshalBinary(v) }
func (v *GlobalValues) UnmarshalBinary(data []byte) error {
	return machine_GlobalValues.Unmarshal(data, v)
}
func (v *GlobalValues) UnmarshalBinaryFrom(rd io.Reader) error {
	return machine_GlobalValues.UnmarshalFrom(rd, v)
}
func (v *GlobalValues) MarshalJSON() ([]byte, error) { return machine_GlobalValues.JSONMarshal(v) }
func (v *GlobalValues) UnmarshalJSON(b []byte) error { return machine_GlobalValues.JSONUnmarshal(b, v) }
