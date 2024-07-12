// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"sort"
	"strings"

	"golang.org/x/crypto/sha3"
)

type TypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TypeDefinition struct {
	Name   string
	Fields *[]*TypeField
}

type TypeSet map[string][]*TypeField

func (s TypeSet) AddField(base, name, typ string) {
	fields, ok := s[base]
	if !ok {
		s[base] = []*TypeField{{name, typ}}
		return
	}

	for _, f := range fields {
		if f.Name == name {
			return
		}
	}
	fields = append(fields, &TypeField{name, typ})
	slices.SortFunc(fields, func(a, b *TypeField) int {
		return strings.Compare(a.Name, b.Name)
	})
	s[base] = fields
}

type EIP712Domain struct {
	Name    string   `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Version string   `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	ChainId *big.Int `json:"chainId,omitempty" form:"chainId" query:"chainId" validate:"required"`
}

var EIP712DomainValue eipResolvedValue
var EIP712DomainHash []byte
var Eip712Domain = EIP712Domain{
	Name:    "Accumulate",
	Version: "1.0.0",
	ChainId: big.NewInt(281),
}

type EIP712Resolver interface {
	Resolve(any, string) (eipResolvedValue, error)
}

type eipResolvedValue interface {
	Hash(types map[string][]*TypeField) ([]byte, error)
	Types(TypeSet)
	MarshalJSON() ([]byte, error)
}

var eip712EncoderMap = map[string]EIP712Resolver{}
var schemaDictionary = map[string]*TypeDefinition{}

