// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag_test

import (
	"crypto/ed25519"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// stressTestCommittee creates a test committee with n validators.
func stressTestCommittee(b *testing.B, n int) (*types.Committee, []ed25519.PrivateKey) {
	b.Helper()
	validators := make([]types.ValidatorInfo, n)
	privKeys := make([]ed25519.PrivateKey, n)

	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(b, err)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
		privKeys[i] = priv
	}

	return types.NewCommittee(validators, 0), privKeys
}

// stressTestCommitteeT creates a test committee for use with testing.T.
func stressTestCommitteeT(t *testing.T, n int) (*types.Committee, []ed25519.PrivateKey) {
	t.Helper()
	validators := make([]types.ValidatorInfo, n)
	privKeys := make([]ed25519.PrivateKey, n)

	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
		privKeys[i] = priv
	}

	return types.NewCommittee(validators, 0), privKeys
}

// createStressCert creates a test certificate for stress testing.
func createStressCert(b *testing.B, committee *types.Committee, privKeys []ed25519.PrivateKey, authorIdx int, round types.Round, parents []types.CertificateDigest) *types.Certificate {
	b.Helper()

	pub := committee.Validators[authorIdx].PublicKey
	priv := privKeys[authorIdx]

	header := types.NewHeader(pub, round, 0, nil, parents)
	err := header.Sign(priv)
	require.NoError(b, err)

	cert := types.NewCertificate(header, nil, nil)

	// Add signatures from enough validators for quorum
	headerDigest := header.Digest()
	quorumCount := (len(privKeys)*2)/3 + 1
	for i := 0; i < quorumCount; i++ {
		sig := ed25519.Sign(privKeys[i], headerDigest[:])
		cert.AddSignature(uint16(i), sig)
	}

	return cert
}

// createStressCertT creates a test certificate for use with testing.T.
func createStressCertT(t *testing.T, committee *types.Committee, privKeys []ed25519.PrivateKey, authorIdx int, round types.Round, parents []types.CertificateDigest) *types.Certificate {
	t.Helper()

	pub := committee.Validators[authorIdx].PublicKey
	priv := privKeys[authorIdx]

	header := types.NewHeader(pub, round, 0, nil, parents)
	err := header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, nil, nil)

	// Add signatures from enough validators for quorum
	headerDigest := header.Digest()
	quorumCount := (len(privKeys)*2)/3 + 1
	for i := 0; i < quorumCount; i++ {
		sig := ed25519.Sign(privKeys[i], headerDigest[:])
		cert.AddSignature(uint16(i), sig)
	}

	return cert
}

