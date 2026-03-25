// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sigcache

import (
	"crypto/ed25519"
)

// VerifyED25519 verifies an ED25519 signature with optional caching.
// If cache is nil, performs direct verification without caching.
// Returns true if the signature is valid.
func VerifyED25519(cache *Cache, data, signature, publicKey []byte) bool {
	if cache == nil || !cache.config.Enabled {
		return ed25519.Verify(publicKey, data, signature)
	}

	// Try cache first
	if valid, found := cache.Check(data, signature, publicKey); found {
		return valid
	}

	// Perform actual verification
	valid := ed25519.Verify(publicKey, data, signature)

	// Store result
	cache.Store(data, signature, publicKey, valid)

	return valid
}
