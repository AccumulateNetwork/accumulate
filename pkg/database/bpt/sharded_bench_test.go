// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// BenchmarkShardedInsert compares sharded vs non-sharded insert performance
// with varying goroutine counts and shard depths.
func BenchmarkShardedInsert(b *testing.B) {
	b.ReportAllocs()
	depths := []int{4, 5, 6} // 16, 32, 64 shards
	goroutineCounts := []int{1, 4, 8, 16, 32}

	for _, depth := range depths {
		numShards := 1 << depth
		b.Run(fmt.Sprintf("sharded-depth=%d-shards=%d", depth, numShards), func(b *testing.B) {
			for _, goroutines := range goroutineCounts {
				b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
					kvs := memory.New(nil).Begin(nil, true)
					defer kvs.Discard()
					store := keyvalue.RecordStore{Store: kvs}

					sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
					if err != nil {
						b.Fatal(err)
					}

					b.ResetTimer()
					b.SetParallelism(goroutines)

					var counter int64
					b.RunParallel(func(pb *testing.PB) {
						var rh common.RandHash
						for pb.Next() {
							idx := atomic.AddInt64(&counter, 1)
							key := record.KeyFromHash(rh.NextA())
							value := rh.Next()

							err := sharded.Insert(key, value)
							if err != nil {
								b.Fatalf("insert %d failed: %v", idx, err)
							}
						}
					})
				})
			}
		})
	}

	// Benchmark non-sharded for comparison
	b.Run("non-sharded", func(b *testing.B) {
		for _, goroutines := range goroutineCounts {
			b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
				kvs := memory.New(nil).Begin(nil, true)
				defer kvs.Discard()
				store := keyvalue.RecordStore{Store: kvs}

				bpt := New(nil, nil, store, record.NewKey("test"))

				// Non-sharded BPT needs external locking for concurrency
				var mu sync.Mutex

				b.ResetTimer()
				b.SetParallelism(goroutines)

				var counter int64
				b.RunParallel(func(pb *testing.PB) {
					var rh common.RandHash
					for pb.Next() {
						idx := atomic.AddInt64(&counter, 1)
						key := record.KeyFromHash(rh.NextA())
						value := rh.Next()

						mu.Lock()
						err := bpt.Insert(key, value)
						mu.Unlock()

						if err != nil {
							b.Fatalf("insert %d failed: %v", idx, err)
						}
					}
				})
			})
		}
	})
}

// BenchmarkShardedGet measures lookup performance with concurrent reads.
func BenchmarkShardedGet(b *testing.B) {
	b.ReportAllocs()
	depths := []int{4, 5, 6}
	goroutineCounts := []int{1, 4, 8, 16, 32}
	numEntries := 10000

	for _, depth := range depths {
		numShards := 1 << depth
		b.Run(fmt.Sprintf("sharded-depth=%d-shards=%d", depth, numShards), func(b *testing.B) {
			// Pre-populate with entries
			kvs := memory.New(nil).Begin(nil, true)
			defer kvs.Discard()
			store := keyvalue.RecordStore{Store: kvs}

			sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
			if err != nil {
				b.Fatal(err)
			}

			keys := make([]*record.Key, numEntries)
			var rh common.RandHash
			for i := 0; i < numEntries; i++ {
				key := record.KeyFromHash(rh.NextA())
				value := rh.Next()
				keys[i] = key
				if err := sharded.Insert(key, value); err != nil {
					b.Fatal(err)
				}
			}

			// Benchmark lookups
			for _, goroutines := range goroutineCounts {
				b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
					b.ResetTimer()
					b.SetParallelism(goroutines)

					var counter int64
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							idx := atomic.AddInt64(&counter, 1) % int64(numEntries)
							key := keys[idx]

							_, err := sharded.Get(key)
							if err != nil {
								b.Fatalf("get failed: %v", err)
							}
						}
					})
				})
			}
		})
	}

	// Benchmark non-sharded for comparison
	b.Run("non-sharded", func(b *testing.B) {
		kvs := memory.New(nil).Begin(nil, true)
		defer kvs.Discard()
		store := keyvalue.RecordStore{Store: kvs}

		bpt := New(nil, nil, store, record.NewKey("test"))

		keys := make([]*record.Key, numEntries)
		var rh common.RandHash
		for i := 0; i < numEntries; i++ {
			key := record.KeyFromHash(rh.NextA())
			value := rh.Next()
			keys[i] = key
			if err := bpt.Insert(key, value); err != nil {
				b.Fatal(err)
			}
		}

		var mu sync.Mutex

		for _, goroutines := range goroutineCounts {
			b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
				b.ResetTimer()
				b.SetParallelism(goroutines)

				var counter int64
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						idx := atomic.AddInt64(&counter, 1) % int64(numEntries)
						key := keys[idx]

						mu.Lock()
						_, err := bpt.Get(key)
						mu.Unlock()

						if err != nil {
							b.Fatalf("get failed: %v", err)
						}
					}
				})
			})
		}
	})
}

