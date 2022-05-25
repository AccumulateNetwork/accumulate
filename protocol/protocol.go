package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
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

	// OperatorBook is the path to a node's operator key book.
	OperatorBook = "operators"

	// Ledger is the path to a node's internal ledger.
	Ledger = "ledger"

	// Synthetic is the path to a node's synthetic transaction ledger.
	Synthetic = "synthetic"

	// AnchorPool is the path to a node's anchor chain account.
	AnchorPool = "anchors"

	// Votes is the path to the scratch data account for subnet voting records
	Votes = "votes"

	// Evidence is the path to the scratch data account for subnet voting records
	Evidence = "evidence"

	// Oracle is the path to a node's anchor chain account.
	Oracle = "oracle"

	// Globals is the path to the Directory network's Mutable Protocol costants data account
	Globals = "globals"

	// MainChain is the main transaction chain of a record.
	MainChain = "main"

	// SignatureChain is the pending signature chain of a record.
	SignatureChain = "signature"

	// // MajorRootChain is the major anchor root chain of a subnet.
	// MajorRootChain = "major-root"

	// MinorRootChain is the minor anchor root chain of a subnet.
	MinorRootChain = "minor-root"

	// // MajorRootIndexChain is the index chain of the major anchor root chain of a subnet.
	// MajorRootIndexChain = "major-root-index"

	// MinorRootIndexChain is the index chain of the minor anchor root chain of a subnet.
	MinorRootIndexChain = "minor-root-index"

	// GenesisBlock is the block index of the first block.
	GenesisBlock = 1

	// FallbackValidatorThreshold is the factor of how many of the validator signatures are required respective of their total number
	FallbackValidatorThreshold = 2.0 / 3.0

	// ScratchPrunePeriodDays is the period after which data chain transactions are pruned
	ScratchPrunePeriodDays = 14
)

//AcmeSupplyLimit set at 500,000,000.00000000 million acme (external units)
const AcmeSupplyLimit = 500_000_000

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
const AcmePrecisionPower = 8

// AcmeOraclePrecision is the precision of the oracle in 100 * USD of one ACME token.
const AcmeOraclePrecision = 10000

// InitialAcmeOracle is the oracle value at launch.
const InitialAcmeOracle = 0.05

// InitialAcmeOracleValue exists because golanglint-ci is dumb.
const InitialAcmeOracleValue = InitialAcmeOracle * AcmeOraclePrecision

// CreditPrecision is the precision of credit balances.
const CreditPrecision = 1e2
const CreditPrecisionPower = 2

// CreditsPerDollar is the credits per dollar in external units (100.00)
const CreditsPerDollar = 1e2

// CreditUnitsPerFiatUnit is the conversion rate from credit balances to fiat
// currency. We expect to use USD indefinitely as the fiat currency.
//
// 100 credits converts to 1 dollar, but we charge down to 0.01 credits. So the
// actual conversion rate of the credit balance field to dollars is 10,000 to 1.
const CreditUnitsPerFiatUnit = CreditsPerDollar * CreditPrecision

// LiteDataAddress returns a lite address for the given chain id as
// `acc://<chain-id-hash-and-checksum>`.
//
// The rules for generating the authority of a lite data chain are
// the same as the address for a Lite Token Account
func LiteDataAddress(chainId []byte) (*url.URL, error) {
	u := LiteAuthorityForHash(chainId)
	if u == nil {
		return nil, fmt.Errorf("cannot create lite authority")
	}
	return u, nil
}

// ParseLiteDataAddress extracts the partial chain id from a lite chain URL.
// Returns `nil, err` if the URL does not appear to be a lite token chain URL.
// Returns an error if the checksum is invalid.
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
func LiteTokenAddress(pubKey []byte, tokenUrlStr string, signatureType SignatureType) (*url.URL, error) {
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

	return LiteAuthorityForKey(pubKey, signatureType).JoinPath(tokenUrl.ShortString()), nil
}

func LiteTokenAddressFromHash(pubKeyHash []byte, tokenUrlStr string) (*url.URL, error) {
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

	liteUrl := LiteAuthorityForHash(pubKeyHash)
	liteUrl.Path = fmt.Sprintf("/%s%s", tokenUrl.Authority, tokenUrl.Path)

	return liteUrl, nil
}

func LiteAuthorityForHash(pubKeyHash []byte) *url.URL {
	liteUrl := new(url.URL)
	keyStr := fmt.Sprintf("%x", pubKeyHash[:20])
	checkSum := sha256.Sum256([]byte(keyStr))
	checkStr := fmt.Sprintf("%x", checkSum[28:])
	liteUrl.Authority = keyStr + checkStr
	return liteUrl
}

func LiteAuthorityForKey(pubKey []byte, signatureType SignatureType) *url.URL {
	var keyHash []byte
	switch signatureType {
	case SignatureTypeRCD1:
		keyHash = GetRCDHashFromPublicKey(pubKey, 1)
	case SignatureTypeBTC, SignatureTypeBTCLegacy:
		keyHash = BTCHash(pubKey)
	case SignatureTypeETH:
		keyHash = ETHhash(pubKey)
	case SignatureTypeED25519, SignatureTypeLegacyED25519:
		h := sha256.Sum256(pubKey)
		keyHash = h[:]
	default:
		h := sha256.Sum256(pubKey)
		keyHash = h[:]
	}
	return LiteAuthorityForHash(keyHash[:])
}

