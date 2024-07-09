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
	"sort"
	"strings"

	"go.uber.org/zap/buffer"
	"golang.org/x/crypto/sha3"
)

type TypeField struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	encoder Eip712Encoder
}

type TypeDefinition struct {
	Fields *[]*TypeField `json:"types"`
}

type EIP712Domain struct {
	Name    string   `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Version string   `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	ChainId *big.Int `json:"chainId,omitempty" form:"chainId" query:"chainId" validate:"required"`
}

var Eip712Domain = EIP712Domain{Name: "Accumulate", Version: "1.0.0", ChainId: big.NewInt(281)}
var EIP712DomainMap map[string]interface{}
var EIP712DomainHash []byte

type Eip712Encoder struct {
	hasher func(v interface{}) ([]byte, error)
	types  func(ret map[string]*TypeDefinition, v interface{}, fieldType string) error
}

var eip712EncoderMap map[string]Eip712Encoder

func init() {
	eip712EncoderMap = make(map[string]Eip712Encoder)
	eip712EncoderMap["bool"] = NewEncoder(FromboolToBytes, nil)
	eip712EncoderMap["bytes"] = NewEncoder(FrombytesToBytes, nil)
	eip712EncoderMap["bytes32"] = NewEncoder(Frombytes32ToBytes, nil)
	eip712EncoderMap["int64"] = NewEncoder(Fromint64ToBytes, nil)
	eip712EncoderMap["uint64"] = NewEncoder(Fromuint64ToBytes, nil)
	eip712EncoderMap["string"] = NewEncoder(FromstringToBytes, nil)
	eip712EncoderMap["address"] = NewEncoder(FromaddressToBytes, nil)
	eip712EncoderMap["uint256"] = NewEncoder(Fromuint256ToBytes, nil)
	eip712EncoderMap["float64"] = NewEncoder(FromfloatToBytes, nil)
	eip712EncoderMap["float"] = NewEncoder(FromfloatToBytes, nil) //Note = Float is not a valid type in EIP-712, so it is converted to a string

	// Handle EIP712 domain initialization
	j, err := Eip712Domain.MarshalJSON()
	if err != nil {
		//should never get here
		panic(err)
	}
	err = json.Unmarshal(j, &EIP712DomainMap)
	if err != nil {
		//should never get here
		panic(err)
	}

	RegisterTypeDefinition(&[]*TypeField{
		NewTypeField("name", "string"),
		NewTypeField("version", "string"),
		NewTypeField("chainId", "uint256"),
	}, "EIP712Domain")

	td := SchemaDictionary["EIP712Domain"]
	EIP712DomainHash, err = td.hash(EIP712DomainMap, "EIP712Domain")
	if err != nil {
		//shouldn't fail, but if it does, catch it
		panic(err)
	}
}

type Func[T any, R any] func(T) (R, error)
type Enum interface {
	SetEnumValue(id uint64) bool
	String() string
}