// BenchmarkGetRootHash measures the critical path - computing root hash
// after many inserts. This should show the 16x speedup from parallel
// executePending across shards.
func BenchmarkGetRootHash(b *testing.B) {
	b.ReportAllocs()
	entryCounts := []int{1000, 5000, 10000}
	depths := []int{4, 5, 6}

	for _, numEntries := range entryCounts {
		b.Run(fmt.Sprintf("entries=%d", numEntries), func(b *testing.B) {
			for _, depth := range depths {
				numShards := 1 << depth
				b.Run(fmt.Sprintf("sharded-depth=%d-shards=%d", depth, numShards), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()

						// Setup fresh BPT for each iteration
						kvs := memory.New(nil).Begin(nil, true)
						store := keyvalue.RecordStore{Store: kvs}

						sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
						if err != nil {
							b.Fatal(err)
						}

						// Insert entries
						var rh common.RandHash
						for j := 0; j < numEntries; j++ {
							key := record.KeyFromHash(rh.NextA())
							value := rh.Next()
							if err := sharded.Insert(key, value); err != nil {
								b.Fatal(err)
							}
						}

						b.StartTimer()

						// Measure GetRootHash time (includes parallel executePending)
						_, err = sharded.GetRootHash()
						if err != nil {
							b.Fatal(err)
						}

						b.StopTimer()
						kvs.Discard()
					}
				})
			}

			// Benchmark non-sharded for comparison
			b.Run("non-sharded", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					b.StopTimer()

					kvs := memory.New(nil).Begin(nil, true)
					store := keyvalue.RecordStore{Store: kvs}

					bpt := New(nil, nil, store, record.NewKey("test"))

					var rh common.RandHash
					for j := 0; j < numEntries; j++ {
						key := record.KeyFromHash(rh.NextA())
						value := rh.Next()
						if err := bpt.Insert(key, value); err != nil {
							b.Fatal(err)
						}
					}

					b.StartTimer()

					// Measure GetRootHash time (sequential executePending)
					_, err := bpt.GetRootHash()
					if err != nil {
						b.Fatal(err)
					}

					b.StopTimer()
					kvs.Discard()
				}
			})
		})
	}
}

