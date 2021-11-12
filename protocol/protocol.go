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

const ACME = "ACME"

// AcmeUrl returns `acc://ACME`
func AcmeUrl() *url.URL {
	return &url.URL{Authority: ACME}
}

// AcmePrecision is the precision of ACME token amounts.
const AcmePrecision = 1e8

// CreditsPerDollar is the conversion rate from 1 unit of fiat currency to
// credits. We call this 'dollars' because it's easier to write, and we are most
// likely going to use USD idefinitely.
const CreditsPerDollar = 1e2

// AnonymousAddress returns an anonymous address for the given public key and
// token URL as `acc://<key-hash-and-checksum>/<token-url>`.
//
// Only the first 20 bytes of the public key hash is used. The checksum is the
// last four bytes of the hexadecimal partial key hash. For an ACME anonymous
// token account URL for a key with a public key hash of
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
func AnonymousAddress(pubKey []byte, tokenUrlStr string) (*url.URL, error) {
	tokenUrl, err := url.Parse(tokenUrlStr)
	if err != nil {
		return nil, err
	}
	authority := url.URL{Authority: tokenUrl.Authority}

	if err := IsValidAdiUrl(&authority); err != nil {
		return nil, errors.New("invalid adi in token URL")
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

	anonUrl := new(url.URL)
	keyHash := sha256.Sum256(pubKey)
	keyStr := fmt.Sprintf("%x", keyHash[:20])
	checkSum := sha256.Sum256([]byte(keyStr))
	checkStr := fmt.Sprintf("%x", checkSum[28:])
	anonUrl.Authority = keyStr + checkStr
	anonUrl.Path = fmt.Sprintf("/%s%s", tokenUrl.Authority, tokenUrl.Path)
	return anonUrl, nil
}

// ParseAnonymousAddress extracts the key hash and token URL from an anonymous
// token account URL. Returns `nil, nil, nil` if the URL is not an anonymous
// token account URL. Returns an error if the checksum is invalid.
func ParseAnonymousAddress(u *url.URL) ([]byte, *url.URL, error) {
	if u.Path == "" || u.Path[0] != '/' || len(u.Path) == 1 {
		// A URL with an empty or invalid path cannot be anonymous
		return nil, nil, nil
	}

	b, err := hex.DecodeString(u.Hostname())
	if err != nil || len(b) != 24 {
		// Hostname is not hex or is the wrong length, therefore the URL is not anonymous
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