func init() {
	eip712EncoderMap["bool"] = newAtomicEncoder("bool", FromboolToBytes)
	eip712EncoderMap["bytes"] = newAtomicEncoder("bytes", FrombytesToBytes)
	eip712EncoderMap["bytes32"] = newAtomicEncoder("bytes32", Frombytes32ToBytes)
	eip712EncoderMap["int64"] = newAtomicEncoder("int64", Fromint64ToBytes)
	eip712EncoderMap["uint64"] = newAtomicEncoder("uint64", Fromuint64ToBytes)
	eip712EncoderMap["string"] = newAtomicEncoder("string", FromstringToBytes)
	eip712EncoderMap["address"] = newAtomicEncoder("address", FromaddressToBytes)
	eip712EncoderMap["uint256"] = newAtomicEncoder("uint256", Fromuint256ToBytes)
	eip712EncoderMap["float64"] = newAtomicEncoder("float64", FromfloatToBytes)
	eip712EncoderMap["float"] = newAtomicEncoder("float", FromfloatToBytes) //Note = Float is not a valid type in EIP-712, so it is converted to a string

	// Handle EIP712 domain initialization
	var jdomain map[string]interface{}
	j := must2(Eip712Domain.MarshalJSON())
	must(json.Unmarshal(j, &jdomain))

	const eipDomainKey = "EIP712Domain"
	RegisterTypeDefinition(&[]*TypeField{
		NewTypeField("name", "string"),
		NewTypeField("version", "string"),
		NewTypeField("chainId", "uint256"),
	}, eipDomainKey)

	td := schemaDictionary[eipDomainKey]
	EIP712DomainValue = must2(td.Resolve(jdomain, eipDomainKey))
	EIP712DomainHash = must2(EIP712DomainValue.Hash(map[string][]*TypeField{
		eipDomainKey: *td.Fields,
	}))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func must2[V any](v V, err error) V {
	must(err)
	return v
}

type Func[T any, R any] func(T) (R, error)
type Enum interface {
	SetEnumValue(id uint64) bool
	String() string
}

func RegisterEnumeratedTypeInterface[T any, R any](op Func[T, R]) {
	//need to store allocator
	typesMap := make(map[string]string)
	enumType := new(T)
	enumValue, ok := any(enumType).(Enum)
	if !ok {
		panic(fmt.Errorf("%T is not an enumeration type", *enumType))
	}
	var a *R
	tp := reflect.TypeOf(a).Elem().String()
	//strip package name if present
	idx := strings.LastIndex(tp, ".")
	if idx != -1 {
		tp = tp[idx+1:]
	}
	//build a map of types
	for maxType := uint64(0); ; maxType++ {
		if !enumValue.SetEnumValue(maxType) {
			break
		}
		op, err := op(*enumType)
		if err != nil {
			continue
		}

		t := reflect.TypeOf(op)
		if t.Kind() == reflect.Ptr {
			t = t.Elem() // Safely obtaining the element type
		}

		name := t.Name()
		idx := strings.LastIndex(name, ".")
		if idx > 0 {
			name = name[idx+1:]
		}

		key := enumValue.String()
		typesMap[key] = name
	}
	eip712EncoderMap[tp] = eip712EnumResolver(typesMap)
}

type eip712EnumResolver map[string]string

func (r eip712EnumResolver) Resolve(v any, typeName string) (eipResolvedValue, error) {
	//this is a complex structure, so upcast it to an interface map
	vv, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", v)
	}

	t, ok := vv["type"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	a, ok := r[t]
	if !ok {
		return nil, fmt.Errorf("type alias does not exist in map: %T, %v", v, t)
	}

	d, ok := schemaDictionary[a]
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	rv, err := d.Resolve(vv, typeName)
	if err != nil {
		return nil, err
	}

	return &eipResolvedUnionValue{
		union:  typeName,
		enum:   t,
		member: a,
		value:  rv,
	}, nil
}

type eipResolvedUnionValue struct {
	union  string
	enum   string
	member string
	value  eipResolvedValue
}

func (e *eipResolvedUnionValue) Hash(types map[string][]*TypeField) ([]byte, error) {
	return hashStruct(e.union, types, func(fn func(encodedValue []byte)) error {
		for _, field := range types[e.union] {
			if field.Name != e.enum {
				fn(make([]byte, 32))
				continue
			}

			encodedValue, err := e.value.Hash(types)
			if err != nil {
				return err
			}
			fn(encodedValue)
		}
		return nil
	})
}

func (e *eipResolvedUnionValue) Types(ret TypeSet) {
	ret.AddField(e.union, e.enum, e.member)
	e.value.Types(ret)
}

func (e *eipResolvedUnionValue) MarshalJSON() ([]byte, error) {
	b, err := e.value.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]json.RawMessage{e.enum: b})
}

func (td *TypeDefinition) Resolve(v any, typeName string) (eipResolvedValue, error) {
	data, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cannot hash type definition with invalid interface %T", v)
	}

	fields := map[string]*resolvedFieldValue{}
	for _, field := range *td.Fields {
		value, _ := data[field.Name]
		v, err := field.resolve(value)
		if err != nil {
			return nil, err
		}
		if v != nil {
			fields[field.Name] = v
		}
	}

	return &eipResolvedStruct{
		typeName: td.Name,
		fields:   fields,
	}, nil
}

type eipResolvedStruct struct {
	typeName string
	fields   map[string]*resolvedFieldValue
}

type resolvedFieldValue struct {
	TypeField
	empty bool
	value eipResolvedValue
}

type eipResolvedArray []eipResolvedValue

func (e eipResolvedArray) MarshalJSON() ([]byte, error) {
	return json.Marshal([]eipResolvedValue(e))
}

type eipResolvedAtomic struct {
	ethType string
	value   any
	hash    func() ([]byte, error)
}