// BenchmarkShardedVsNonSharded directly compares same workload on both
// implementations with various goroutine counts.
func BenchmarkShardedVsNonSharded(b *testing.B) {
	b.ReportAllocs()
	goroutineCounts := []int{1, 4, 8, 16, 32}
	depth := 4 // 16 shards
	numShards := 1 << depth

	for _, goroutines := range goroutineCounts {
		b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
			b.Run(fmt.Sprintf("sharded-%d-shards", numShards), func(b *testing.B) {
				kvs := memory.New(nil).Begin(nil, true)
				defer kvs.Discard()
				store := keyvalue.RecordStore{Store: kvs}

				sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
				if err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				b.SetParallelism(goroutines)

				var counter int64
				b.RunParallel(func(pb *testing.PB) {
					var rh common.RandHash
					for pb.Next() {
						idx := atomic.AddInt64(&counter, 1)
						key := record.KeyFromHash(rh.NextA())
						value := rh.Next()

						err := sharded.Insert(key, value)
						if err != nil {
							b.Fatalf("insert %d failed: %v", idx, err)
						}
					}
				})
			})

			b.Run("non-sharded", func(b *testing.B) {
				kvs := memory.New(nil).Begin(nil, true)
				defer kvs.Discard()
				store := keyvalue.RecordStore{Store: kvs}

				bpt := New(nil, nil, store, record.NewKey("test"))
				var mu sync.Mutex

				b.ResetTimer()
				b.SetParallelism(goroutines)

				var counter int64
				b.RunParallel(func(pb *testing.PB) {
					var rh common.RandHash
					for pb.Next() {
						idx := atomic.AddInt64(&counter, 1)
						key := record.KeyFromHash(rh.NextA())
						value := rh.Next()

						mu.Lock()
						err := bpt.Insert(key, value)
						mu.Unlock()

						if err != nil {
							b.Fatalf("insert %d failed: %v", idx, err)
						}
					}
				})
			})
		})
	}
}

// BenchmarkContention measures lock contention across different shard configurations.
//
// NOTE: This benchmark measures throughput under contention rather than using
// inline timing (which adds measurement overhead). For detailed contention analysis,
// use Go's profiling tools:
//
//	go test -bench=BenchmarkContention -cpuprofile=cpu.out -blockprofile=block.out
//	go tool pprof -http=:8080 block.out
//
// The block profile will show actual lock contention without measurement overhead.
func BenchmarkContention(b *testing.B) {
	b.ReportAllocs()

	depths := []int{4, 5, 6} // 16, 32, 64 shards
	goroutines := []int{4, 8, 16, 32}

	for _, depth := range depths {
		for _, numGoroutines := range goroutines {
			name := fmt.Sprintf("depth=%d,shards=%d,goroutines=%d",
				depth, 1<<depth, numGoroutines)

			b.Run(name, func(b *testing.B) {
				kvs := memory.New(nil).Begin(nil, true)
				defer kvs.Discard()
				store := keyvalue.RecordStore{Store: kvs}

				bptKey := record.NewKey("contention-bench")
				sharded, err := NewShardedBPT(store, bptKey, depth)
				if err != nil {
					b.Fatal(err)
				}

				b.SetParallelism(numGoroutines)
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					var rh common.RandHash
					for pb.Next() {
						key := record.KeyFromHash(rh.NextA())
						value := rh.Next()

						err := sharded.Insert(key, value)
						if err != nil {
							b.Error(err)
							return
						}
					}
				})
			})
		}
	}
}

// BenchmarkShardedDelete measures delete performance across different configurations.
// Deletes are tested on a pre-populated tree to measure realistic delete performance.
func BenchmarkShardedDelete(b *testing.B) {
	b.ReportAllocs()

	depths := []int{4, 5, 6} // 16, 32, 64 shards
	prepopulate := 10000     // Pre-populate with 10K entries

	for _, depth := range depths {
		numShards := 1 << depth
		name := fmt.Sprintf("depth=%d,shards=%d", depth, numShards)

		b.Run(name, func(b *testing.B) {
			kvs := memory.New(nil).Begin(nil, true)
			defer kvs.Discard()
			store := keyvalue.RecordStore{Store: kvs}

			bptKey := record.NewKey("delete-bench")
			sharded, err := NewShardedBPT(store, bptKey, depth)
			if err != nil {
				b.Fatal(err)
			}

			// Pre-populate tree
			var rh common.RandHash
			keys := make([]*record.Key, prepopulate)
			for i := 0; i < prepopulate; i++ {
				key := record.KeyFromHash(rh.NextA())
				keys[i] = key
				value := rh.Next()
				_ = sharded.Insert(key, value)
			}

			b.ResetTimer()

			// Benchmark deletes
			// Note: We delete keys in a cycle to keep tree populated
			idx := 0
			for i := 0; i < b.N; i++ {
				key := keys[idx%prepopulate]
				err := sharded.Delete(key)
				if err != nil {
					b.Error(err)
					return
				}
				// Re-insert to maintain tree size
				if (i+1)%prepopulate == 0 {
					for j := 0; j < prepopulate; j++ {
						value := rh.Next()
						_ = sharded.Insert(keys[j], value)
					}
				}
				idx++
			}
		})
	}
}

