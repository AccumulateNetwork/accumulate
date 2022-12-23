// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

type RecordType int

type CollectionType int

type Record interface {
	Type() RecordType
	GetParent() *EntityRecord
	GetParameters() []*Field
	FullName() string
	GetName() string
}

type ValueRecord interface {
	Record
	Wrapped() bool
	CollectionType() CollectionType
	IsSet() bool
	IsList() bool
	IsCounted() bool
	IsPointer() bool
	IsUnion() bool
	GetDataType() FieldType
}

func (t RecordType) IsEntity() bool { return t == RecordTypeEntity }
func (t RecordType) IsChain() bool  { return t == RecordTypeChain }
func (t RecordType) IsState() bool  { return t == RecordTypeState }
func (t RecordType) IsIndex() bool  { return t == RecordTypeIndex }
func (t RecordType) IsOther() bool  { return t == RecordTypeOther }

func (r *RecordFields) GetName() string          { return r.Name }
func (r *RecordFields) GetParameters() []*Field  { return r.Parameters }
func (r *RecordFields) GetParent() *EntityRecord { return r.Parent }

func (r *EntityRecord) FullName() string {
	if r.CustomFullName != "" {
		return r.CustomFullName
	}
	if r.Root {
		return r.Name
	}
	return recordFullName(r)
}

func (r *ChainRecord) FullName() string { return recordFullName(r) }
func (r *StateRecord) FullName() string { return recordFullName(r) }
func (r *IndexRecord) FullName() string { return recordFullName(r) }
func (r *OtherRecord) FullName() string { return recordFullName(r) }

func (r *StateRecord) Wrapped() bool { return r.DataType.Code != TypeCodeUnknown }
func (r *IndexRecord) Wrapped() bool { return r.DataType.Code != TypeCodeUnknown }

func (r *StateRecord) CollectionType() CollectionType { return r.Collection }
func (r *IndexRecord) CollectionType() CollectionType { return r.Collection }

func (r *StateRecord) IsSet() bool { return r.Collection == CollectionTypeSet }
func (r *IndexRecord) IsSet() bool { return r.Collection == CollectionTypeSet }

func (r *StateRecord) IsList() bool { return r.Collection == CollectionTypeList }
func (r *IndexRecord) IsList() bool { return r.Collection == CollectionTypeList }

func (r *StateRecord) IsCounted() bool { return r.Collection == CollectionTypeCounted }
func (r *IndexRecord) IsCounted() bool { return r.Collection == CollectionTypeCounted }

func (r *StateRecord) IsUnion() bool { return r.Union }
func (r *IndexRecord) IsUnion() bool { return r.Union }

func (r *StateRecord) IsPointer() bool { return r.Pointer }
func (r *IndexRecord) IsPointer() bool { return r.Pointer }

func (r *StateRecord) GetDataType() FieldType { return r.DataType }
func (r *IndexRecord) GetDataType() FieldType { return r.DataType }

func recordFullName(r Record) string {
	if entity, ok := r.(*EntityRecord); ok && entity.Root {
		return ""
	}
	if r.GetParent() == nil {
		return r.GetName()
	}
	return recordFullName(r.GetParent()) + r.GetName()
}