func mapEnumTypes[T any, R any](f Func[T, R]) (string, map[string]string) {
	//need to store allocator
	ret := make(map[string]string)
	enumType := new(T)
	enumValue, ok := any(enumType).(Enum)
	if !ok {
		return "unknown", nil
	}
	var a *R
	opName := reflect.TypeOf(a).Elem().String()
	//strip package name if present
	idx := strings.LastIndex(opName, ".")
	if idx != -1 {
		opName = opName[idx+1:]
	}
	//build a map of types
	for maxType := uint64(0); ; maxType++ {
		if !enumValue.SetEnumValue(maxType) {
			break
		}
		op, err := f(*enumType)
		if err != nil {
			//fmt.Println("Error calling function with enum:", err)
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
		ret[key] = name
	}
	return opName, ret
}

func RegisterEnumeratedTypeInterface[T any, R any](op Func[T, R]) {
	tp, typesMap := mapEnumTypes(op)
	eip712EncoderMap[tp] = NewEncoder(func(v interface{}) ([]byte, error) {
		return FromTypedInterfaceToBytes(v, typesMap)
	}, func(ret map[string]*TypeDefinition, v interface{}, typeField string) error {
		return FromTypedInterfaceToTypes(ret, v, typesMap)
	})
}

func FromTypedInterfaceToBytes(v interface{}, typesAliasMap map[string]string) ([]byte, error) {
	//this is a complex structure, so upcast it to an interface map
	vv, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", v)
	}

	t, ok := vv["type"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	a, ok := typesAliasMap[t]
	if !ok {
		return nil, fmt.Errorf("type alias does not exist in map: %T, %v", v, t)
	}

	d, ok := SchemaDictionary[a]
	if !ok {
		return nil, fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	b, err := d.hash(vv, t) //encodeData(vv, t, d)
	if err != nil {
		return nil, err
	}
	return keccak256(b), nil
}

func FromTypedInterfaceToTypes(ret map[string]*TypeDefinition, v interface{}, typesAliasMap map[string]string) error {
	//this is a complex structure, so upcast it to an interface map
	vv, ok := v.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid data entry type: %T", v)
	}

	t, ok := vv["type"].(string)
	if !ok {
		return fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	a, ok := typesAliasMap[t]
	if !ok {
		return fmt.Errorf("type alias does not exist in map: %T, %v", v, t)
	}

	d, ok := SchemaDictionary[a]
	if !ok {
		return fmt.Errorf("invalid data entry type: %T", vv["type"])
	}

	err := d.types(ret, v, t)
	if err != nil {
		return err
	}
	return nil
}

type TypedData struct {
	TypeName string
	Structs  []*TypedData
	Types    []TypeField
}

func (td *TypeDefinition) types(ret map[string]*TypeDefinition, d interface{}, typeName string) error {
	var err error
	//define the type structure
	if ret == nil {
		ret = make(map[string]*TypeDefinition)
	}

	data := d.(map[string]interface{})

	//now loop through the fields and either encode the value or recursively dive into more types

	//the stripping shouldn't be necessary, but do it as a precaution
	strippedType, _ := stripSlice(typeName)
	tdr := &TypeDefinition{}
	ret[strippedType] = tdr

	for i, field := range *td.Fields {
		value, ok := data[field.Name]
		if !ok {
			continue
		}

		//append the fields
		*tdr.Fields = append(*tdr.Fields, (*td.Fields)[i])

		//breakdown field further if required
		err = field.types(ret, value, field.Type)
		if err != nil {
			return err
		}
	}

	return nil
}

func (td *TypeDefinition) hash(v interface{}, typeName string) ([]byte, error) {
	data, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot hash type definition with invalid interface %T", v)
	}

	//define the type structure
	var header bytes.Buffer
	var body bytes.Buffer

	//now loop through the fields and either encode the value or recursively dive into more types
	first := true
	//the stripping shouldn't be necessary, but do it as a precaution
	strippedType, _ := stripSlice(typeName)
	header.WriteString(strippedType + "(")
	for _, field := range *td.Fields {
		value, ok := data[field.Name]
		if !ok {
			continue
		}

		//now run the hasher
		encodedValue, err := field.encoder.hasher(value)
		if err != nil {
			return nil, err
		}
		if !first {
			header.WriteString(",")
		}
		header.WriteString(field.Type + " " + field.Name)
		body.Write(encodedValue)
		first = false
	}
	header.WriteString(")")
	return keccak256(append(keccak256(header.Bytes()), body.Bytes()...)), nil
}

func (t *TypeField) types(ret map[string]*TypeDefinition, v interface{}, fieldType string) error {
	if t.encoder.types != nil {
		//process more complex type
		return t.encoder.types(ret, v, fieldType)
	}
	return nil
}

func NewEncoder[T any](hasher func(T) ([]byte, error), types func(ret map[string]*TypeDefinition, v interface{}, typeField string) error) Eip712Encoder {
	return Eip712Encoder{func(v interface{}) ([]byte, error) {
		t, ok := v.(T)
		if !ok {
			return nil, fmt.Errorf("eip712 value of type %T does not match type field", v)
		}
		return hasher(t)
	}, types,
	}
}

func NewTypeField(n string, tp string) *TypeField {
	return &TypeField{n, tp,
		Eip712Encoder{func(v interface{}) ([]byte, error) {
			strippedType, slices := stripSlice(tp)
			encoder, ok := eip712EncoderMap[strippedType]
			if ok {
				if slices > 0 {
					vv, ok := v.([]interface{})
					if !ok {
						return nil, fmt.Errorf("eip712 field %s is not of an array of interfaces", n)
					}
					var buff buffer.Buffer
					for _, vvv := range vv {
						b, err := encoder.hasher(vvv)
						if err != nil {
							return nil, err
						}
						_, err = buff.Write(b)
						if err != nil {
							return nil, err
						}
					}
					return keccak256(buff.Bytes()), nil
				}
				return encoder.hasher(v)
			}

			//from here on down we are expecting a struct
			if slices > 0 {
				//we expect a slice
				vv, ok := v.([]interface{})
				if !ok {
					return nil, fmt.Errorf("eip712 field %s is not of an array of interfaces", n)
				}
				var buff buffer.Buffer
				//iterate through the interfaces
				for _, vvv := range vv {
					//now run the hasher for the type
					// look for encoder, if we don't have one, call the types encoder
					fields, ok := SchemaDictionary[strippedType]
					if !ok {
						return nil, fmt.Errorf("eip712 field %s", tp)
					}
					b, err := fields.hash(vvv, tp)
					if err != nil {
						return nil, err
					}

					_, err = buff.Write(b)
					if err != nil {
						return nil, err
					}
				}
				return keccak256(buff.Bytes()), nil
			}

			//if we get here, we are expecting a struct
			fields, ok := SchemaDictionary[strippedType]
			if !ok {
				return nil, fmt.Errorf("eip712 field %s", tp)
			}

			b, err := fields.hash(v, tp) //encodeData(m, tp, fields)
			if err != nil {
				return nil, err
			}
			return b, nil
		}, func(ret map[string]*TypeDefinition, v interface{}, fieldType string) error {
			return nil
		},
		},
	}
}

// stripSlice removes all array indicators from the input string
// and returns the cleaned string along with the count of stripped indicators.
func stripSlice(input string) (string, int) {
	count := 0
	indicator := "[]"

	for strings.Contains(input, indicator) {
		input = strings.Replace(input, indicator, "", 1)
		count++
	}
	return input, count
}

var SchemaDictionary map[string]*TypeDefinition

var resolvers map[string]func() error

func RegisterTypeDefinitionResolver(name string, deferFunc func() error) {
	if resolvers == nil {
		resolvers = make(map[string]func() error)
	}
	resolvers[name] = deferFunc
}

func (td *TypeDefinition) sort() {
	//all types need to be sorted, so just make sure they are...
	sort.Slice(*td.Fields, func(i, j int) bool {
		return (*td.Fields)[i].Name < (*td.Fields)[j].Name
	})
}

func RegisterTypeDefinition(tf *[]*TypeField, aliases ...string) {
	td := &TypeDefinition{tf}
	td.sort()

	if SchemaDictionary == nil {
		SchemaDictionary = make(map[string]*TypeDefinition)
	}

	for _, alias := range aliases {
		SchemaDictionary[alias] = td
	}
}

func Eip712Hash(v map[string]interface{}, typeName string, td *TypeDefinition) ([]byte, error) {
	messageHash, err := td.hash(v, typeName)
	if err != nil {
		return nil, err
	}
	return keccak256(append(EIP712DomainHash, messageHash...)), nil
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
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}
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