// BenchmarkShardedMemory measures memory allocation patterns for insert operations.
// This helps detect allocation regressions and guides optimization efforts.
func BenchmarkShardedMemory(b *testing.B) {
	b.ReportAllocs()

	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	bptKey := record.NewKey("memory-bench")
	sharded, err := NewShardedBPT(store, bptKey, 4) // 16 shards
	if err != nil {
		b.Fatal(err)
	}

	var rh common.RandHash

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := record.KeyFromHash(rh.NextA())
		value := rh.Next()

		err := sharded.Insert(key, value)
		if err != nil {
			b.Error(err)
			return
		}
	}
}

// BenchmarkMixedWorkload simulates realistic usage with mixed insert/get operations.
func BenchmarkMixedWorkload(b *testing.B) {
	b.ReportAllocs()
	depths := []int{0, 4, 5, 6} // 0 = non-sharded
	goroutineCounts := []int{4, 8, 16}
	readRatio := 0.7 // 70% reads, 30% writes

	for _, depth := range depths {
		var label string
		if depth == 0 {
			label = "non-sharded"
		} else {
			numShards := 1 << depth
			label = fmt.Sprintf("sharded-%d-shards", numShards)
		}

		b.Run(label, func(b *testing.B) {
			for _, goroutines := range goroutineCounts {
				b.Run(fmt.Sprintf("goroutines=%d", goroutines), func(b *testing.B) {
					kvs := memory.New(nil).Begin(nil, true)
					defer kvs.Discard()
					store := keyvalue.RecordStore{Store: kvs}

					// Pre-populate with some data
					keys := make([]*record.Key, 1000)
					var rh common.RandHash

					if depth == 0 {
						bpt := New(nil, nil, store, record.NewKey("test"))
						for i := 0; i < 1000; i++ {
							key := record.KeyFromHash(rh.NextA())
							value := rh.Next()
							keys[i] = key
							if err := bpt.Insert(key, value); err != nil {
								b.Fatal(err)
							}
						}

						var mu sync.Mutex

						b.ResetTimer()
						b.SetParallelism(goroutines)

						var counter int64
						b.RunParallel(func(pb *testing.PB) {
							var localRh common.RandHash
							for pb.Next() {
								idx := atomic.AddInt64(&counter, 1)

								mu.Lock()
								if float64(idx%100)/100.0 < readRatio {
									// Read operation
									key := keys[idx%int64(len(keys))]
									_, _ = bpt.Get(key)
								} else {
									// Write operation
									key := record.KeyFromHash(localRh.NextA())
									value := localRh.Next()
									_ = bpt.Insert(key, value)
								}
								mu.Unlock()
							}
						})
					} else {
						sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
						if err != nil {
							b.Fatal(err)
						}

						for i := 0; i < 1000; i++ {
							key := record.KeyFromHash(rh.NextA())
							value := rh.Next()
							keys[i] = key
							if err := sharded.Insert(key, value); err != nil {
								b.Fatal(err)
							}
						}

						b.ResetTimer()
						b.SetParallelism(goroutines)

						var counter int64
						b.RunParallel(func(pb *testing.PB) {
							var localRh common.RandHash
							for pb.Next() {
								idx := atomic.AddInt64(&counter, 1)

								if float64(idx%100)/100.0 < readRatio {
									// Read operation
									key := keys[idx%int64(len(keys))]
									_, _ = sharded.Get(key)
								} else {
									// Write operation
									key := record.KeyFromHash(localRh.NextA())
									value := localRh.Next()
									_ = sharded.Insert(key, value)
								}
							}
						})
					}
				})
			}
		})
	}
}
