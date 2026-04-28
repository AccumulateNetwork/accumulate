// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package trustbundle

import (
	"context"
	"errors"
	"sync"
)

// ErrNoBundle indicates no bundle is currently available for the
// requested partition / height. Distinct from a transport error so
// callers can decide whether to retry.
var ErrNoBundle = errors.New("trustbundle: no bundle available")

// Producer is the validator-side surface that supplies trust bundles
// to the GetTrustAnchor service method (issue #3983, parent #3953).
//
// Implementations are responsible for:
//
//  - Periodically capturing partition state at confirmed depth.
//  - Constructing and signing a Bundle.
//  - Caching the latest bundle and returning it on demand.
//
// The minimal implementation is the in-memory Cache below, which holds
// a single bundle that an external scheduler updates. A production
// validator wraps it with the capture loop. The service handler treats
// Producer as opaque so different deployment modes (validator,
// bundle-relay node, test fixture) plug in via the same interface.
type Producer interface {
	// CurrentBundle returns the current trust bundle for the named
	// partition. Returns ErrNoBundle if none is available yet.
	CurrentBundle(ctx context.Context, partition string) (*Bundle, error)
}

// Cache is a thread-safe in-memory Producer holding the latest bundle
// per partition. Validator code calls Set whenever a fresh bundle is
// produced; the service method reads via CurrentBundle.
type Cache struct {
	mu       sync.RWMutex
	byPart   map[string]*Bundle
}

// NewCache returns an empty Cache.
func NewCache() *Cache {
	return &Cache{byPart: make(map[string]*Bundle)}
}

// Set installs a bundle as the latest for its partition. Overwrites
// any prior bundle for the same partition.
func (c *Cache) Set(b *Bundle) {
	if b == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byPart[b.Partition] = b
}

// CurrentBundle returns the latest bundle for the given partition, or
// ErrNoBundle if none has been published.
func (c *Cache) CurrentBundle(_ context.Context, partition string) (*Bundle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.byPart[partition]
	if !ok {
		return nil, ErrNoBundle
	}
	return b, nil
}
