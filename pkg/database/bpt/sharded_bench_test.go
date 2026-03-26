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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// BenchmarkShardedInsert compares sharded vs non-sharded insert performance
// with varying goroutine counts and shard depths.
func BenchmarkShardedInsert(b *testing.B) {
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

// BenchmarkContention measures lock contention by timing how long goroutines
// spend waiting for locks vs doing actual work.
func BenchmarkContention(b *testing.B) {
	depths := []int{0, 4, 5, 6} // 0 = non-sharded
	goroutineCounts := []int{4, 8, 16, 32}
	opsPerGoroutine := 1000

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
					for i := 0; i < b.N; i++ {
						b.StopTimer()

						kvs := memory.New(nil).Begin(nil, true)
						store := keyvalue.RecordStore{Store: kvs}

						var totalWaitTime int64
						var totalWorkTime int64

						if depth == 0 {
							// Non-sharded with single lock
							bpt := New(nil, nil, store, record.NewKey("test"))
							var mu sync.Mutex

							var wg sync.WaitGroup
							wg.Add(goroutines)

							b.StartTimer()

							for g := 0; g < goroutines; g++ {
								go func() {
									defer wg.Done()

									var rh common.RandHash
									for j := 0; j < opsPerGoroutine; j++ {
										key := record.KeyFromHash(rh.NextA())
										value := rh.Next()

										lockStart := time.Now()
										mu.Lock()
										lockEnd := time.Now()

										workStart := time.Now()
										_ = bpt.Insert(key, value)
										workEnd := time.Now()

										mu.Unlock()

										atomic.AddInt64(&totalWaitTime, lockEnd.Sub(lockStart).Nanoseconds())
										atomic.AddInt64(&totalWorkTime, workEnd.Sub(workStart).Nanoseconds())
									}
								}()
							}

							wg.Wait()
						} else {
							// Sharded with per-shard locks
							sharded, err := NewShardedBPT(store, record.NewKey("test"), depth)
							if err != nil {
								b.Fatal(err)
							}

							var wg sync.WaitGroup
							wg.Add(goroutines)

							b.StartTimer()

							for g := 0; g < goroutines; g++ {
								go func() {
									defer wg.Done()

									var rh common.RandHash
									for j := 0; j < opsPerGoroutine; j++ {
										key := record.KeyFromHash(rh.NextA())
										value := rh.Next()

										lockStart := time.Now()
										shardID, shard := sharded.routeToShard(key.Hash())
										sharded.shardMu[shardID].Lock()
										lockEnd := time.Now()

										workStart := time.Now()
										_ = shard.Insert(key, value)
										workEnd := time.Now()

										sharded.shardMu[shardID].Unlock()

										atomic.AddInt64(&totalWaitTime, lockEnd.Sub(lockStart).Nanoseconds())
										atomic.AddInt64(&totalWorkTime, workEnd.Sub(workStart).Nanoseconds())
									}
								}()
							}

							wg.Wait()
						}

						b.StopTimer()

						// Report contention metrics
						waitMs := float64(totalWaitTime) / 1e6
						workMs := float64(totalWorkTime) / 1e6
						contentionRatio := float64(totalWaitTime) / float64(totalWorkTime+totalWaitTime)

						b.ReportMetric(waitMs, "wait_ms")
						b.ReportMetric(workMs, "work_ms")
						b.ReportMetric(contentionRatio*100, "contention_%")

						kvs.Discard()
					}
				})
			}
		})
	}
}

// BenchmarkMixedWorkload simulates realistic usage with mixed insert/get operations.
func BenchmarkMixedWorkload(b *testing.B) {
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