// BenchmarkDAG_HighTPS measures DAG performance under high transaction throughput.
// Target: 10,000+ TPS sustained
func BenchmarkDAG_HighTPS(b *testing.B) {
	validators := 4
	committee, privKeys := stressTestCommittee(b, validators)
	d := dag.NewDAG(100)

	// Pre-create genesis certificates
	genesisCerts := make([]*types.Certificate, validators)
	for i := 0; i < validators; i++ {
		cert := createStressCert(b, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(b, err)
		genesisCerts[i] = cert
	}

	// Track current round and parents
	currentRound := types.Round(1)
	parentDigests := make([]types.CertificateDigest, validators)
	for i, cert := range genesisCerts {
		parentDigests[i] = cert.Digest()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		authorIdx := i % validators
		cert := createStressCert(b, committee, privKeys, authorIdx, currentRound, parentDigests)
		err := d.Insert(cert)
		if err != nil {
			b.Fatal(err)
		}

		// Move to next round after all validators have submitted
		if authorIdx == validators-1 {
			// Update parents for next round
			newParents := make([]types.CertificateDigest, 0, validators)
			for _, c := range d.GetRound(currentRound) {
				newParents = append(newParents, c.Digest())
			}
			if len(newParents) > 0 {
				parentDigests = newParents
			}
			currentRound++
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "certs/sec")
}

// BenchmarkDAG_Insert measures certificate insertion performance.
func BenchmarkDAG_Insert(b *testing.B) {
	sizes := []int{4, 16, 64, 128}

	for _, n := range sizes {
		b.Run(fmt.Sprintf("validators=%d", n), func(b *testing.B) {
			committee, privKeys := stressTestCommittee(b, n)
			d := dag.NewDAG(100)

			// Create genesis
			for i := 0; i < n; i++ {
				cert := createStressCert(b, committee, privKeys, i, 0, nil)
				_ = d.InsertGenesis(cert)
			}

			parentDigests := make([]types.CertificateDigest, n)
			for i, cert := range d.GetRound(0) {
				parentDigests[i] = cert.Digest()
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				round := types.Round(1 + i/n)
				authorIdx := i % n
				cert := createStressCert(b, committee, privKeys, authorIdx, round, parentDigests[:1])
				_ = d.Insert(cert)
			}
		})
	}
}

// BenchmarkDAG_GetByDigest measures digest lookup performance.
func BenchmarkDAG_GetByDigest(b *testing.B) {
	committee, privKeys := stressTestCommittee(b, 4)
	d := dag.NewDAG(1000)

	// Populate DAG with certificates
	var certs []*types.Certificate
	var prevParents []types.CertificateDigest

	for round := types.Round(0); round <= 100; round++ {
		var currentCerts []*types.Certificate
		for i := 0; i < 4; i++ {
			var cert *types.Certificate
			if round == 0 {
				cert = createStressCert(b, committee, privKeys, i, round, nil)
				_ = d.InsertGenesis(cert)
			} else {
				cert = createStressCert(b, committee, privKeys, i, round, prevParents)
				_ = d.Insert(cert)
			}
			certs = append(certs, cert)
			currentCerts = append(currentCerts, cert)
		}
		prevParents = make([]types.CertificateDigest, len(currentCerts))
		for i, c := range currentCerts {
			prevParents[i] = c.Digest()
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cert := certs[i%len(certs)]
		_ = d.GetByDigest(cert.Digest())
	}
}

// BenchmarkDAG_GetRound measures round lookup performance.
func BenchmarkDAG_GetRound(b *testing.B) {
	committee, privKeys := stressTestCommittee(b, 4)
	d := dag.NewDAG(1000)

	// Populate DAG with 100 rounds
	var prevParents []types.CertificateDigest

	for round := types.Round(0); round <= 100; round++ {
		var currentCerts []*types.Certificate
		for i := 0; i < 4; i++ {
			var cert *types.Certificate
			if round == 0 {
				cert = createStressCert(b, committee, privKeys, i, round, nil)
				_ = d.InsertGenesis(cert)
			} else {
				cert = createStressCert(b, committee, privKeys, i, round, prevParents)
				_ = d.Insert(cert)
			}
			currentCerts = append(currentCerts, cert)
		}
		prevParents = make([]types.CertificateDigest, len(currentCerts))
		for i, c := range currentCerts {
			prevParents[i] = c.Digest()
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = d.GetRound(types.Round(i % 100))
	}
}

// BenchmarkDAG_HasQuorum measures quorum checking performance.
func BenchmarkDAG_HasQuorum(b *testing.B) {
	committee, privKeys := stressTestCommittee(b, 4)
	d := dag.NewDAG(1000)

	// Insert genesis with quorum
	for i := 0; i < 4; i++ {
		cert := createStressCert(b, committee, privKeys, i, 0, nil)
		_ = d.InsertGenesis(cert)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = d.HasQuorum(0, committee)
	}
}

// BenchmarkDAG_GarbageCollect measures GC performance.
func BenchmarkDAG_GarbageCollect(b *testing.B) {
	gcDepths := []types.Round{10, 50, 100}

	for _, gcDepth := range gcDepths {
		b.Run(fmt.Sprintf("depth=%d", gcDepth), func(b *testing.B) {
			committee, privKeys := stressTestCommittee(b, 4)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				d := dag.NewDAG(gcDepth)

				// Insert rounds up to 2x GC depth
				var prevParents []types.CertificateDigest
				for round := types.Round(0); round <= gcDepth*2; round++ {
					var currentCerts []*types.Certificate
					for j := 0; j < 4; j++ {
						var cert *types.Certificate
						if round == 0 {
							cert = createStressCert(b, committee, privKeys, j, round, nil)
							_ = d.InsertGenesis(cert)
						} else {
							cert = createStressCert(b, committee, privKeys, j, round, prevParents)
							_ = d.Insert(cert)
						}
						currentCerts = append(currentCerts, cert)
					}
					prevParents = make([]types.CertificateDigest, len(currentCerts))
					for j, c := range currentCerts {
						prevParents[j] = c.Digest()
					}
				}

				b.StartTimer()

				// Run GC
				d.GarbageCollect(gcDepth * 2)
			}
		})
	}
}

// BenchmarkDAG_ConcurrentReadWrite measures concurrent access performance.
func BenchmarkDAG_ConcurrentReadWrite(b *testing.B) {
	committee, privKeys := stressTestCommittee(b, 4)
	d := dag.NewDAG(100)

	// Create genesis
	for i := 0; i < 4; i++ {
		cert := createStressCert(b, committee, privKeys, i, 0, nil)
		_ = d.InsertGenesis(cert)
	}

	parentDigests := make([]types.CertificateDigest, 4)
	for i, cert := range d.GetRound(0) {
		parentDigests[i] = cert.Digest()
	}

	var round atomic.Int64
	round.Store(1)

	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(8)

	b.RunParallel(func(pb *testing.PB) {
		localRound := types.Round(round.Add(1))
		for pb.Next() {
			// Write operation
			cert := createStressCert(b, committee, privKeys, int(localRound)%4, localRound, parentDigests[:1])
			_ = d.Insert(cert)
			localRound++

			// Read operations
			_ = d.GetRound(0)
			_ = d.HasQuorum(0, committee)
			_ = d.Size()
		}
	})
}

// BenchmarkDAG_LargeBatch measures performance with large batch payloads.
func BenchmarkDAG_LargeBatch(b *testing.B) {
	batchSizes := []int{100, 1000, 10000}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("batch=%d", batchSize), func(b *testing.B) {
			committee, privKeys := stressTestCommittee(b, 4)
			d := dag.NewDAG(100)

			// Create payload with batch digests
			payload := make([]types.PayloadEntry, 0, batchSize)
			for i := 0; i < batchSize; i++ {
				var digest types.BatchDigest
				digest[0] = byte(i)
				digest[1] = byte(i >> 8)
				digest[2] = byte(i >> 16)
				payload = append(payload, types.PayloadEntry{Digest: digest, Worker: types.WorkerID(i % 4)})
			}

			pub := committee.Validators[0].PublicKey
			priv := privKeys[0]

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				header := types.NewHeader(pub, types.Round(i), 0, payload, nil)
				_ = header.Sign(priv)

				cert := types.NewCertificate(header, nil, nil)
				headerDigest := header.Digest()
				quorumCount := 3
				for j := 0; j < quorumCount; j++ {
					sig := ed25519.Sign(privKeys[j], headerDigest[:])
					cert.AddSignature(uint16(j), sig)
				}

				_ = d.InsertGenesis(cert)
			}

			b.ReportMetric(float64(batchSize), "batch_size")
		})
	}
}

