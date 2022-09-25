package url

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

// URL is an Accumulate URL.
type URL struct {
	UserInfo  string
	Authority string
	Path      string
	Query     string
	Fragment  string

	memoize urlMemoize
}

type urlMemoize struct {
	str        string
	hash       [32]byte
	accountID  [32]byte
	identityID [32]byte
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

// StripExtras returns a URL with everything except the authority and path
// stripped. The receiver is returned if it meets the criteria.
func (u *URL) StripExtras() *URL {
	if u.UserInfo == "" && u.Query == "" && u.Fragment == "" {
		return u
	}

	v := new(URL)
	v.Authority = u.Authority
	v.Path = u.Path
	return v
}

// WithUserInfo creates a copy of the URL with UserInfo set to the given value.
func (u *URL) WithUserInfo(s string) *URL {
	v := u.copy()
	v.UserInfo = s
	return v
}

// WithAuthority creates a copy of the URL with Authority set to the given value.
func (u *URL) WithAuthority(s string) *URL {
	v := u.copy()
	v.Authority = s
	return v
}

// WithPath creates a copy of the URL with Path set to the given value.
func (u *URL) WithPath(s string) *URL {
	v := u.copy()
	v.Path = s
	return v
}

// WithQuery creates a copy of the URL with Query set to the given value.
func (u *URL) WithQuery(s string) *URL {
	v := u.copy()
	v.Query = s
	return v
}

// WithFragment creates a copy of the URL with Fragment set to the given value.
func (u *URL) WithFragment(s string) *URL {
	v := u.copy()
	v.Fragment = s
	return v
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

// copy returns a copy of the url.
func (u *URL) copy() *URL {
	v := *u
	v.memoize = urlMemoize{}
	return &v
}

func (u *URL) format(txid []byte, encode bool) string {
	var buf strings.Builder

	buf.WriteString("acc://")
	if txid != nil {
		enc := make([]byte, hex.EncodedLen(len(txid)))
		hex.Encode(enc, txid)
		buf.Write(enc)
		buf.WriteByte('@')
	} else if u.UserInfo != "" {
		buf.WriteString(u.UserInfo)
		buf.WriteByte('@')
	}

	buf.WriteString(u.Authority)

	p := normalizePath(u.Path)
	for len(p) > 0 {
		buf.WriteByte('/')
		i := strings.IndexByte(p[1:], '/') + 1
		if i <= 0 {
			if encode {
				buf.WriteString(url.PathEscape(p[1:]))
			} else {
				buf.WriteString(p[1:])
			}
			break
		}

		if encode {
			buf.WriteString(url.PathEscape(p[1:i]))
		} else {
			buf.WriteString(p[1:i])
		}
		p = p[i:]
	}

	if u.Query != "" {
		buf.WriteByte('?')
		buf.WriteString(u.Query)
	}

	if u.Fragment != "" {
		buf.WriteByte('#')
		buf.WriteString(u.Fragment)
	}

	return buf.String()
}

// String reassembles the URL into a valid URL string. See net/url.URL.String().
func (u *URL) String() string {
	if u.memoize.str != "" {
		return u.memoize.str
	}

	u.memoize.str = u.format(nil, true)
	return u.memoize.str
}

// RawString reassembles the URL into a valid URL string without encoding any
// component.
func (u *URL) RawString() string {
	return u.format(nil, false)
}

// ShortString returns String without the scheme prefix.
func (u *URL) ShortString() string {
	return u.String()[6:]
}

// ValidUTF8 verifies that all URL components are valid UTF-8 strings.
func (u *URL) ValidUTF8() bool {
	return utf8.ValidString(u.UserInfo) &&
		utf8.ValidString(u.Authority) &&
		utf8.ValidString(u.Path) &&
		utf8.ValidString(u.Query) &&
		utf8.ValidString(u.Fragment)
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

func normalizePath(s string) string {
	s = strings.Trim(s, "/")
	if s == "" {
		return ""
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
//	ID = Hash(LowerCase(u.Host()))
func (u *URL) IdentityAccountID() []byte {
	c := u.IdentityAccountID32()
	return c[:]
}

// IdentityAccountID32 returns IdentityAccountID as a [32]byte.
func (u *URL) IdentityAccountID32() [32]byte {
	if u.memoize.identityID == [32]byte{} {
		u.memoize.identityID = id(u.Hostname())
	}
	return u.memoize.identityID
}

// AccountID constructs an account identifier from the lower case hostname and
// path. The port is not included. If the path does not begin with `/`, `/` is
// added between the hostname and the path.
//
//	ID = Hash(LowerCase(Sprintf("%s/%s", u.Host(), u.Path)))
func (u *URL) AccountID() []byte {
	c := u.AccountID32()
	return c[:]
}

// AccountID32 returns AccountID as a [32]byte.
func (u *URL) AccountID32() [32]byte {
	if u.memoize.accountID == [32]byte{} {
		u.memoize.accountID = id(u.Hostname() + normalizePath(u.Path))
	}
	return u.memoize.accountID
}

// Routing returns the first 8 bytes of the identity account ID as an integer.
//
//	Routing = uint64(u.IdentityAccountID()[:8])
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
	if u.memoize.hash != [32]byte{} {
		return u.memoize.hash
	}

	hash := u.AccountID32()
	if u.Query != "" {
		hash = concatId(hash, id(u.Query))
	}
	if u.Fragment != "" {
		hash = concatId(hash, id(u.Fragment))
	}
	u.memoize.hash = hash
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
	v := u.copy()
	if len(v.Path) == 0 {
		v.Path = "/"
	}
	v.Path = path.Join(append([]string{v.Path}, s...)...)
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
