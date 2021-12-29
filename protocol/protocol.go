package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AccumulateNetwork/accumulate/internal/url"
)

const (
	// ACME is the name of the ACME token.
	ACME = "ACME"

	// Directory is the subnet ID of the DN.
	Directory = "Directory"

	// MinorRoot is the URL path of the subnet's minor root chain.
	MinorRoot = "minor-root"

	// MajorRoot is the URL path of the subnet's major root chain.
	MajorRoot = "major-root"

	// Synthetic is the URL path of the subnet's synthetic transaction chain.
	Synthetic = "synthetic"

	// Main is the main transaction chain of a record.
	Main = "Main"

	// Pending is the pending signature chain of a record.
	Pending = "Pending"

	// Data is the data chain of a record.
	Data = "Data"
)

// AcmeUrl returns `acc://ACME`.
func AcmeUrl() *url.URL {
	return &url.URL{Authority: ACME}
}

// AcmePrecision is the precision of ACME token amounts.
const AcmePrecision = 1e8

// FiatUnitsPerAcmeToken fixes the conversion between ACME tokens and fiat
// currency to 1:1, as in $1 per 1 ACME token.
//
// As soon as we have an oracle, this must be removed.
const FiatUnitsPerAcmeToken = 1

// CreditPrecision is the precision of credit balances.
const CreditPrecision = 1e2

// CreditsPerFiatUnit is the conversion rate from credit balances to fiat
// currency. We expect to use USD indefinitely as the fiat currency.
//
// 100 credits converts to 1 dollar, but we charge down to 0.01 credits, so the
// actual conversion rate of the credit balance field to dollars is is 10,000 to
// 1.
const CreditsPerFiatUnit = 1e2 * CreditPrecision

// LiteAddress returns an lite address for the given public key and token URL as
// `acc://<key-hash-and-checksum>/<token-url>`.
//
// Only the first 20 bytes of the public key hash is used. The checksum is the
// last four bytes of the hexadecimal partial key hash. For an ACME lite token
// account URL for a key with a public key hash of
//
//   "aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"
//
// The checksum is calculated as
//
//   sha256("aec070645fe53ee3b3763059376134f058cc3372")[28:] == "26e2a324"
//
// The resulting URL is
//
//   "acc://aec070645fe53ee3b3763059376134f058cc337226e2a324/ACME"
func LiteAddress(pubKey []byte, tokenUrlStr string) (*url.URL, error) {
	tokenUrl, err := url.Parse(tokenUrlStr)
	if err != nil {
		return nil, err
	}

	if !AcmeUrl().Equal(tokenUrl) {
		if err := IsValidAdiUrl(tokenUrl.Identity()); err != nil {
			return nil, errors.New("invalid adi in token URL")
		}
	}
	if tokenUrl.Path == "" && !bytes.EqualFold([]byte(tokenUrl.Authority), []byte("acme")) {
		return nil, errors.New("must have a path in token URL")
	}
	if tokenUrl.UserInfo != "" {
		return nil, errors.New("token URLs cannot include user info")
	}
	if tokenUrl.Port() != "" {
		return nil, errors.New("token URLs cannot include a port number")
	}
	if tokenUrl.Query != "" {
		return nil, errors.New("token URLs cannot include a query")
	}
	if tokenUrl.Fragment != "" {
		return nil, errors.New("token URLs cannot include a fragment")
	}

	liteUrl := new(url.URL)
	keyHash := sha256.Sum256(pubKey)
	keyStr := fmt.Sprintf("%x", keyHash[:20])
	checkSum := sha256.Sum256([]byte(keyStr))
	checkStr := fmt.Sprintf("%x", checkSum[28:])
	liteUrl.Authority = keyStr + checkStr
	liteUrl.Path = fmt.Sprintf("/%s%s", tokenUrl.Authority, tokenUrl.Path)
	return liteUrl, nil
}

