package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulated/internal/url"
)

//go:generate go run ../internal/cmd/gentypes types.yml

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
// Only the first 20 bytes of the public key hash is used. The checksum is
// equivalent to last four bytes of the resource chain ID. For an ACME anonymous
// token account URL for a key with a public key hash of
//
//   "aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"
//
// The checksum is calculated as
//
//   sha256("aec070645fe53ee3b3763059376134f058cc3372/acme")[28:] == "3c63cf54"
//
// The resulting URL is
//
//   "acc://aec070645fe53ee3b3763059376134f058cc33723c63cf54/ACME"
func AnonymousAddress(pubKey []byte, tokenUrlStr string) (*url.URL, error) {
	tokenUrl, err := url.Parse(tokenUrlStr)
	if err != nil {
		return nil, err
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
	anonUrl.Authority = fmt.Sprintf("%x", keyHash[:20])
	anonUrl.Path = fmt.Sprintf("/%s%s", tokenUrl.Authority, tokenUrl.Path)

	checkSum := anonUrl.ResourceChain()[28:]
	anonUrl.Authority += fmt.Sprintf("%x", checkSum)
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
	if !bytes.Equal(b[20:], v.ResourceChain()[28:]) {
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
