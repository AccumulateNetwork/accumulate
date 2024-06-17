// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package url

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"sync/atomic"

	binary2 "gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

// TxID is a transaction identifier.
type TxID struct {
	url  *URL
	hash [32]byte

	memoize struct {
		str atomic.Pointer[string]
	}
}

// ParseTxID parses the string as a URL and parses the URL's user info as a
// transaction hash.
func ParseTxID(s string) (*TxID, error) {
	u, err := Parse(s)
	if err != nil {
		return nil, err
	}

	return u.AsTxID()
}

// MustParseTxID calls ParseTxID and panics if it returns an error.
func MustParseTxID(s string) *TxID {
	x, err := ParseTxID(s)
	if err != nil {
		panic(err)
	}
	return x
}

// AsTxID parses the URL's user info as a transaction hash.
func (u *URL) AsTxID() (*TxID, error) {
	if u.UserInfo == "" {
		return nil, missingHash(u)
	}

	hash, err := hex.DecodeString(u.UserInfo)
	if err != nil {
		return nil, invalidHash(u, err)
	}

	if len(hash) != 32 {
		return nil, invalidHash(u, "wrong length")
	}

	u = u.WithUserInfo("")
	return &TxID{url: u, hash: *(*[32]byte)(hash)}, nil
}

// WithTxID constructs a transaction ID.
func (u *URL) WithTxID(hash [32]byte) *TxID {
	return &TxID{url: u, hash: hash}
}

// AsUrl returns a URL with the transaction ID as the user info.
func (x *TxID) AsUrl() *URL {
	u := x.url.copy()
	u.UserInfo = hex.EncodeToString(x.hash[:])
	return u
}

// Account returns the account URL.
func (x *TxID) Account() *URL { return x.url }

// Hash returns the transaction hash.
func (x *TxID) Hash() [32]byte { return x.hash }

// HashSlice returns the transaction hash as a slice.
func (x *TxID) HashSlice() []byte { return x.hash[:] }

// Equal checks if X and Y are the same transaction ID.
func (x *TxID) Equal(y *TxID) bool {
	return x.hash == y.hash &&
		x.url.Equal(y.url)
}

func (x *TxID) Compare(y *TxID) int {
	if x.hash != y.hash {
		return bytes.Compare(x.hash[:], y.hash[:])
	}
	return x.url.Compare(y.url)
}

// String reassembles the transaction ID into a valid URL string. See
// net/url.URL.String().
func (x *TxID) String() string {
	return getOrMakeAtomic(&x.memoize.str, func() string {
		return x.url.format(x.hash[:], true)
	})
}

// RawString reassembles the URL into a valid URL string without encoding any
// component.
func (x *TxID) RawString() string {
	return x.url.format(x.hash[:], false)
}

// ShortString returns String without the scheme prefix.
func (x *TxID) ShortString() string {
	return x.String()[6:]
}

// MarshalJSON marshals the transaction ID to JSON as a string.
func (x *TxID) MarshalJSON() ([]byte, error) {
	return json.Marshal(x.String())
}

// UnmarshalJSON unmarshals the transaction ID from JSON as a string.
func (x *TxID) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return x.unmarshal(s)
}

func (x *TxID) UnmarshalBinaryV2(dec *binary2.Decoder) error {
	s, err := dec.DecodeString()
	if err != nil {
		return err
	}
	return x.unmarshal(s)
}

func (x *TxID) unmarshal(s string) error {
	v, err := ParseTxID(s)
	if err != nil {
		return err
	}

	*x = *v //nolint:govet
	return nil
}
