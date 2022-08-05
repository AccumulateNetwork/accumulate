package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdurl "net/url"
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

func (a *AddressBookEntries) Set(e *AddressBookEntry) {
	ptr, _ := sortutil.BinaryInsert(&a.Entries, func(f *AddressBookEntry) int { return bytes.Compare(f.PublicKeyHash[:], e.PublicKeyHash[:]) })
	*ptr = e
}

func (a *AddressBookEntries) Remove(keyHash []byte) {
	i, found := sortutil.Search(a.Entries, func(f *AddressBookEntry) int { return bytes.Compare(f.PublicKeyHash[:], keyHash) })
	if found {
		a.Entries = append(a.Entries[:i], a.Entries[i+1:]...)
	}
}

type InternetAddress struct {
	stdurl.URL
	port int
}

// MustParseInternetAddress calls ParseInternetAddress and panics if it returns
// an error.
func MustParseInternetAddress(s string) *InternetAddress {
	a, err := ParseInternetAddress(s)
	if err != nil {
		panic(err)
	}
	return a
}

// ParseInternetAddress parses s as a URL. ParseInternetAddress will return an
// error if the port is not numeric or missing.
func ParseInternetAddress(s string) (*InternetAddress, error) {
	u, err := stdurl.Parse(s)
	if err != nil {
		return nil, err
	}
	p, err := strconv.ParseInt(u.Port(), 10, 64)
	if err != nil {
		return nil, err
	}
	return &InternetAddress{*u, int(p)}, nil
}

func NewInternetAddress(scheme, host string, port int) *InternetAddress {
	v := new(InternetAddress)
	v.port = port
	v.Scheme = scheme
	v.Host = fmt.Sprintf("%s:%d", host, port)
	return v
}

// func (u *InternetAddress) Port() int { return u.port }

// func (u *InternetAddress) WithScheme(scheme string) *InternetAddress {
// 	v := new(InternetAddress)
// 	v.port = u.port
// 	v.URL = u.URL
// 	v.URL.Scheme = scheme
// 	return v
// }

// func (u *InternetAddress) WithHostname(hostname string) *InternetAddress {
// 	v := new(InternetAddress)
// 	v.port = u.port
// 	v.URL = u.URL
// 	v.URL.Host = fmt.Sprintf("%s:%d", hostname, v.port)
// 	return v
// }

// func (u *InternetAddress) WithOffset(offset int) *InternetAddress {
// 	v := new(InternetAddress)
// 	v.port = u.port + offset
// 	v.URL = u.URL
// 	v.URL.Host = fmt.Sprintf("%s:%d", u.Hostname(), v.port)
// 	return v
// }

// Copy implements the type generator's value interface. It does not copy. URLs
// should be immutable so there's no reason to copy.
func (u *InternetAddress) Copy() *InternetAddress {
	return u
}

func (u *InternetAddress) Equal(v *InternetAddress) bool {
	switch {
	case u == v:
		return true
	case u == nil || v == nil:
		return false
	case u.URL == v.URL:
		return true
	}
	return u.String() == v.String()
}

// MarshalJSON marshals the URL to JSON as a string.
func (u *InternetAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON unmarshals the URL from JSON as a string.
func (u *InternetAddress) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	v, err := ParseInternetAddress(s)
	if err != nil {
		return err
	}

	*u = *v
	return nil
}

// MarshalBinary marshals the URL to Binary as a string.
func (u *InternetAddress) MarshalBinary() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalBinary unmarshals the URL from Binary as a string.
func (u *InternetAddress) UnmarshalBinary(data []byte) error {
	v, err := ParseInternetAddress(string(data))
	if err != nil {
		return err
	}

	*u = *v
	return nil
}
