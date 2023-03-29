// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SectionTypeHeader .
const SectionTypeHeader SectionType = 1

// SectionTypeAccounts .
const SectionTypeAccounts SectionType = 2

// SectionTypeTransactions .
const SectionTypeTransactions SectionType = 3

// SectionTypeSignatures .
const SectionTypeSignatures SectionType = 4

// SectionTypeGzTransactions .
const SectionTypeGzTransactions SectionType = 5

// GetEnumValue returns the value of the Section Type
func (v SectionType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *SectionType) SetEnumValue(id uint64) bool {
	u := SectionType(id)
	switch u {
	case SectionTypeHeader, SectionTypeAccounts, SectionTypeTransactions, SectionTypeSignatures, SectionTypeGzTransactions:
		*v = u
		return true
	}
	return false
}

// String returns the name of the Section Type.
func (v SectionType) String() string {
	switch v {
	case SectionTypeHeader:
		return "header"
	case SectionTypeAccounts:
		return "accounts"
	case SectionTypeTransactions:
		return "transactions"
	case SectionTypeSignatures:
		return "signatures"
	case SectionTypeGzTransactions:
		return "gzTransactions"
	}
	return fmt.Sprintf("SectionType:%d", v)
}

// SectionTypeByName returns the named Section Type.
func SectionTypeByName(name string) (SectionType, bool) {
	switch strings.ToLower(name) {
	case "header":
		return SectionTypeHeader, true
	case "accounts":
		return SectionTypeAccounts, true
	case "transactions":
		return SectionTypeTransactions, true
	case "signatures":
		return SectionTypeSignatures, true
	case "gztransactions":
		return SectionTypeGzTransactions, true
	}
	return 0, false
}

// MarshalJSON marshals the Section Type to JSON as a string.
func (v SectionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Section Type from JSON as a string.
func (v *SectionType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = SectionTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Section Type %q", s)
	}
	return nil
}
