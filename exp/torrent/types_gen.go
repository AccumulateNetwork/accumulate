// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package torrent

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type ChunkMetadata struct {
	fieldsSet []bool
	Index     uint64   `json:"index,omitempty" form:"index" query:"index" validate:"required"`
	Size      uint64   `json:"size,omitempty" form:"size" query:"size" validate:"required"`
	Offset    uint64   `json:"offset,omitempty" form:"offset" query:"offset" validate:"required"`
	Hash      [32]byte `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"`
	extraData []byte
}

var machine_ChunkMetadata = &encoding.Machine[*ChunkMetadata]{
	ExtraData: func(v *ChunkMetadata) *[]byte { return &v.extraData },
	Seen:      func(v *ChunkMetadata) *[]bool { return &v.fieldsSet },
	Fields: []*encoding.Field[*ChunkMetadata]{
		{Name: "Index", Number: 1, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.UintField[*ChunkMetadata](func(v *ChunkMetadata) *uint64 { return &v.Index })},
		{Name: "Size", Number: 2, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.UintField[*ChunkMetadata](func(v *ChunkMetadata) *uint64 { return &v.Size })},
		{Name: "Offset", Number: 3, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.UintField[*ChunkMetadata](func(v *ChunkMetadata) *uint64 { return &v.Offset })},
		{Name: "Hash", Number: 4, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.HashField[*ChunkMetadata](func(v *ChunkMetadata) *[32]byte { return &v.Hash })},
	},
}

func (v *ChunkMetadata) IsValid() error               { return machine_ChunkMetadata.IsValid(v) }
func (v *ChunkMetadata) Copy() *ChunkMetadata         { return encoding.Copy(machine_ChunkMetadata, v) }
func (v *ChunkMetadata) CopyAsInterface() interface{} { return v.Copy() }
func (v *ChunkMetadata) Equal(u *ChunkMetadata) bool  { return machine_ChunkMetadata.Equal(v, u) }
func (v *ChunkMetadata) MarshalBinary() ([]byte, error) {
	return machine_ChunkMetadata.MarshalBinary(v)
}
func (v *ChunkMetadata) UnmarshalBinary(data []byte) error {
	return machine_ChunkMetadata.Unmarshal(data, v)
}
func (v *ChunkMetadata) UnmarshalBinaryFrom(rd io.Reader) error {
	return machine_ChunkMetadata.UnmarshalFrom(rd, v)
}
func (v *ChunkMetadata) MarshalJSON() ([]byte, error) { return machine_ChunkMetadata.JSONMarshal(v) }
func (v *ChunkMetadata) UnmarshalJSON(b []byte) error {
	return machine_ChunkMetadata.JSONUnmarshal(b, v)
}

type FileMetadata struct {
	fieldsSet []bool
	Chunks    []*ChunkMetadata `json:"chunks,omitempty" form:"chunks" query:"chunks" validate:"required"`
	extraData []byte
}

var machine_FileMetadata = &encoding.Machine[*FileMetadata]{
	ExtraData: func(v *FileMetadata) *[]byte { return &v.extraData },
	Seen:      func(v *FileMetadata) *[]bool { return &v.fieldsSet },
	Fields: []*encoding.Field[*FileMetadata]{
		{Name: "Chunks", Number: 1, Binary: true, OmitEmpty: true, Required: true, Accessor: encoding.SliceField[*FileMetadata, *ChunkMetadata, encoding.StructPtrField[encoding.SliceIndex[*ChunkMetadata], *ChunkMetadata, ChunkMetadata]](func(v *FileMetadata) *[]*ChunkMetadata { return &v.Chunks })},
	},
}

func (v *FileMetadata) IsValid() error                 { return machine_FileMetadata.IsValid(v) }
func (v *FileMetadata) Copy() *FileMetadata            { return encoding.Copy(machine_FileMetadata, v) }
func (v *FileMetadata) CopyAsInterface() interface{}   { return v.Copy() }
func (v *FileMetadata) Equal(u *FileMetadata) bool     { return machine_FileMetadata.Equal(v, u) }
func (v *FileMetadata) MarshalBinary() ([]byte, error) { return machine_FileMetadata.MarshalBinary(v) }
func (v *FileMetadata) UnmarshalBinary(data []byte) error {
	return machine_FileMetadata.Unmarshal(data, v)
}
func (v *FileMetadata) UnmarshalBinaryFrom(rd io.Reader) error {
	return machine_FileMetadata.UnmarshalFrom(rd, v)
}
func (v *FileMetadata) MarshalJSON() ([]byte, error) { return machine_FileMetadata.JSONMarshal(v) }
func (v *FileMetadata) UnmarshalJSON(b []byte) error { return machine_FileMetadata.JSONUnmarshal(b, v) }
