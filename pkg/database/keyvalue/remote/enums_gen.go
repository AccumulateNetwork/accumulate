// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// callTypeGet .
const callTypeGet callType = 1

// callTypePut .
const callTypePut callType = 2

// callTypeDelete .
const callTypeDelete callType = 3

// callTypeForEach .
const callTypeForEach callType = 4

// callTypeCommit .
const callTypeCommit callType = 5

// callTypeBatch .
const callTypeBatch callType = 6

// responseTypeOk .
const responseTypeOk responseType = 1

// responseTypeError .
const responseTypeError responseType = 2

// responseTypeNotFound .
const responseTypeNotFound responseType = 3

// responseTypeValue .
const responseTypeValue responseType = 4

// responseTypeEntry .
const responseTypeEntry responseType = 5

// responseTypeBatch .
const responseTypeBatch responseType = 6

// responseTypeUnsupportedCall .
const responseTypeUnsupportedCall responseType = 7

// GetEnumValue returns the value of the call Type
func (v callType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *callType) SetEnumValue(id uint64) bool {
	u := callType(id)
	switch u {
	case callTypeGet, callTypePut, callTypeDelete, callTypeForEach, callTypeCommit, callTypeBatch:
		*v = u
		return true
	}
	return false
}

// String returns the name of the call Type.
func (v callType) String() string {
	switch v {
	case callTypeGet:
		return "get"
	case callTypePut:
		return "put"
	case callTypeDelete:
		return "delete"
	case callTypeForEach:
		return "forEach"
	case callTypeCommit:
		return "commit"
	case callTypeBatch:
		return "batch"
	}
	return fmt.Sprintf("callType:%d", v)
}

// callTypeByName returns the named call Type.
func callTypeByName(name string) (callType, bool) {
	switch strings.ToLower(name) {
	case "get":
		return callTypeGet, true
	case "put":
		return callTypePut, true
	case "delete":
		return callTypeDelete, true
	case "foreach":
		return callTypeForEach, true
	case "commit":
		return callTypeCommit, true
	case "batch":
		return callTypeBatch, true
	}
	return 0, false
}

// MarshalJSON marshals the call Type to JSON as a string.
func (v callType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the call Type from JSON as a string.
func (v *callType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = callTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid call Type %q", s)
	}
	return nil
}

// GetEnumValue returns the value of the response Type
func (v responseType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *responseType) SetEnumValue(id uint64) bool {
	u := responseType(id)
	switch u {
	case responseTypeOk, responseTypeError, responseTypeNotFound, responseTypeValue, responseTypeEntry, responseTypeBatch, responseTypeUnsupportedCall:
		*v = u
		return true
	}
	return false
}

// String returns the name of the response Type.
func (v responseType) String() string {
	switch v {
	case responseTypeOk:
		return "ok"
	case responseTypeError:
		return "error"
	case responseTypeNotFound:
		return "notFound"
	case responseTypeValue:
		return "value"
	case responseTypeEntry:
		return "entry"
	case responseTypeBatch:
		return "batch"
	case responseTypeUnsupportedCall:
		return "unsupportedCall"
	}
	return fmt.Sprintf("responseType:%d", v)
}

// responseTypeByName returns the named response Type.
func responseTypeByName(name string) (responseType, bool) {
	switch strings.ToLower(name) {
	case "ok":
		return responseTypeOk, true
	case "error":
		return responseTypeError, true
	case "notfound":
		return responseTypeNotFound, true
	case "value":
		return responseTypeValue, true
	case "entry":
		return responseTypeEntry, true
	case "batch":
		return responseTypeBatch, true
	case "unsupportedcall":
		return responseTypeUnsupportedCall, true
	}
	return 0, false
}

// MarshalJSON marshals the response Type to JSON as a string.
func (v responseType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the response Type from JSON as a string.
func (v *responseType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = responseTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid response Type %q", s)
	}
	return nil
}
