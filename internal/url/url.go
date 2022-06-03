package url

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

// URL is an Accumulate URL.
type URL struct {
	UserInfo  string
	Authority string
	Path      string
	Query     string
	Fragment  string
}

type Values = url.Values

// Parse parses the string as an Accumulate URL. The scheme may be omitted, in
// which case `acc://` will be added, but if present it must be `acc`. The
// hostname must be non-empty. RawPath, ForceQuery, and RawFragment are not
// preserved.
func Parse(s string) (*URL, error) {
	u, err := url.Parse(s)
	if err == nil && u.Scheme == "" {
		u, err = url.Parse("acc://" + s)
	}
	if err != nil {
		return nil, err
	}

	if u.Scheme != "acc" {
		return nil, wrongScheme(s)
	}

	if u.Host == "" || u.Host[0] == ':' {
		return nil, missingHost(s)
	}

	v := new(URL)
	v.Authority = u.Host
	v.Path = u.Path
	v.Query = u.RawQuery
	v.Fragment = u.Fragment
	if u.User != nil {
		v.UserInfo = u.User.Username()
		if pw, _ := u.User.Password(); pw != "" {
			v.UserInfo += ":" + pw
		}
	}
	return v, nil
}

// MustParse calls Parse and panics if it returns an error.
func MustParse(s string) *URL {
	u, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func splitColon(s string) (string, string) {
	t := strings.SplitN(s, ":", 2)
	if len(t) == 1 {
		return t[0], ""
	}
	return t[0], t[1]
}

// URL returns a net/url.URL.
func (u *URL) URL() *url.URL {
	v := new(url.URL)
	v.Scheme = "acc"
	v.Host = u.Authority
	v.Path = u.Path
	v.RawQuery = u.Query
	v.Fragment = u.Fragment
	if u.UserInfo != "" {
		user, pass := splitColon(u.UserInfo)
		if pass != "" {
			v.User = url.UserPassword(user, pass)
		} else {
			v.User = url.User(user)
		}
	}
	return v
}

// Compare returns an integer comparing two URLs as lower case strings. The
// result will be 0 if u == v, -1 if u < v, and +1 if u > v.
func (u *URL) Compare(v *URL) int {
	uStr := strings.ToLower(u.String())
	vStr := strings.ToLower(v.String())
	switch {
	case uStr < vStr:
		return -1
	case uStr > vStr:
		return +1
	default:
		return 0
	}
}

// Copy returns a copy of the url.
func (u *URL) Copy() *URL {
	v := *u
	return &v
}

// String reassembles the URL into a valid URL string. See net/url.URL.String().
func (u *URL) String() string {
	return u.URL().String()
}

// ShortString returns String without the scheme prefix.
func (u *URL) ShortString() string {
	return u.String()[6:]
}

// RawString concatenates all of the URL parts. Does not percent-encode
// anything. Primarily used for validation.
func (u *URL) RawString() string {
	s := "acc://"
	if u.UserInfo != "" {
		s += u.UserInfo + "@"
	}
	s += u.Authority + u.Path
	if u.Query != "" {
		s += "?" + u.Query
	}
	if u.Fragment != "" {
		s += "#" + u.Fragment
	}
	return s
}

// Hostname returns the hostname from the authority component.
func (u *URL) Hostname() string {
	s, _ := splitColon(u.Authority)
	return s
}

// Port returns the port from the authority component.
func (u *URL) Port() string {
	_, s := splitColon(u.Authority)
	return s
}

// Username returns the username from the user info component.
func (u *URL) Username() string {
	s, _ := splitColon(u.UserInfo)
	return s
}

// Password returns the password from the user info component.
func (u *URL) Password() string {
	_, s := splitColon(u.UserInfo)
	return s
}

// QueryValues parses Query and returns the corresponding values. It silently
// discards malformed value pairs. To check errors use net/url.ParseQuery.
func (u *URL) QueryValues() Values {
	v, _ := url.ParseQuery(u.Query)
	return v
}

func id(s string) [32]byte {
	s = strings.ToLower(s)
	h := sha256.Sum256([]byte(s))
	return h
}

func concatId(ids ...[32]byte) [32]byte {
	digest := sha256.New()
	for _, id := range ids {
		digest.Write(id[:]) //nolint:rangevarref
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result
}

func ensurePath(s string) string {
	if s == "" || s[0] == '/' {
		return s
	}
	return "/" + s
}

// RootIdentity returns a copy of the URL with an empty path.
func (u *URL) RootIdentity() *URL {
	return &URL{Authority: u.Authority, UserInfo: u.UserInfo}
}

// Identity returns a copy of the URL with the last section cut off the path.
func (u *URL) Identity() *URL {
	v, _ := u.Parent()
	return v
}

// IsRootIdentity returns true if the URL is a root identity.
func (u *URL) IsRootIdentity() bool {
	return u.Path == "" || u.Path == "/"
}

// Parent gets the URL's parent path, or returns the original URL and false.
func (u *URL) Parent() (*URL, bool) {
	v := new(URL)
	v.UserInfo = u.UserInfo
	v.Authority = u.Authority
	v.Path = u.Path

	// Canonicalize the path
	v.Path = strings.TrimSuffix(v.Path, "/")
	if len(v.Path) == 0 {
		return v, false
	}
	slashIdx := strings.LastIndex(v.Path, "/")
	if slashIdx == -1 {
		v.Path = ""
	} else {
		v.Path = v.Path[:slashIdx]
	}
	return v, true
}

// ParentOf returns true if U is the parent of V
func (u *URL) ParentOf(v *URL) bool {
	v, ok := v.Parent()
	return ok && u.Equal(v)
}

// PrefixOf returns true if U is a prefix of V.
func (u *URL) PrefixOf(v *URL) bool {
	if !strings.EqualFold(u.Authority, v.Authority) {
		return false
	}
	if len(u.Path) > len(v.Path) {
		return false
	}
	for {
		var ok bool
		v, ok = v.Parent()
		if !ok {
			return false
		}

		if u.Equal(v) {
			return true
		}
	}
}

// LocalTo returns true if U is local to V, that is if they have the same root
// identity.
func (u *URL) LocalTo(v *URL) bool {
	return u.RootIdentity().Equal(v.RootIdentity())
}

// IdentityAccountID constructs an account identifier from the lower case
// hostname. The port is not included.
//
//   ID = Hash(LowerCase(u.Host()))
func (u *URL) IdentityAccountID() []byte {
	c := u.IdentityAccountID32()
	return c[:]
}

// IdentityAccountID32 returns IdentityAccountID as a [32]byte.
func (u *URL) IdentityAccountID32() [32]byte {
	return id(u.Hostname())
}

// AccountID constructs an account identifier from the lower case hostname and
// path. The port is not included. If the path does not begin with `/`, `/` is
// added between the hostname and the path.
//
//   ID = Hash(LowerCase(Sprintf("%s/%s", u.Host(), u.Path)))
func (u *URL) AccountID() []byte {
	c := u.AccountID32()
	return c[:]
}

// AccountID32 returns AccountID as a [32]byte.
func (u *URL) AccountID32() [32]byte {
	return id(u.Hostname() + ensurePath(u.Path))
}

// Routing returns the first 8 bytes of the identity account ID as an integer.
//
//   Routing = uint64(u.IdentityAccountID()[:8])
func (u *URL) Routing() uint64 {
	return binary.BigEndian.Uint64(u.IdentityAccountID())
}

// Hash calculates a hash of the URL starting with AccountID, hashing and
// concatenating Query unless it is empty, and hashing and concatenating
// Fragment unless it is empty. If Query and Fragment are both non-empty, with H
// defined as the SHA-256 hash of the lower case of the operand, the result is
// H(H(AccountID + H(Query)) + H(Fragment)).
func (u *URL) Hash() []byte {
	hash := u.Hash32()
	return hash[:]
}

// Hash32 returns Hash as a [32]byte.
func (u *URL) Hash32() [32]byte {
	hash := u.AccountID32()
	if u.Query != "" {
		hash = concatId(hash, id(u.Query))
	}
	if u.Fragment != "" {
		hash = concatId(hash, id(u.Fragment))
	}
	return hash
}

// Equal reports whether u and v, converted to strings and interpreted as UTF-8,
// are equal under Unicode case-folding.
func (u *URL) Equal(v *URL) bool {
	if u == v {
		return true
	}
	if u == nil || v == nil {
		return false
	}
	return strings.EqualFold(u.String(), v.String())
}

// PathEqual reports whether u.Path and v, normalized and interpreted as UTF-8,
// are equal under Unicode case-folding.
func (u *URL) PathEqual(v string) bool {
	up, v := strings.Trim(u.Path, "/"), strings.Trim(v, "/")
	return strings.EqualFold(up, v)
}

// JoinPath returns a copy of U with additional path elements.
func (u *URL) JoinPath(s ...string) *URL {
	if len(s) == 0 {
		return u
	}
	v := u.Copy()
	if len(v.Path) == 0 {
		v.Path = "/"
	}
	v.Path = path.Join(append([]string{v.Path}, s...)...)
	return v
}

// WithFragment returns a copy of U with the fragment set.
func (u *URL) WithFragment(s string) *URL {
	v := u.Copy()
	v.Fragment = s
	return v
}

// MarshalJSON marshals the URL to JSON as a string.
func (u *URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON unmarshals the URL from JSON as a string.
func (u *URL) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	v, err := Parse(s)
	if err != nil {
		return err
	}

	*u = *v
	return nil
}
