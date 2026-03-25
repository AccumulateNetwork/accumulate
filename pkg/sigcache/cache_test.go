// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sigcache

import (
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"
)

func TestCache_StoreAndCheck(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	data := []byte("test data")
	sig := []byte("signature")
	pubKey := []byte("public key")

	// Cache miss initially
	result, found := cache.Check(data, sig, pubKey)
	if found {
		t.Error("expected cache miss")
	}

	// Store valid result
	cache.Store(data, sig, pubKey, true)

	// Cache hit
	result, found = cache.Check(data, sig, pubKey)
	if !found {
		t.Error("expected cache hit")
	}
	if !result {
		t.Error("expected valid result")
	}

	// Verify metrics
	hits, misses := cache.Metrics()
	if hits != 1 || misses != 1 {
		t.Errorf("unexpected metrics: hits=%d, misses=%d", hits, misses)
	}
}

func TestCache_TTL(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        50 * time.Millisecond,
		Enabled:    true,
	})

	data := []byte("test data")
	sig := []byte("signature")
	pubKey := []byte("public key")

	// Store result
	cache.Store(data, sig, pubKey, true)

	// Should be found immediately
	_, found := cache.Check(data, sig, pubKey)
	if !found {
		t.Error("expected cache hit")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	_, found = cache.Check(data, sig, pubKey)
	if found {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	maxEntries := 10
	cache := New(&Config{
		MaxEntries: maxEntries,
		TTL:        time.Minute,
		Enabled:    true,
	})

	// Add entries up to capacity
	for i := 0; i < maxEntries; i++ {
		data := []byte{byte(i)}
		cache.Store(data, data, data, true)
	}

	if cache.Size() != maxEntries {
		t.Errorf("expected size %d, got %d", maxEntries, cache.Size())
	}

	// Add one more entry - should evict oldest
	newData := []byte{byte(maxEntries)}
	cache.Store(newData, newData, newData, true)

	if cache.Size() != maxEntries {
		t.Errorf("expected size %d after eviction, got %d", maxEntries, cache.Size())
	}

	// First entry should be evicted
	firstData := []byte{0}
	_, found := cache.Check(firstData, firstData, firstData)
	if found {
		t.Error("expected oldest entry to be evicted")
	}

	// Last entry should still be there
	_, found = cache.Check(newData, newData, newData)
	if !found {
		t.Error("expected newest entry to be present")
	}
}

func TestCache_Disabled(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    false,
	})

	data := []byte("test data")
	sig := []byte("signature")
	pubKey := []byte("public key")

	// Store should be no-op
	cache.Store(data, sig, pubKey, true)

	// Check should always miss
	_, found := cache.Check(data, sig, pubKey)
	if found {
		t.Error("expected cache miss when disabled")
	}

	if cache.Size() != 0 {
		t.Error("expected cache to be empty when disabled")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	// Add some entries
	for i := 0; i < 10; i++ {
		data := []byte{byte(i)}
		cache.Store(data, data, data, true)
	}

	if cache.Size() != 10 {
		t.Errorf("expected size 10, got %d", cache.Size())
	}

	// Clear cache
	cache.Clear()

	if cache.Size() != 0 {
		t.Error("expected cache to be empty after clear")
	}

	// Verify entries are gone
	data := []byte{0}
	_, found := cache.Check(data, data, data)
	if found {
		t.Error("expected cache miss after clear")
	}
}

func TestCache_HitRate(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	data := []byte("test")
	sig := []byte("sig")
	pubKey := []byte("key")

	// 1 miss
	cache.Check(data, sig, pubKey)

	// Store
	cache.Store(data, sig, pubKey, true)

	// 3 hits
	cache.Check(data, sig, pubKey)
	cache.Check(data, sig, pubKey)
	cache.Check(data, sig, pubKey)

	hitRate := cache.HitRate()
	expected := 75.0 // 3 hits out of 4 total checks
	if hitRate != expected {
		t.Errorf("expected hit rate %.2f%%, got %.2f%%", expected, hitRate)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 1000,
		TTL:        time.Second,
		Enabled:    true,
	})

	var wg sync.WaitGroup
	numGoroutines := 10
	numOps := 100

	// Concurrent writes and reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				data := []byte{byte(id), byte(j)}
				cache.Store(data, data, data, true)
				cache.Check(data, data, data)
			}
		}(i)
	}

	wg.Wait()

	// Verify no panics and cache is in valid state
	if cache.Size() < 0 || cache.Size() > cache.config.MaxEntries {
		t.Errorf("invalid cache size after concurrent access: %d", cache.Size())
	}
}