func (e *eipResolvedStruct) MarshalJSON() ([]byte, error) {
	v := map[string]json.RawMessage{}
	for _, f := range e.fields {
		var err error
		v[f.Name], err = f.value.MarshalJSON()
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(v)
}

func (e *eipResolvedStruct) Hash(types map[string][]*TypeField) ([]byte, error) {
	//the stripping shouldn't be necessary, but do it as a precaution
	strippedType, _ := stripSlice(e.typeName)

	return hashStruct(strippedType, types, func(fn func(encodedValue []byte)) error {
		for _, field := range types[strippedType] {
			f, ok := e.fields[field.Name]
			if !ok {
				fn(make([]byte, 32))
				continue
			}

			encodedValue, err := f.value.Hash(types)
			if err != nil {
				return err
			}
			fn(encodedValue)
		}
		return nil
	})
}

func getDeps(deps map[string]bool, typeName string, types map[string][]*TypeField) {
	fields, ok := types[typeName]
	if !ok {
		return
	}

	deps[typeName] = true
	for _, f := range fields {
		getDeps(deps, strings.TrimSuffix(f.Type, "[]"), types)
	}
}

const debugHash = false

func hashStruct(typeName string, types map[string][]*TypeField, rangeFields func(func(encodedValue []byte)) error) ([]byte, error) {
	deps := map[string]bool{}
	getDeps(deps, typeName, types)

	depNames := []string{typeName}
	delete(deps, typeName)
	for name := range deps {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames[1:])

	var header bytes.Buffer
	for _, name := range depNames {
		header.WriteString(name)
		header.WriteRune('(')
		for i, f := range types[name] {
			if i > 0 {
				header.WriteRune(',')
			}
			header.WriteString(f.Type)
			header.WriteRune(' ')
			header.WriteString(f.Name)
		}
		header.WriteRune(')')
	}

	var buf bytes.Buffer
	hash := keccak256(header.Bytes())
	buf.Write(hash)

	var parts [][]byte
	if debugHash {
		parts = append(parts, hash)
	}

	err := rangeFields(func(encodedValue []byte) {
		buf.Write(encodedValue)
		if debugHash {
			parts = append(parts, encodedValue)
		}
	})
	if err != nil {
		return nil, err
	}

	hash = keccak256(buf.Bytes())
	if debugHash {
		fmt.Println(header.String())
		for i, v := range parts {
			if i == 0 {
				fmt.Printf("    %x\n", v)
			} else {
				fmt.Printf("  + %x\n", v)
			}
		}
		fmt.Printf("  = %x\n\n", hash)
	}

	return hash, nil
}

func (e *eipResolvedStruct) Types(ret TypeSet) {
	name, _ := stripSlice(e.typeName)
	for _, f := range e.fields {
		if f.empty {
			continue
		}
		ret.AddField(name, f.Name, f.Type)
		f.value.Types(ret)
	}
}

func (e eipResolvedArray) Hash(types map[string][]*TypeField) ([]byte, error) {
	var buf bytes.Buffer
	for _, v := range e {
		hash, err := v.Hash(types)
		if err != nil {
			return nil, err
		}
		_, _ = buf.Write(hash)
	}
	return keccak256(buf.Bytes()), nil
}

func (e eipResolvedArray) Types(ret TypeSet) {
	for _, v := range e {
		v.Types(ret)
	}
}

func (e *eipResolvedAtomic) Types(TypeSet) {}

func (e *eipResolvedAtomic) Hash(map[string][]*TypeField) ([]byte, error) { return e.hash() }

func (e *eipResolvedAtomic) MarshalJSON() ([]byte, error) {
	v := e.value
	switch e.ethType {
	case "bytes", "bytes32", "address":
		if s, ok := v.(string); ok && s == "" {
			return json.Marshal("")
		}
		v = fmt.Sprintf("0x%v", v)
	}
	return json.Marshal(v)
}

type eip712AtomicResolver[V any] struct {
	ethType string
	hash    func(V) ([]byte, error)
}

func (r *eip712AtomicResolver[T]) Resolve(v any, _ string) (eipResolvedValue, error) {
	// If v is nil, use T's zero value instead
	if v == nil {
		var z T
		v = z
	}

	// JSON always decodes numbers as floats
	if u, ok := v.(float64); ok {
		var z T
		switch any(z).(type) {
		case int64:
			v = int64(u)
		case uint64:
			v = uint64(u)
		}
	}

	t, ok := v.(T)
	if !ok {
		return nil, fmt.Errorf("eip712 value of type %T does not match type field", v)
	}

	return &eipResolvedAtomic{r.ethType, v, func() ([]byte, error) { return r.hash(t) }}, nil
}

func newAtomicEncoder[T any](ethType string, hasher func(T) ([]byte, error)) EIP712Resolver {
	return &eip712AtomicResolver[T]{ethType, hasher}
}

func (f *TypeField) resolve(v any) (*resolvedFieldValue, error) {
	strippedType, slices := stripSlice(f.Type)
	encoder, ok := eip712EncoderMap[strippedType]
	if ok {
		if slices {
			// If v is nil, return an empty array
			if v == nil {
				return &resolvedFieldValue{*f, true, eipResolvedArray{}}, nil
			}
			vv, ok := v.([]interface{})
			if !ok {
				return nil, fmt.Errorf("eip712 field %s is not of an array of interfaces", f.Name)
			}
			var array eipResolvedArray
			for _, vvv := range vv {
				r, err := encoder.Resolve(vvv, strippedType)
				if err != nil {
					return nil, err
				}
				array = append(array, r)
			}
			return &resolvedFieldValue{*f, false, array}, nil
		}
		r, err := encoder.Resolve(v, f.Type)
		if err != nil {
			return nil, err
		}
		return &resolvedFieldValue{*f, v == nil, r}, nil
	}

	//if we get here, we are expecting a struct
	fields, ok := schemaDictionary[strippedType]
	if !ok {
		return nil, fmt.Errorf("eip712 field %s", f.Type)
	}

	// If v is nil and the type is a struct, skip this value
	if v == nil {
		return nil, nil
	}

	//from here on down we are expecting a struct
	if slices {
		//we expect a slice
		vv, ok := v.([]interface{})
		if !ok {
			return nil, fmt.Errorf("eip712 field %s is not of an array of interfaces", f.Name)
		}
		var array eipResolvedArray
		for _, vvv := range vv {
			//now run the hasher for the type
			// look for encoder, if we don't have one, call the types encoder
			fields, ok := schemaDictionary[strippedType]
			if !ok {
				return nil, fmt.Errorf("eip712 field %s", f.Type)
			}
			r, err := fields.Resolve(vvv, strippedType)
			if err != nil {
				return nil, err
			}
			array = append(array, r)
		}
		return &resolvedFieldValue{*f, false, array}, nil
	}

	r, err := fields.Resolve(v, f.Type)
	if err != nil {
		return nil, err
	}
	return &resolvedFieldValue{*f, false, r}, nil
}

func NewTypeField(n string, tp string) *TypeField {
	return &TypeField{n, tp}
}

// stripSlice removes all array indicators from the input string
// and returns the cleaned string along with the count of stripped indicators.
func stripSlice(input string) (string, bool) {
	s := strings.TrimSuffix(input, "[]")
	return s, len(s) < len(input)
}

func (td *TypeDefinition) sort() {
	//all types need to be sorted, so just make sure they are...
	sort.Slice(*td.Fields, func(i, j int) bool {
		return (*td.Fields)[i].Name < (*td.Fields)[j].Name
	})
}

func RegisterTypeDefinition(tf *[]*TypeField, name string, aliases ...string) {
	td := &TypeDefinition{name, tf}
	td.sort()

	schemaDictionary[name] = td
	for _, alias := range aliases {
		schemaDictionary[alias] = td
	}
}

// Construct the wallet RPC call
type EIP712Call struct {
	Types       map[string][]*TypeField `json:"types"`
	PrimaryType string                  `json:"primaryType"`
	Domain      EIP712Domain            `json:"domain"`
	Message     eipResolvedValue        `json:"message"`
}

func NewEIP712Call(value any, typ EIP712Resolver) (*EIP712Call, error) {
	r, err := typ.Resolve(value, "")
	if err != nil {
		return nil, err
	}

	e := EIP712Call{}
	e.PrimaryType = "Transaction"
	e.Domain = Eip712Domain
	e.Message = r

	e.Types = map[string][]*TypeField{}
	r.Types(e.Types)
	EIP712DomainValue.Types(e.Types)

	return &e, nil
}

func (c *EIP712Call) Hash() ([]byte, error) {
	messageHash, err := c.Message.Hash(c.Types)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteByte(0x19)
	buf.WriteByte(0x01)
	buf.Write(EIP712DomainHash)
	buf.Write(messageHash)
	return keccak256(buf.Bytes()), nil
}

func FromstringToBytes(s string) ([]byte, error) {
	return keccak256([]byte(s)), nil
}

func Fromuint256ToBytes(s string) ([]byte, error) {
	n := big.NewInt(0)
	n.SetString(s, 10)
	ret := make([]byte, 32)
	b := n.Bytes()
	copy(ret[32-len(b):], b)
	return ret, nil
}

func Fromint64ToBytes(n int64) ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	padded := make([]byte, 32)
	copy(padded[32-8:], b)
	return padded, nil
}