// ParseLiteIdentity extracts the key hash and token URL from a lite identity
// account URL. Returns `nil, nil` if the URL is not a lite identity URL.
// Returns an error if the checksum is invalid.
func ParseLiteIdentity(u *url.URL) ([]byte, error) {
	if u.Path != "" && u.Path != "/" {
		// A URL with a non-empty path cannot be a lite identity
		return nil, nil
	}

	authority := u.Authority
	b, err := hex.DecodeString(authority)
	if err != nil || len(b) != 24 {
		// Hostname is not hex or is the wrong length, therefore the URL is not
		// lite
		return nil, nil
	}

	authority = authority[:40]
	checkSum := sha256.Sum256([]byte(authority))
	if !bytes.Equal(b[20:], checkSum[28:]) {
		return nil, errors.New("invalid checksum")
	}

	return b[:20], nil
}

// ParseLiteTokenAddress extracts the key hash and token URL from an lite token
// account URL. Returns `nil, nil, nil` if the URL is not an lite token account
// URL. Returns an error if the checksum is invalid.
func ParseLiteTokenAddress(u *url.URL) ([]byte, *url.URL, error) {
	if u.Path == "" || len(u.Path) == 1 {
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
	if i >= 0 {
		i++
		v.Authority = u.Path[1:i]
		v.Path = u.Path[i:]
	} else if u.Path[0] == '/' {
		v.Authority = u.Path[1:]
		v.Path = ""
	} else {
		v.Authority = u.Path
		v.Path = ""
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
	_, ok := ParseSubnetUrl(u)
	return ok || BelongsToDn(u)
}

// DnUrl returns `acc://dn`.
func DnUrl() *url.URL {
	return &url.URL{Authority: "dn"}
}

// SubnetUrl returns `acc://bvn-${subnet}` or `acc://dn`.
func SubnetUrl(subnet string) *url.URL {
	if strings.EqualFold(subnet, Directory) {
		return DnUrl()
	}
	return &url.URL{Authority: "bvn-" + subnet}
}

// IsDnUrl checks if the URL is the DN ADI URL.
func IsDnUrl(u *url.URL) bool {
	u = u.RootIdentity()
	return DnUrl().Equal(u)
}

// IsBvnUrl checks if the URL is the BVN ADI URL.
func IsBvnUrl(u *url.URL) bool {
	return strings.HasPrefix(u.Hostname(), "bvn-")
}

// ParseSubnetUrl extracts the BVN subnet name from a BVN URL, if the URL is a
// valid BVN ADI URL.
func ParseSubnetUrl(u *url.URL) (string, bool) {
	if IsDnUrl(u) {
		return Directory, true
	}
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

func GetValidatorsMOfN(validatorCount int, ratio float64) uint64 {
	return uint64(math.Round(ratio * float64(validatorCount)))
}

const rootAnchorSuffix = "-root"
const bptAnchorSuffix = "-bpt"

// RootAnchorChain returns the name of the intermediate anchor chain for the given
// subnet's root chain.
func RootAnchorChain(name string) string {
	return name + rootAnchorSuffix
}

// BPTAnchorChain returns the name of the intermediate anchor chain for the given
// subnet's BPT.
func BPTAnchorChain(name string) string {
	return name + bptAnchorSuffix
}

// ParseBvnUrl extracts the subnet name from a intermediate anchor chain name.
func ParseAnchorChain(name string) (string, bool) {
	if !strings.HasSuffix(strings.ToLower(name), rootAnchorSuffix) {
		return "", false
	}
	return name[:len(name)-len(rootAnchorSuffix)], true
}

// SyntheticIndexChain returns the name of the synthetic transaction index chain
// for the given subnet.
func SyntheticIndexChain(name string) string {
	return "index-" + name
}

// FormatKeyPageUrl constructs the URL of a key page from the URL of its key
// book and the page index. For example, the URL for the first page of id/book
// is id/book/1.
func FormatKeyPageUrl(keyBook *url.URL, pageIndex uint64) *url.URL {
	return keyBook.JoinPath(strconv.FormatUint(pageIndex+1, 10))
}

// ParseKeyPageUrl parses a key page URL, returning the key book URL and the
// page number. If the URL does not represent a key page, the last return value
// is false. ParseKeyPageUrl is the inverse of FormatKeyPageUrl.
func ParseKeyPageUrl(keyPage *url.URL) (*url.URL, uint64, bool) {
	i := strings.LastIndexByte(keyPage.Path, '/')
	if i < 0 {
		return nil, 0, false
	}

	index, err := strconv.ParseUint(keyPage.Path[i+1:], 10, 16)
	if err != nil {
		return nil, 0, false
	}

	keyBook := *keyPage
	keyBook.Path = keyBook.Path[:i]
	return &keyBook, index, true
}
