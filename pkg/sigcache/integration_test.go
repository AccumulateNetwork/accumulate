// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sigcache

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestVerifyED25519_WithCache(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("test message")
	sig := ed25519.Sign(priv, data)

	// First verification should miss cache
	if !VerifyED25519(cache, data, sig, pub) {
		t.Error("expected valid signature")
	}

	hits, misses := cache.Metrics()
	if hits != 0 || misses != 1 {
		t.Errorf("expected 0 hits and 1 miss, got %d hits and %d misses", hits, misses)
	}

	// Second verification should hit cache
	if !VerifyED25519(cache, data, sig, pub) {
		t.Error("expected valid signature")
	}

	hits, misses = cache.Metrics()
	if hits != 1 || misses != 1 {
		t.Errorf("expected 1 hit and 1 miss, got %d hits and %d misses", hits, misses)
	}
}

func TestVerifyED25519_InvalidSignature(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("test message")
	invalidSig := make([]byte, ed25519.SignatureSize)
	rand.Read(invalidSig)

	// Should cache invalid result
	if VerifyED25519(cache, data, invalidSig, pub) {
		t.Error("expected invalid signature")
	}

	// Second check should hit cache with invalid result
	if VerifyED25519(cache, data, invalidSig, pub) {
		t.Error("expected invalid signature from cache")
	}

	hits, misses := cache.Metrics()
	if hits != 1 || misses != 1 {
		t.Errorf("expected 1 hit and 1 miss, got %d hits and %d misses", hits, misses)
	}
}

func TestVerifyED25519_WithoutCache(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("test message")
	sig := ed25519.Sign(priv, data)

	// Should work without cache
	if !VerifyED25519(nil, data, sig, pub) {
		t.Error("expected valid signature")
	}

	// Should work with disabled cache
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    false,
	})

	if !VerifyED25519(cache, data, sig, pub) {
		t.Error("expected valid signature")
	}

	// Cache should be empty
	if cache.Size() != 0 {
		t.Error("expected cache to be empty when disabled")
	}
}

func BenchmarkVerifyED25519_Comparison(b *testing.B) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, 32)
	rand.Read(data)
	sig := ed25519.Sign(priv, data)

	cache := New(&Config{
		MaxEntries: 10000,
		TTL:        time.Minute,
		Enabled:    true,
	})

	b.Run("DirectED25519", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ed25519.Verify(pub, data, sig)
		}
	})

	b.Run("WithCache_ColdStart", func(b *testing.B) {
		cache.Clear()
		cache.ResetMetrics()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			VerifyED25519(cache, data, sig, pub)
		}
		b.StopTimer()
		hits, misses := cache.Metrics()
		hitRate := cache.HitRate()
		b.ReportMetric(hitRate, "hit_rate_%")
		b.Logf("Hits: %d, Misses: %d, Hit Rate: %.2f%%", hits, misses, hitRate)
	})

	b.Run("WithCache_WarmStart", func(b *testing.B) {
		// Prime the cache
		VerifyED25519(cache, data, sig, pub)
		cache.ResetMetrics()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			VerifyED25519(cache, data, sig, pub)
		}
		b.StopTimer()
		hits, misses := cache.Metrics()
		hitRate := cache.HitRate()
		b.ReportMetric(hitRate, "hit_rate_%")
		b.Logf("Hits: %d, Misses: %d, Hit Rate: %.2f%%", hits, misses, hitRate)
	})
}
