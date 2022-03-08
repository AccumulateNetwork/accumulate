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

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// Well known strings
const (
	// ACME is the name of the ACME token.
	ACME = "ACME"

	// Directory is the subnet ID of the DN.
	Directory = "Directory"

	// ValidatorBook is the path to a node's validator key book.
	ValidatorBook = "validators"

	// Ledger is the path to a node's internal ledger.
	Ledger = "ledger"

	// AnchorPool is the path to a node's anchor chain account.
	AnchorPool = "anchors"

	// Oracle is the path to a node's anchor chain account.
	Oracle = "oracle"

	// MainChain is the main transaction chain of a record.
	MainChain = "main"

	// PendingChain is the pending signature chain of a record.
	PendingChain = "pending"

	// DataChain is the data chain of a record.
	DataChain = "data"

	// MajorRootChain is the major anchor root chain of a subnet.
	MajorRootChain = "major-root"

	// MinorRootChain is the minor anchor root chain of a subnet.
	MinorRootChain = "minor-root"

	// MajorRootIndexChain is the index chain of the major anchor root chain of a subnet.
	MajorRootIndexChain = "major-root-index"

	// MinorRootIndexChain is the index chain of the minor anchor root chain of a subnet.
	MinorRootIndexChain = "minor-root-index"

	// SyntheticChain is the synthetic transaction chain of a subnet.
	SyntheticChain = "synthetic"

	// DefaultKeyBook is the default key book name when not specified
	DefaultKeyBook = "book0"

	// DefaultKeyPage is the default key page name when not specified
	DefaultKeyPage = "page0"
)

// AcmeUrl returns `acc://ACME`.
func AcmeUrl() *url.URL {
	return &url.URL{Authority: ACME}
}

// PriceOracle returns acc://dn/oracle
func PriceOracle() *url.URL {
	return DnUrl().JoinPath(Oracle)
}

var PriceOracleAuthority = PriceOracle().String()

// AcmePrecision is the precision of ACME token amounts.
const AcmePrecision = 1e8

// AcmeOraclePrecision is the precision of the oracle in 100 * USD of one ACME token.
const AcmeOraclePrecision = 10000

// CreditPrecision is the precision of credit balances.
const CreditPrecision = 1e2

// CreditsPerFiatUnit is the conversion rate from credit balances to fiat
// currency. We expect to use USD indefinitely as the fiat currency.
//
// 100 credits converts to 1 dollar, but we charge down to 0.01 credits. So the
// actual conversion rate of the credit balance field to dollars is 10,000 to
// 1.
const CreditsPerFiatUnit = 1e2 * CreditPrecision

// LiteDataAddress returns a lite address for the given chain id as
// `acc://<chain-id-hash-and-checksum>`.
//
// The rules for generating the authority of a lite data chain are
// the same as the address for a Lite Token Account
func LiteDataAddress(chainId []byte) (*url.URL, error) {

	chainStr := fmt.Sprintf("%x", chainId[:20])

	liteUrl := new(url.URL)
	checkSum := sha256.Sum256([]byte(chainStr))
	checkStr := fmt.Sprintf("%x", checkSum[28:])
	liteUrl.Authority = chainStr + checkStr
	return liteUrl, nil
}

// ParseLiteDataAddress extracts the partial chain id from a lite chain URL.
// Returns `nil, err if the URL does not appear to be a lite token chain
// URL. Returns an error if the checksum is invalid.
func ParseLiteDataAddress(u *url.URL) ([]byte, error) {
	if u.Path != "" {
		// A chain URL can have no path
		return nil, errors.New("invalid chain url")
	}

	b, err := hex.DecodeString(u.Hostname())
	if err != nil || len(b) != 24 {
		return nil, errors.New("hostname is not hex or is wrong length")
	}

	v := *u
	v.Authority = u.Authority[:40]
	checkSum := sha256.Sum256([]byte(v.Authority))
	if !bytes.Equal(b[20:], checkSum[28:]) {
		return nil, errors.New("invalid checksum")
	}

	return b[:20], nil
}

// LiteTokenAddress returns an lite address for the given public key and token URL as
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
func LiteTokenAddress(pubKey []byte, tokenUrlStr string) (*url.URL, error) {
	tokenUrl, err := url.Parse(tokenUrlStr)
	if err != nil {
		return nil, err
	}

	if !AcmeUrl().Equal(tokenUrl) {
		if err := IsValidAdiUrl(tokenUrl.Identity()); err != nil {
			return nil, errors.New("invalid adi in token URL")
		}
		if tokenUrl.Path == "" {
			return nil, errors.New("must have a path in token URL")
		}
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

// ParseLiteTokenAddress extracts the key hash and token URL from an lite token
// account URL. Returns `nil, nil, nil` if the URL is not an lite token account
// URL. Returns an error if the checksum is invalid.
func ParseLiteTokenAddress(u *url.URL) ([]byte, *url.URL, error) {
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
		i++
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
		errs = append(errs, "identity could be a lite token account key")
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
	return ok || BelongsToDn(u)
}

// DnUrl returns `acc://dn`.
func DnUrl() *url.URL {
	return &url.URL{Authority: "dn"}
}

// BvnUrl returns `acc://bvn-${subnet}`.
func BvnUrl(subnet string) *url.URL {
	return &url.URL{Authority: "bvn-" + subnet}
}

// IsDnUrl checks if the URL is the DN ADI URL.
func IsDnUrl(u *url.URL) bool {
	u = u.RootIdentity()
	return DnUrl().Equal(u)
}

// ParseBvnUrl extracts the BVN subnet name from a BVN URL, if the URL is a
// valid BVN ADI URL.
func ParseBvnUrl(u *url.URL) (string, bool) {
	if !strings.HasPrefix(u.Authority, "bvn-") {
		return "", false
	}
	return u.Authority[4:], true
}

// BelongsToDn checks if the give account belongs to the DN.
func BelongsToDn(u *url.URL) bool {
	return IsDnUrl(u) || u.RootIdentity().Equal(AcmeUrl())
}

// BvnNameFromSubnetId formats a BVN subnet name from the configuration to a valid URL hostname.
func BvnNameFromSubnetId(subnetId string) string {
	return "bvn-" + strings.ToLower(subnetId)
}

// IndexChain returns the major or minor index chain name for a given chain. Do
// not use for the root anchor chain.
func IndexChain(name string, major bool) string {
	if major {
		return "major-" + name + "-index"
	}
	return "minor-" + name + "-index"
}
