// Package testconfig provides centralized configuration for AIP-50 example scripts.
// Change the version number here to create fresh ADIs for new test runs.
package testconfig

import (
	"crypto/ed25519"
	"encoding/hex"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// =============================================================================
// VERSION - Increment this to use fresh ADIs on re-run
// =============================================================================
const Version = "1"

// =============================================================================
// Network Configuration
// =============================================================================
const ACC_API = "http://127.0.0.1:26660/v3"

// =============================================================================
// ADI Names - Derived from Version
// =============================================================================
var (
	AliceADIName   = "alice-adi-" + Version + ".acme"
	BobADIName     = "bob-adi-" + Version + ".acme"
	ServiceADIName = "fee-service-" + Version + ".acme"
	UserName       = "user" + Version // For namespace user accounts (e.g., user3)
)

// =============================================================================
// Fixed Keys - For reproducible testing
// =============================================================================
const (
	// Alice's fixed private key (64 bytes hex = 32 byte seed + 32 byte public key)
	AlicePrivateKeyHex = "0e60416a6f0cfb491192e2bd8a3a876a95bb114ea81b5f3a5e57ac72d035de5bf30838a484fcfede8b3803c87f3b6753d315e47060283fff970d62bce900bb2c"

	// Service's fixed seed (32 bytes hex) - used to derive consistent keypair
	ServiceSeedHex = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
)

// =============================================================================
// Helper Functions
// =============================================================================

// GetAliceKey returns Alice's ED25519 private key
func GetAliceKey() ed25519.PrivateKey {
	keyBytes, _ := hex.DecodeString(AlicePrivateKeyHex)
	return ed25519.PrivateKey(keyBytes)
}

// GetAlicePubKey returns Alice's ED25519 public key
func GetAlicePubKey() ed25519.PublicKey {
	return GetAliceKey().Public().(ed25519.PublicKey)
}

// GetAliceLiteAddress returns Alice's lite token address
func GetAliceLiteAddress() *url.URL {
	addr, _ := protocol.LiteTokenAddress(GetAlicePubKey(), "ACME", protocol.SignatureTypeED25519)
	return addr
}

// GetServiceKey returns the service's ED25519 private key (derived from seed)
func GetServiceKey() ed25519.PrivateKey {
	seedBytes, _ := hex.DecodeString(ServiceSeedHex)
	return ed25519.NewKeyFromSeed(seedBytes)
}

// GetServicePubKey returns the service's ED25519 public key
func GetServicePubKey() ed25519.PublicKey {
	return GetServiceKey().Public().(ed25519.PublicKey)
}

// =============================================================================
// URL Helpers
// =============================================================================

// AliceADI returns the Alice ADI URL
func AliceADI() *url.URL {
	return protocol.AccountUrl(AliceADIName)
}

// AliceKeyBook returns Alice's key book URL
func AliceKeyBook() *url.URL {
	return AliceADI().JoinPath("book")
}

// AliceKeyPage returns Alice's key page URL (page 1)
func AliceKeyPage() *url.URL {
	return protocol.FormatKeyPageUrl(AliceKeyBook(), 0)
}

// AliceTokens returns Alice's token account URL
func AliceTokens() *url.URL {
	return AliceADI().JoinPath("tokens")
}

// BobADI returns the Bob ADI URL
func BobADI() *url.URL {
	return protocol.AccountUrl(BobADIName)
}

// BobKeyBook returns Bob's key book URL
func BobKeyBook() *url.URL {
	return BobADI().JoinPath("book")
}

// BobKeyPage returns Bob's key page URL (page 1)
func BobKeyPage() *url.URL {
	return protocol.FormatKeyPageUrl(BobKeyBook(), 0)
}

// BobTokens returns Bob's token account URL
func BobTokens() *url.URL {
	return BobADI().JoinPath("tokens")
}

// ServiceADI returns the service ADI URL
func ServiceADI() *url.URL {
	return protocol.AccountUrl(ServiceADIName)
}

// ServiceKeyBook returns the service's key book URL
func ServiceKeyBook() *url.URL {
	return ServiceADI().JoinPath("book")
}

// ServiceKeyPage returns the service's key page URL (page 1)
func ServiceKeyPage() *url.URL {
	return protocol.FormatKeyPageUrl(ServiceKeyBook(), 0)
}

// ServiceFees returns the service's fee account URL
func ServiceFees() *url.URL {
	return ServiceADI().JoinPath("fees")
}

// UserKeyBook returns the namespace user's key book URL (under service namespace)
func UserKeyBook() *url.URL {
	return ServiceADI().JoinPath(UserName + "-book")
}

// UserKeyPage returns the namespace user's key page URL
func UserKeyPage() *url.URL {
	return protocol.FormatKeyPageUrl(UserKeyBook(), 0)
}

// UserTokens returns the namespace user's token account URL (under service namespace)
func UserTokens() *url.URL {
	return ServiceADI().JoinPath(UserName + "-tokens")
}
