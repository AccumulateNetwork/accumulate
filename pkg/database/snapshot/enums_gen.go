// Copyright 2023 The Accumulate Authors
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

// SectionTypeHeader is the snapshot's header.
const SectionTypeHeader SectionType = 1

// SectionTypeAccountsV1 contains accounts (v1).
const SectionTypeAccountsV1 SectionType = 2

// SectionTypeTransactionsV1 contains transactions (v1).
const SectionTypeTransactionsV1 SectionType = 3

// SectionTypeSignaturesV1 contains signatures (v1).
const SectionTypeSignaturesV1 SectionType = 4

// SectionTypeGzTransactionsV1 contains gzipped transactions (v1).
const SectionTypeGzTransactionsV1 SectionType = 5

// SectionTypeSnapshot contains another snapshot.
const SectionTypeSnapshot SectionType = 6

// GetEnumValue returns the value of the Section Type
func (v SectionType) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *SectionType) SetEnumValue(id uint64) bool {
	u := SectionType(id)
	switch u {
	case SectionTypeHeader, SectionTypeAccountsV1, SectionTypeTransactionsV1, SectionTypeSignaturesV1, SectionTypeGzTransactionsV1, SectionTypeSnapshot:
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
	case SectionTypeAccountsV1:
		return "accountsV1"
	case SectionTypeTransactionsV1:
		return "transactionsV1"
	case SectionTypeSignaturesV1:
		return "signaturesV1"
	case SectionTypeGzTransactionsV1:
		return "gzTransactionsV1"
	case SectionTypeSnapshot:
		return "snapshot"
	}
	return fmt.Sprintf("SectionType:%d", v)
}

// SectionTypeByName returns the named Section Type.
func SectionTypeByName(name string) (SectionType, bool) {
	switch strings.ToLower(name) {
	case "header":
		return SectionTypeHeader, true
	case "accountsv1":
		return SectionTypeAccountsV1, true
	case "transactionsv1":
		return SectionTypeTransactionsV1, true
	case "signaturesv1":
		return SectionTypeSignaturesV1, true
	case "gztransactionsv1":
		return SectionTypeGzTransactionsV1, true
	case "snapshot":
		return SectionTypeSnapshot, true
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