// ParseLiteAddress extracts the key hash and token URL from an lite token
// account URL. Returns `nil, nil, nil` if the URL is not an lite token account
// URL. Returns an error if the checksum is invalid.
func ParseLiteAddress(u *url.URL) ([]byte, *url.URL, error) {
	if u.Path == "" || u.Path[0] != '/' || len(u.Path) == 1 {
		// A URL with an empty or invalid path cannot be lite
		return nil, nil, nil
	}

	b, err := hex.DecodeString(u.Hostname())
	if err != nil || len(b) != 24 {
		// Hostname is not hex or is the wrong length, therefore the URL is not lite
		return nil, nil, nil
	}

	v := *u
	v.Authority = u.Authority[:40]
	checkSum := sha256.Sum256([]byte(v.Authority))
	if !bytes.Equal(b[20:], checkSum[28:]) {
		return nil, nil, errors.New("invalid checksum")
	}

	// Reuse V as the token URL
	i := strings.IndexRune(u.Path[1:], '/')
	if i < 0 {
		v.Authority = u.Path[1:]
		v.Path = ""
	} else {
		v.Authority = u.Path[1:i]
		v.Path = u.Path[i:]
	}
	return b[:20], &v, nil
}

var reDigits10 = regexp.MustCompile("^[0-9]+$")
var reDigits16 = regexp.MustCompile("^[0-9a-fA-F]+$")

// IsValidAdiUrl returns an error if the URL is not valid for an ADI.
//
// An ADI URL:
// 1) Must be valid UTF-8.
// 2) Authority must not include a port number.
// 3) Must have a (non-empty) hostname.
// 4) Hostname must not include dots (cannot be a domain).
// 5) Hostname must not be a number.
// 6) Hostname must not be 48 hexidecimal digits.
// 7) Must not have a path, query, or fragment.
// 8) Must not be a reserved URL, such as ACME, DN, or BVN-*
func IsValidAdiUrl(u *url.URL) error {
	var errs []string

	if !utf8.ValidString(u.RawString()) {
		errs = append(errs, "not valid UTF-8")
	}
	if u.Port() != "" {
		errs = append(errs, "identity has a port number")
	}
	if u.Authority == "" {
		errs = append(errs, "identity is empty")
	}
	if strings.ContainsRune(u.Authority, '.') {
		errs = append(errs, "identity contains dot(s)")
	}
	if reDigits10.MatchString(u.Authority) {
		errs = append(errs, "identity is a number")
	}
	if reDigits16.MatchString(u.Authority) && len(u.Authority) == 48 {
		errs = append(errs, "identity could be a lite account key")
	}
	if u.Path != "" {
		errs = append(errs, "path is not empty")
	}
	if u.Query != "" {
		errs = append(errs, "query is not empty")
	}
	if u.Fragment != "" {
		errs = append(errs, "fragment is not empty")
	}

	if IsReserved(u) {
		errs = append(errs, fmt.Sprintf("%q is a reserved URL", u))
	}

	for _, r := range u.Hostname() {
		if unicode.In(r, unicode.Letter, unicode.Number) {
			continue
		}

		if len(errs) > 0 && (r == '.' || r == 65533) {
			// Do not report "invalid character '.'" in addition to "identity contains dot(s)"
			// Do not report "invalid character '�'" in addition to "not valid UTF-8"
			continue
		}

		switch r {
		case '-':
			// OK
		default:
			errs = append(errs, fmt.Sprintf("illegal character %q", r))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, ", "))
}

// IsReserved checks if the given URL is reserved.
func IsReserved(u *url.URL) bool {
	_, ok := ParseBvnUrl(u)
	return ok || IsDnUrl(u)
}

// DnUrl returns `acc://dn`.
func DnUrl() *url.URL {
	return &url.URL{Authority: "dn"}
}

// BvnUrl returns `acc://bvn-${subnet}`.
func BvnUrl(subnet string) *url.URL {
	return &url.URL{Authority: "bvn-" + subnet}
}

// IsDnUrl checks if the URL belongs to the DN.
func IsDnUrl(u *url.URL) bool {
	u = u.Identity()
	return DnUrl().Equal(u) || AcmeUrl().Equal(u)
}

// ParseBvnUrl extracts the BVN subnet name from a BVN URL, if the URL is a
// valid BVN URL.
func ParseBvnUrl(u *url.URL) (string, bool) {
	if !strings.HasPrefix(u.Authority, "bvn-") {
		return "", false
	}
	return u.Authority[4:], true
}