func TestCache_UpdateExisting(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	data := []byte("test")
	sig := []byte("sig")
	pubKey := []byte("key")

	// Store as valid
	cache.Store(data, sig, pubKey, true)

	// Update to invalid
	cache.Store(data, sig, pubKey, false)

	// Should return updated value
	result, found := cache.Check(data, sig, pubKey)
	if !found {
		t.Error("expected cache hit")
	}
	if result {
		t.Error("expected invalid result after update")
	}

	// Size should not increase
	if cache.Size() != 1 {
		t.Errorf("expected size 1 after update, got %d", cache.Size())
	}
}

func TestCache_DifferentKeys(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	data1 := []byte("data1")
	data2 := []byte("data2")
	sig := []byte("sig")
	pubKey := []byte("key")

	// Store with different data
	cache.Store(data1, sig, pubKey, true)
	cache.Store(data2, sig, pubKey, false)

	// Should have separate entries
	result1, found1 := cache.Check(data1, sig, pubKey)
	result2, found2 := cache.Check(data2, sig, pubKey)

	if !found1 || !found2 {
		t.Error("expected both entries to be found")
	}
	if !result1 || result2 {
		t.Error("expected different results for different keys")
	}
}

func TestCache_ResetMetrics(t *testing.T) {
	cache := New(&Config{
		MaxEntries: 100,
		TTL:        time.Second,
		Enabled:    true,
	})

	data := []byte("test")

	// Generate some metrics
	cache.Check(data, data, data) // miss
	cache.Store(data, data, data, true)
	cache.Check(data, data, data) // hit

	hits, misses := cache.Metrics()
	if hits == 0 || misses == 0 {
		t.Error("expected non-zero metrics")
	}

	cache.ResetMetrics()

	hits, misses = cache.Metrics()
	if hits != 0 || misses != 0 {
		t.Error("expected metrics to be reset to zero")
	}
}

func BenchmarkCache_Hit(b *testing.B) {
	cache := New(&Config{
		MaxEntries: 10000,
		TTL:        time.Minute,
		Enabled:    true,
	})

	data := []byte("benchmark data")
	sig := []byte("signature")
	pubKey := []byte("public key")

	cache.Store(data, sig, pubKey, true)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Check(data, sig, pubKey)
		}
	})
}

func BenchmarkCache_Miss(b *testing.B) {
	cache := New(&Config{
		MaxEntries: 10000,
		TTL:        time.Minute,
		Enabled:    true,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		data := make([]byte, 32)
		for pb.Next() {
			rand.Read(data)
			cache.Check(data, data, data)
		}
	})
}

func BenchmarkCache_Store(b *testing.B) {
	cache := New(&Config{
		MaxEntries: 10000,
		TTL:        time.Minute,
		Enabled:    true,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := []byte{byte(i), byte(i >> 8), byte(i >> 16)}
		cache.Store(data, data, data, true)
	}
}

func BenchmarkCache_MixedLoad(b *testing.B) {
	cache := New(&Config{
		MaxEntries: 10000,
		TTL:        time.Minute,
		Enabled:    true,
	})

	// Pre-populate with some entries
	for i := 0; i < 5000; i++ {
		data := []byte{byte(i), byte(i >> 8)}
		cache.Store(data, data, data, true)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			data := []byte{byte(i), byte(i >> 8)}
			if i%2 == 0 {
				cache.Check(data, data, data)
			} else {
				cache.Store(data, data, data, true)
			}
			i++
		}
	})
}

func BenchmarkCache_ED25519Verification(b *testing.B) {
	// Benchmark comparing cached vs uncached ED25519 verification
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, 32)
	rand.Read(data)
	sig := ed25519.Sign(priv, data)

	b.Run("Uncached", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ed25519.Verify(pub, data, sig)
		}
	})

	b.Run("Cached", func(b *testing.B) {
		cache := New(&Config{
			MaxEntries: 10000,
			TTL:        time.Minute,
			Enabled:    true,
		})
		cache.Store(data, sig, pub, true)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if result, found := cache.Check(data, sig, pub); !found {
				valid := ed25519.Verify(pub, data, sig)
				cache.Store(data, sig, pub, valid)
			} else {
				_ = result
			}
		}
	})
}