func Fromuint64ToBytes(n uint64) ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	padded := make([]byte, 32)
	copy(padded[32-8:], b)
	return padded, nil
}

func FromaddressToBytes(address string) ([]byte, error) {
	address = address[2:] // remove '0x' prefix
	b, err := hex.DecodeString(address)
	if err != nil {
		return nil, fmt.Errorf("invalid eip712 hex encoding for address, %v", err)
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded, nil
}

func Frombytes32ToBytes(value string) ([]byte, error) {
	b, err := hexStringToBytes(value)
	if err != nil {
		return nil, fmt.Errorf("invalid eip712 hex encoding for bytes32, %v", err)
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded, nil
}

func FromboolToBytes(b bool) ([]byte, error) {
	ret := make([]byte, 32)
	if b {
		ret[31] = 1
	}
	return ret, nil
}

func FrombytesToBytes(value string) ([]byte, error) {
	b, err := hexStringToBytes(value)
	if err != nil {
		return nil, fmt.Errorf("invalid eip712 hex encoding for bytes, %v", err)
	}
	return keccak256(b), nil
}

// FromfloatToBytes eip712 doesn't officially support floats, but we need to provide this for the code generator
func FromfloatToBytes(value float64) ([]byte, error) {
	return FromstringToBytes(fmt.Sprintf("%f", value))
}

func hexStringToBytes(hexStr string) ([]byte, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

func (v *EIP712Domain) MarshalJSON() ([]byte, error) {
	u := struct {
		Name    string  `json:"name,omitempty"`
		Version string  `json:"version,omitempty"`
		ChainId *string `json:"chainId,omitempty"`
	}{}
	if !(len(v.Name) == 0) {
		u.Name = v.Name
	}
	if !(len(v.Version) == 0) {
		u.Version = v.Version
	}
	if !(v.ChainId == nil) {
		u.ChainId = BigintToJSON(v.ChainId)
	}
	return json.Marshal(&u)
}

func (v *EIP712Domain) UnmarshalJSON(data []byte) error {
	u := struct {
		Name    string  `json:"name,omitempty"`
		Version string  `json:"version,omitempty"`
		ChainId *string `json:"chainId,omitempty"`
	}{}
	u.Name = v.Name
	u.Version = v.Version
	u.ChainId = BigintToJSON(v.ChainId)
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.Name = u.Name
	v.Version = u.Version
	if u.ChainId != nil {
		if x, err := BigintFromJSON(u.ChainId); err != nil {
			return fmt.Errorf("error decoding ChainId: %w", err)
		} else {
			v.ChainId = x
		}
	}
	return nil
}
