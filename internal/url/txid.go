package url

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/url"
)

// TxID is a transaction identifier.
type TxID struct {
	url  *URL
	hash [32]byte
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

	u.UserInfo = ""
	return &TxID{u, *(*[32]byte)(hash)}, nil
}

// WithTxID constructs a transaction ID.
func (u *URL) WithTxID(hash [32]byte) *TxID {
	return &TxID{u, hash}
}

// AsUrl returns a URL with the transaction ID as the user info.
func (x *TxID) AsUrl() *URL {
	u := x.url.Copy()
	u.UserInfo = hex.EncodeToString(x.hash[:])
	return u
}

// Account returns the account URL.
func (x *TxID) Account() *URL { return x.url }

// Hash returns the transaction hash.
func (x *TxID) Hash() [32]byte { return x.hash }

// Copy returns a copy of the transaction ID.
func (x *TxID) Copy() *TxID {
	y := *x
	y.url = x.url.Copy()
	return &y
}

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
	u := x.url.URL()
	u.User = url.User(hex.EncodeToString(x.hash[:]))
	return u.String()
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

	v, err := ParseTxID(s)
	if err != nil {
		return err
	}

	*x = *v
	return nil
}
