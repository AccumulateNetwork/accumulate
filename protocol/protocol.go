// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Well known strings
const (
	// TLD is the top-level domain for ADIs.
	TLD = ".acme"

	// ACME is the name of the ACME token.
	ACME = "ACME"

	// Unknown is used to indicate that the principal of a transaction is unknown
	Unknown = "unknown"

	// Directory is the partition ID of the DN.
	Directory = "Directory"

	// Operators is the path to a node's operator key book.
	Operators = "operators"

	// Ledger is the path to a node's internal ledger.
	Ledger = "ledger"

	// Synthetic is the path to a node's synthetic transaction ledger.
	Synthetic = "synthetic"

	// AnchorPool is the path to a node's anchor chain account.
	AnchorPool = "anchors"

	// Votes is the path to the scratch data account for partition voting records
	Votes = "votes"

	// Evidence is the path to the scratch data account for partition voting records
	Evidence = "evidence"

	// Oracle is the path to a node's anchor chain account.
	Oracle = "oracle"

	// Network is the path to a node's network definition data account.
	Network = "network"

	// Routing is the path to a node's routing table data account.
	Routing = "routing"

	// Globals is the path to the Directory network's Mutable Protocol costants data account
	Globals = "globals"

	// MainChain is the main transaction chain of a record.
	MainChain = "main"

	// GenesisBlock is the block index of the first block.
	GenesisBlock = 1

	// ScratchPrunePeriodDays is the period after which data chain transactions are pruned
	ScratchPrunePeriodDays = 14

	// DefaultMajorBlockSchedule is the default cron schedule of when new major blocks are created
	DefaultMajorBlockSchedule = "0 */12 * * *"

	//AccountUrlMaxLength is the maximum size allowed for accumulate adi urls
	AccountUrlMaxLength = 500
)

// AcmeSupplyLimit set at 500,000,000.00000000 million acme (external units)
const AcmeSupplyLimit = 500_000_000

// DelegationDepthLimit limits the number of layers of delegation.
const DelegationDepthLimit = 20

// AcmeUrl returns `acc://ACME`.
func AcmeUrl() *url.URL {
	return &url.URL{Authority: ACME}
}

// UnknownUrl returns `acc://unknown`.
func UnknownUrl() *url.URL {
	return &url.URL{Authority: Unknown}
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
const AcmeOraclePrecision = 1e4
const AcmeOraclePrecisionPower = 4

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
// `acc://<chain-id>`.
//
// The rules for generating the authority of a lite data chain are
// the same as the address for a Lite Token Account
func LiteDataAddress(chainId []byte) (*url.URL, error) {
	if len(chainId) < 32 {
		return nil, errors.New("chainId for LiteDataAddress must be 32 bytes in length")
	}
	keyStr := hex.EncodeToString(chainId[:32])
	return &url.URL{Authority: keyStr}, nil
}

// ParseLiteAddress parses the hostname as a hex string and verifies its
// checksum.
func ParseLiteAddress(u *url.URL) ([]byte, error) {
	b, err := hex.DecodeString(u.Hostname())
	if err != nil {
		return nil, err
	}

	const checksumLen = 4
	if len(b) <= checksumLen {
		return nil, errors.New("too short")
	}

	i := len(b) - checksumLen
	byteValue, byteCheck := b[:i], b[i:]
	hexValue := u.Authority[:len(u.Authority)-checksumLen*2]
	checkSum := sha256.Sum256([]byte(hexValue))
	if !bytes.Equal(byteCheck, checkSum[len(checkSum)-checksumLen:]) {
		return nil, errors.New("invalid checksum")
	}

	return byteValue, nil
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
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("lite data address is not hex")
	}

	if len(b) != 32 {
		return nil, errors.New("lite data address is the wrong length")
	}

	return b, nil
}

