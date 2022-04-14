package typegen

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MarshalAsBasic marshals the field as a basic type.
const MarshalAsBasic MarshalAs = 0

// MarshalAsNone omits the field from marshalling.
const MarshalAsNone MarshalAs = 1

// MarshalAsEnum marshals the field as an enumeration.
const MarshalAsEnum MarshalAs = 2

// MarshalAsValue marshals the field as a value type.
const MarshalAsValue MarshalAs = 3

// MarshalAsReference marshals the field as a reference type.
const MarshalAsReference MarshalAs = 4

// MarshalAsUnion marshals the field as a union type.
const MarshalAsUnion MarshalAs = 5

// TypeCodeUnknown .
const TypeCodeUnknown TypeCode = 0

// TypeCodeInt .
const TypeCodeInt TypeCode = 1

// TypeCodeUint .
const TypeCodeUint TypeCode = 2

// TypeCodeBool .
const TypeCodeBool TypeCode = 3

// TypeCodeString .
const TypeCodeString TypeCode = 4

// TypeCodeHash .
const TypeCodeHash TypeCode = 5

// TypeCodeBytes .
const TypeCodeBytes TypeCode = 6

// TypeCodeUrl .
const TypeCodeUrl TypeCode = 7

// TypeCodeTime .
const TypeCodeTime TypeCode = 8

// TypeCodeDuration .
const TypeCodeDuration TypeCode = 9

// TypeCodeBigInt .
const TypeCodeBigInt TypeCode = 10

// TypeCodeAny .
const TypeCodeAny TypeCode = 11

// TypeCodeRawJson .
const TypeCodeRawJson TypeCode = 12

// GetEnumValue returns the value of the Marshal As
func (v MarshalAs) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *MarshalAs) SetEnumValue(id uint64) bool {
	u := MarshalAs(id)
	switch u {
	case MarshalAsBasic, MarshalAsNone, MarshalAsEnum, MarshalAsValue, MarshalAsReference, MarshalAsUnion:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Marshal As
func (v MarshalAs) String() string {
	switch v {
	case MarshalAsBasic:
		return "basic"
	case MarshalAsNone:
		return "none"
	case MarshalAsEnum:
		return "enum"
	case MarshalAsValue:
		return "value"
	case MarshalAsReference:
		return "reference"
	case MarshalAsUnion:
		return "union"
	default:
		return fmt.Sprintf("MarshalAs:%d", v)
	}
}

// MarshalAsByName returns the named Marshal As.
func MarshalAsByName(name string) (MarshalAs, bool) {
	switch strings.ToLower(name) {
	case "basic":
		return MarshalAsBasic, true
	case "none":
		return MarshalAsNone, true
	case "enum":
		return MarshalAsEnum, true
	case "value":
		return MarshalAsValue, true
	case "reference":
		return MarshalAsReference, true
	case "union":
		return MarshalAsUnion, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Marshal As to JSON as a string.
func (v MarshalAs) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Marshal As from JSON as a string.
func (v *MarshalAs) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = MarshalAsByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Marshal As %q", s)
	}
	return nil
}

// GetEnumValue returns the value of the Type Code
func (v TypeCode) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *TypeCode) SetEnumValue(id uint64) bool {
	u := TypeCode(id)
	switch u {
	case TypeCodeUnknown, TypeCodeInt, TypeCodeUint, TypeCodeBool, TypeCodeString, TypeCodeHash, TypeCodeBytes, TypeCodeUrl, TypeCodeTime, TypeCodeDuration, TypeCodeBigInt, TypeCodeAny, TypeCodeRawJson:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Type Code
func (v TypeCode) String() string {
	switch v {
	case TypeCodeUnknown:
		return "unknown"
	case TypeCodeInt:
		return "int"
	case TypeCodeUint:
		return "uint"
	case TypeCodeBool:
		return "bool"
	case TypeCodeString:
		return "string"
	case TypeCodeHash:
		return "hash"
	case TypeCodeBytes:
		return "bytes"
	case TypeCodeUrl:
		return "url"
	case TypeCodeTime:
		return "time"
	case TypeCodeDuration:
		return "duration"
	case TypeCodeBigInt:
		return "bigInt"
	case TypeCodeAny:
		return "any"
	case TypeCodeRawJson:
		return "rawJson"
	default:
		return fmt.Sprintf("TypeCode:%d", v)
	}
}

// TypeCodeByName returns the named Type Code.
func TypeCodeByName(name string) (TypeCode, bool) {
	switch name {
	case "unknown":
		return TypeCodeUnknown, true
	case "int":
		return TypeCodeInt, true
	case "varint":
		return TypeCodeInt, true
	case "uint":
		return TypeCodeUint, true
	case "uvarint":
		return TypeCodeUint, true
	case "bool":
		return TypeCodeBool, true
	case "boolean":
		return TypeCodeBool, true
	case "string":
		return TypeCodeString, true
	case "hash":
		return TypeCodeHash, true
	case "chain":
		return TypeCodeHash, true
	case "bytes":
		return TypeCodeBytes, true
	case "url":
		return TypeCodeUrl, true
	case "time":
		return TypeCodeTime, true
	case "duration":
		return TypeCodeDuration, true
	case "bigInt":
		return TypeCodeBigInt, true
	case "bigint":
		return TypeCodeBigInt, true
	case "any":
		return TypeCodeAny, true
	case "rawJson":
		return TypeCodeRawJson, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Type Code to JSON as a string.
func (v TypeCode) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Type Code from JSON as a string.
func (v *TypeCode) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = TypeCodeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Type Code %q", s)
	}
	return nil
}