// TestDAG_MemoryStability tests memory stability under sustained load.
// This test ensures memory does not grow unbounded.
func TestDAG_MemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory stability test in short mode")
	}

	validators := 4
	committee, privKeys := stressTestCommitteeT(t, validators)
	gcDepth := types.Round(50)
	d := dag.NewDAG(gcDepth)

	// Track memory usage
	var m runtime.MemStats

	// Create genesis
	for i := 0; i < validators; i++ {
		cert := createStressCertT(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	// Get initial memory
	runtime.GC()
	runtime.ReadMemStats(&m)
	initialAlloc := m.Alloc
	t.Logf("Initial memory: %d MB", initialAlloc/1024/1024)

	// Run for many rounds with GC
	numRounds := 500
	var prevParents []types.CertificateDigest
	for i, c := range d.GetRound(0) {
		if prevParents == nil {
			prevParents = make([]types.CertificateDigest, validators)
		}
		prevParents[i] = c.Digest()
	}

	for round := types.Round(1); round <= types.Round(numRounds); round++ {
		var currentCerts []*types.Certificate
		for i := 0; i < validators; i++ {
			cert := createStressCertT(t, committee, privKeys, i, round, prevParents)
			err := d.Insert(cert)
			require.NoError(t, err)
			currentCerts = append(currentCerts, cert)
		}

		// Update parents
		prevParents = make([]types.CertificateDigest, len(currentCerts))
		for i, c := range currentCerts {
			prevParents[i] = c.Digest()
		}

		// Run GC periodically
		if round%gcDepth == 0 {
			d.GarbageCollect(round)

			// Check memory
			runtime.GC()
			runtime.ReadMemStats(&m)
			currentAlloc := m.Alloc
			t.Logf("Round %d: memory %d MB, DAG size %d", round, currentAlloc/1024/1024, d.Size())

			// Memory should not grow unboundedly - allow 10x growth factor
			maxExpected := initialAlloc * 10
			if currentAlloc > maxExpected && currentAlloc > 100*1024*1024 {
				t.Errorf("Memory grew too much: initial=%d MB, current=%d MB",
					initialAlloc/1024/1024, currentAlloc/1024/1024)
			}
		}
	}

	// Final check
	runtime.GC()
	runtime.ReadMemStats(&m)
	finalAlloc := m.Alloc
	t.Logf("Final memory: %d MB, DAG size: %d", finalAlloc/1024/1024, d.Size())

	// DAG size should be bounded by GC depth
	maxExpectedSize := int(gcDepth+1) * validators
	if d.Size() > maxExpectedSize*2 {
		t.Errorf("DAG size %d exceeds expected max %d", d.Size(), maxExpectedSize*2)
	}
}

// TestDAG_GoroutineLeak tests that no goroutines are leaked during operations.
func TestDAG_GoroutineLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping goroutine leak test in short mode")
	}

	initialGoroutines := runtime.NumGoroutine()

	// Run multiple iterations of DAG operations
	for iter := 0; iter < 10; iter++ {
		validators := 4
		committee, privKeys := stressTestCommitteeT(t, validators)
		d := dag.NewDAG(10)

		// Create genesis
		for i := 0; i < validators; i++ {
			cert := createStressCertT(t, committee, privKeys, i, 0, nil)
			_ = d.InsertGenesis(cert)
		}

		parentDigests := make([]types.CertificateDigest, validators)
		for i, cert := range d.GetRound(0) {
			parentDigests[i] = cert.Digest()
		}

		// Concurrent operations
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				// Read operations
				_ = d.GetRound(0)
				_ = d.Size()
				_ = d.HasQuorum(0, committee)
				_ = d.LatestRound()
			}(i)
		}
		wg.Wait()

		// Run GC
		d.GarbageCollect(100)
	}

	// Give goroutines time to clean up
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()

	// Allow for some variance (test framework may spawn goroutines)
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Goroutine leak detected: initial=%d, final=%d",
			initialGoroutines, finalGoroutines)
	}
}