// LiteTokenAddress returns an lite address for the given public key and token URL as
// `acc://<key-hash-and-checksum>/<token-url>`.
//
// Only the first 20 bytes of the public key hash is used. The checksum is the
// last four bytes of the hexadecimal partial key hash. For an ACME lite token
// account URL for a key with a public key hash of
//
//	"aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"
//
// The checksum is calculated as
//
//	sha256("aec070645fe53ee3b3763059376134f058cc3372")[28:] == "26e2a324"
//
// The resulting URL is
//
//	"acc://aec070645fe53ee3b3763059376134f058cc337226e2a324/ACME"
func LiteTokenAddress(pubKey []byte, tokenUrlStr string, signatureType SignatureType) (*url.URL, error) {
	tokenUrl, err := url.Parse(tokenUrlStr)
	if err != nil {
		return nil, err
	}

	if !AcmeUrl().Equal(tokenUrl) {
		if err := IsValidAdiUrl(tokenUrl.Identity(), false); err != nil {
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
		if err := IsValidAdiUrl(tokenUrl.Identity(), false); err != nil {
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

	liteUrl := LiteAuthorityForHash(pubKeyHash).
		WithPath(fmt.Sprintf("/%s%s", tokenUrl.Authority, tokenUrl.Path))

	return liteUrl, nil
}

func LiteAuthorityForHash(pubKeyHash []byte) *url.URL {
	keyStr := fmt.Sprintf("%x", pubKeyHash[:20])
	checkSum := sha256.Sum256([]byte(keyStr))
	checkStr := fmt.Sprintf("%x", checkSum[28:])
	return &url.URL{Authority: keyStr + checkStr}
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

	b, err := ParseLiteAddress(u)
	if err != nil {
		return nil, err
	}
	if b == nil || len(b) != 20 {
		// Hostname is not hex or is the wrong length, therefore the URL is not
		// lite
		return nil, nil
	}

	return b, nil
}

// ParseLiteTokenAddress extracts the key hash and token URL from an lite token
// account URL. Returns `nil, nil, nil` if the URL is not an lite token account
// URL. Returns an error if the checksum is invalid.
func ParseLiteTokenAddress(u *url.URL) ([]byte, *url.URL, error) {
	if u.Path == "" || len(u.Path) == 1 {
		// A URL with an empty or invalid path cannot be lite
		return nil, nil, nil
	}

	b, err := ParseLiteAddress(u)
	if err != nil {
		return nil, nil, err
	}
	if err != nil || len(b) != 20 {
		// Hostname is not hex or is the wrong length, therefore the URL is not lite
		return nil, nil, nil //nolint:nilerr
	}

	i := strings.IndexRune(u.Path[1:], '/')
	var v *url.URL
	if i >= 0 {
		i++
		v = &url.URL{Authority: u.Path[1:i], Path: u.Path[i:]}
	} else if u.Path[0] == '/' {
		v = &url.URL{Authority: u.Path[1:]}
	} else {
		v = &url.URL{Authority: u.Path}
	}
	return b[:20], v, nil
}

var reDigits10 = regexp.MustCompile("^[0-9]+$")

// var reDigits16 = regexp.MustCompile("^[0-9a-fA-F]+$")

// AccountUrl returns a URL with the given root identity and path. The .acme TLD
// is appended to the identity if it is not already present. AccountUrl does not
// check if the identity is valid.
func AccountUrl(rootIdentity string, path ...string) *url.URL {
	if !strings.HasSuffix(rootIdentity, TLD) {
		rootIdentity += TLD
	}
	return (&url.URL{Authority: rootIdentity}).JoinPath(path...)
}

// IsValidAdiUrl returns an error if the URL is not valid for an ADI.
//
// An ADI URL:
// 1) Must be valid UTF-8.
// 2) Authority must not include a port number.
// 3) Must have a (non-empty) hostname.
// 4) Hostname must not include dots (cannot be a domain).
// 5) Hostname must not be a number.
// 6) Hostname must not be 48 hexadecimal digits.
// 7) Must not have a path, query, or fragment.
// 8) Must not be a reserved URL, such as ACME, DN, or BVN-*
func IsValidAdiUrl(u *url.URL, allowReserved bool) error {
	var errs []string

	if !u.ValidUTF8() {
		errs = append(errs, "not valid UTF-8")
	}
	if u.Port() != "" {
		errs = append(errs, "identity has a port number")
	}
	a := u.Hostname()
	if a == "" {
		errs = append(errs, "identity is empty")
	} else if strings.HasSuffix(a, TLD) {
		a = a[:len(a)-len(TLD)]
		if a == "" {
			errs = append(errs, "identity is empty")
		}
	} else {
		errs = append(errs, "identity must end in "+TLD)
	}
	if strings.ContainsRune(a, '.') {
		errs = append(errs, "identity contains a subdomain")
	}
	if reDigits10.MatchString(a) {
		errs = append(errs, "identity is a number")
	}
	if u.Query != "" {
		errs = append(errs, "query is not empty")
	}
	if u.Fragment != "" {
		errs = append(errs, "fragment is not empty")
	}

	if !allowReserved && IsReserved(u) {
		errs = append(errs, fmt.Sprintf("%v is a reserved URL", u))
	}

	for _, r := range a {
		if unicode.In(r, unicode.Letter, unicode.Number) {
			continue
		}

		if len(errs) > 0 && (r == '.' || r == 65533) {
			// Do not report "invalid character '.'" in addition to "identity contains a subdomain"
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

func IsValidAccountPath(s string) error {
	var errs []string
	if !utf8.ValidString(s) {
		errs = append(errs, "not valid UTF-8")
	}

	var multiSlash bool
	seen := map[rune]bool{}
	for {
		i := 0
		for len(s) > 0 && s[0] == '/' {
			// Allow one slash at the beginning and one in between each
			if i > 0 {
				multiSlash = true
			}
			i++
			s = s[1:]
		}
		if len(s) == 0 {
			break
		}

		var part string
		i = strings.IndexByte(s, '/')
		if i < 0 {
			part, s = s, ""
		} else {
			part, s = s[:i], s[i:]
		}

		for _, r := range part {
			if unicode.In(r,
				// Allow letters and numbers
				unicode.Letter,
				unicode.Number,
				// Allow marks
				unicode.Mark,
				// Allow dash and connector punctuation
				unicode.Pd,
				unicode.Pc,
			) {
				continue
			}

			if len(errs) > 0 && r == 65533 {
				// Do not report "invalid character '�'" in addition to "not valid UTF-8"
				continue
			}

			switch r {
			case ' ':
				// Allow plain spaces
			case '.', ':', '+', '~', '|':
				// Allow some other symbols
			default:
				if seen[r] {
					continue
				}
				seen[r] = true
				errs = append(errs, fmt.Sprintf("illegal character %q", r))
			}
		}
	}

	if multiSlash {
		errs = append(errs, "adjacent path separators")
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, ", "))
}

// IsUnknown checks if the authority is 'unknown' or 'unknown.acme'.
func IsUnknown(u *url.URL) bool {
	return strings.EqualFold(u.Authority, Unknown) ||
		strings.EqualFold(u.Authority, Unknown+TLD)
}

// IsReserved checks if the given URL is reserved.
func IsReserved(u *url.URL) bool {
	if IsUnknown(u) {
		return true
	}
	_, ok := ParsePartitionUrl(u)
	return ok || BelongsToDn(u)
}

const dnUrlBase = "dn"

// DnUrl returns `acc://dn.acme`.
func DnUrl() *url.URL {
	return &url.URL{Authority: dnUrlBase + TLD}
}

// IsDnUrl checks if the URL is the DN ADI URL.
func IsDnUrl(u *url.URL) bool {
	u = u.RootIdentity()
	return DnUrl().Equal(u)
}

// BelongsToDn checks if the give account belongs to the DN.
func BelongsToDn(u *url.URL) bool {
	return IsDnUrl(u) || u.RootIdentity().Equal(AcmeUrl())
}

const bvnUrlPrefix = "bvn-"

// PartitionUrl returns `acc://bvn-${partition}.acme` or `acc://dn.acme`.
func PartitionUrl(partition string) *url.URL {
	if strings.EqualFold(partition, Directory) {
		return DnUrl()
	}
	return &url.URL{Authority: bvnUrlPrefix + partition + TLD}
}

// IsBvnUrl checks if the URL is the BVN ADI URL.
func IsBvnUrl(u *url.URL) bool {
	return strings.HasPrefix(u.Hostname(), bvnUrlPrefix)
}

// ParsePartitionUrl extracts the BVN partition name from a BVN URL, if the URL is a
// valid BVN ADI URL.
func ParsePartitionUrl(u *url.URL) (string, bool) {
	if IsDnUrl(u) {
		return Directory, true
	}
	a := u.Authority
	if !strings.HasPrefix(a, bvnUrlPrefix) {
		return "", false
	}
	if !strings.HasSuffix(a, TLD) {
		return "", false
	}
	return a[len(bvnUrlPrefix) : len(a)-len(TLD)], true
}

// BvnNameFromPartitionId formats a BVN partition name from the configuration to a valid URL hostname.
func BvnNameFromPartitionId(partition string) string {
	return PartitionUrl(strings.ToLower(partition)).Authority
}

func GetMOfN(count int, ratio float64) uint64 {
	return uint64(math.Ceil(ratio * float64(count)))
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

	return keyPage.Identity(), index, true
}
