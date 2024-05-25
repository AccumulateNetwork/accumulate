// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding"
	"io"
)

type TypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TypeDefinition map[string]*[]TypeField

var SchemaDictionary TypeDefinition
var resolvers map[string]func()

func RegisterTypeDefinitionResolver(name string, deferFunc func()) {
	if resolvers == nil {
		resolvers = make(map[string]func())
	}
	resolvers[name] = deferFunc
}

func UnregisterTypeDefinitionResolver(name string) {
	if resolvers == nil {
		return
	}
	delete(resolvers, name)
}

func ResolveTypeDefinitions() {
	//make a copy in case resolver removes itself from the map
	rs := resolvers
	for _, v := range rs {
		v()
	}
}

type Error struct {
	E error
}

func (e Error) Error() string { return e.E.Error() }
func (e Error) Unwrap() error { return e.E }

type EnumValueGetter interface {
	GetEnumValue() uint64
}

type EnumValueSetter interface {
	SetEnumValue(uint64) bool
}

type BinaryValue interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	CopyAsInterface() interface{}
	UnmarshalBinaryFrom(io.Reader) error
}

type UnionValue interface {
	BinaryValue
	UnmarshalFieldsFrom(reader *Reader) error
}