// TestDAG_LongDuration simulates long-duration operation.
// In actual usage, run with -timeout flag for 1+ hour tests.
func TestDAG_LongDuration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long duration test in short mode")
	}

	// For CI, run shorter version (10 seconds)
	// For full stress test, use: go test -run TestDAG_LongDuration -timeout 1h -count=1
	duration := 10 * time.Second
	if testing.Verbose() {
		duration = 60 * time.Second // 1 minute for verbose runs
	}

	validators := 4
	committee, privKeys := stressTestCommitteeT(t, validators)
	gcDepth := types.Round(100)
	d := dag.NewDAG(gcDepth)

	// Create genesis
	for i := 0; i < validators; i++ {
		cert := createStressCertT(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	parentDigests := make([]types.CertificateDigest, validators)
	for i, cert := range d.GetRound(0) {
		parentDigests[i] = cert.Digest()
	}

	startTime := time.Now()
	round := types.Round(1)
	totalOps := 0
	var lastGCTime time.Time

	for time.Since(startTime) < duration {
		// Insert certificates for this round
		var currentCerts []*types.Certificate
		for i := 0; i < validators; i++ {
			cert := createStressCertT(t, committee, privKeys, i, round, parentDigests)
			err := d.Insert(cert)
			require.NoError(t, err)
			currentCerts = append(currentCerts, cert)
			totalOps++
		}

		// Update parents
		parentDigests = make([]types.CertificateDigest, len(currentCerts))
		for i, c := range currentCerts {
			parentDigests[i] = c.Digest()
		}

		// GC periodically
		if round%gcDepth == 0 {
			d.GarbageCollect(round)
			lastGCTime = time.Now()
		}

		round++
	}

	elapsed := time.Since(startTime)
	opsPerSec := float64(totalOps) / elapsed.Seconds()

	t.Logf("Duration: %v", elapsed)
	t.Logf("Total rounds: %d", round)
	t.Logf("Total operations: %d", totalOps)
	t.Logf("Operations/sec: %.2f", opsPerSec)
	t.Logf("Final DAG size: %d", d.Size())
	t.Logf("Last GC: %v ago", time.Since(lastGCTime))

	// Verify DAG is in good state
	require.NotNil(t, d.GetRound(round-1))
}

// TestDAG_HighConcurrency tests behavior under high concurrency.
func TestDAG_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping high concurrency test in short mode")
	}

	validators := 4
	committee, privKeys := stressTestCommitteeT(t, validators)
	d := dag.NewDAG(100)

	// Create genesis
	for i := 0; i < validators; i++ {
		cert := createStressCertT(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	parentDigests := make([]types.CertificateDigest, validators)
	for i, cert := range d.GetRound(0) {
		parentDigests[i] = cert.Digest()
	}

	// High concurrency readers
	numReaders := 100
	numIterations := 1000

	var wg sync.WaitGroup
	errChan := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				// Mix of read operations
				switch j % 5 {
				case 0:
					_ = d.GetRound(0)
				case 1:
					_ = d.Size()
				case 2:
					_ = d.HasQuorum(0, committee)
				case 3:
					_ = d.LatestRound()
				case 4:
					_ = d.Rounds()
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Error during concurrent access: %v", err)
	}
}

// BenchmarkDAG_MemoryProfile runs a memory-intensive benchmark for profiling.
// Run with: go test -bench BenchmarkDAG_MemoryProfile -memprofile mem.prof
func BenchmarkDAG_MemoryProfile(b *testing.B) {
	validators := 4
	committee, privKeys := stressTestCommittee(b, validators)
	gcDepth := types.Round(50)
	d := dag.NewDAG(gcDepth)

	// Create genesis
	for i := 0; i < validators; i++ {
		cert := createStressCert(b, committee, privKeys, i, 0, nil)
		_ = d.InsertGenesis(cert)
	}

	parentDigests := make([]types.CertificateDigest, validators)
	for i, cert := range d.GetRound(0) {
		parentDigests[i] = cert.Digest()
	}

	b.ResetTimer()
	b.ReportAllocs()

	round := types.Round(1)
	for i := 0; i < b.N; i++ {
		// Insert certificates
		var currentCerts []*types.Certificate
		for j := 0; j < validators; j++ {
			cert := createStressCert(b, committee, privKeys, j, round, parentDigests)
			_ = d.Insert(cert)
			currentCerts = append(currentCerts, cert)
		}

		// Update parents
		parentDigests = make([]types.CertificateDigest, len(currentCerts))
		for j, c := range currentCerts {
			parentDigests[j] = c.Digest()
		}

		// GC periodically
		if round%gcDepth == 0 {
			d.GarbageCollect(round)
		}

		round++
	}
}

// BenchmarkDAG_CPUProfile runs a CPU-intensive benchmark for profiling.
// Run with: go test -bench BenchmarkDAG_CPUProfile -cpuprofile cpu.prof
func BenchmarkDAG_CPUProfile(b *testing.B) {
	validators := 4
	committee, privKeys := stressTestCommittee(b, validators)
	d := dag.NewDAG(100)

	// Create genesis
	genesisCerts := make([]*types.Certificate, validators)
	for i := 0; i < validators; i++ {
		cert := createStressCert(b, committee, privKeys, i, 0, nil)
		_ = d.InsertGenesis(cert)
		genesisCerts[i] = cert
	}

	parentDigests := make([]types.CertificateDigest, validators)
	for i, cert := range genesisCerts {
		parentDigests[i] = cert.Digest()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		round := types.Round(1 + i/validators)
		authorIdx := i % validators
		cert := createStressCert(b, committee, privKeys, authorIdx, round, parentDigests[:1])
		_ = d.Insert(cert)

		// Mix in some read operations
		_ = d.GetByDigest(genesisCerts[0].Digest())
		_ = d.HasQuorum(0, committee)
	}
}
