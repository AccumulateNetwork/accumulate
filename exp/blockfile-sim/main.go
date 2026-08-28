// Command blockfile-sim measures whether moving immutable entries out of
// LevelDB into append-only "major block files" reduces LevelDB overhead.
//
// Two layouts run the same workload:
//
//   - mode=leveldb (baseline): every write — immutable entries (transactions,
//     signatures, statuses, chain entries) and mutable BPT nodes — goes into
//     LevelDB as a full value, exactly like the node today.
//
//   - mode=blockfile: immutable entries are appended to a flat block file
//     (rolled by size, standing in for the 12-hour major block); LevelDB holds
//     only a fixed-width locator (file, offset, length) per key, plus the same
//     mutable BPT churn as the baseline.
//
// The workload models a 1000 TPS chain: per transaction ~7 immutable writes
// (~1.1 KiB), 6 BPT node rewrites over a bounded working set, and a read mix
// of random historical entries (healers/trackers) and random BPT nodes.
// Writes are batched per block (block-per-commit). LevelDB options mirror
// pkg/database/keyvalue/leveldb: bloom(10), 256 MiB block cache, 16 MiB write
// buffer, buffer pool disabled.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	mode     = flag.String("mode", "", "leveldb | blockfile")
	dir      = flag.String("dir", "", "data directory (created fresh)")
	txs      = flag.Int("txs", 1_000_000, "transactions to simulate")
	blockTxs = flag.Int("block", 1000, "transactions per block (one batch commit)")
	bptKeys  = flag.Int("bpt", 400_000, "distinct BPT node keys (mutable working set)")
	readsImm = flag.Int("reads-imm", 3, "random immutable reads per tx")
	readsBpt = flag.Int("reads-bpt", 3, "random BPT reads per tx")
	fileMax  = flag.Int64("filemax", 512<<20, "block file roll size")
	cacheMB  = flag.Int("cache", 256, "leveldb block cache capacity, MiB")
	writeMB  = flag.Int("writebuf", 16, "leveldb write buffer, MiB")
	label    = flag.String("label", "", "label for the result line")
)

// Immutable writes per tx: index 0 is the transaction blob, the rest are
// signatures, status, and chain/merkle entries. Sizes are fixed per slot so
// both modes see identical bytes and keys are reconstructible for reads.
var immSizes = []int{512, 200, 200, 150, 64, 64, 64} // ~1254 B/tx
const bptValSize = 300
const bptWritesPerTx = 6

func immKey(tx, slot int) [32]byte { return derive(uint64(tx)*8 + uint64(slot)) }
func bptKey(i int) [32]byte        { return derive(1<<62 | uint64(i)) }

// derive fills 32 bytes from splitmix64 — unique, deterministic, hash-like.
func derive(x uint64) (k [32]byte) {
	for i := 0; i < 4; i++ {
		x += 0x9e3779b97f4a7c15
		z := x
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		z ^= z >> 31
		binary.BigEndian.PutUint64(k[i*8:], z)
	}
	return k
}

// rng is a splitmix64 stream for workload choices (which keys to read, etc).
type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}
func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

// blockStore is the append-only major-block file store.
type blockStore struct {
	dir    string
	cur    *os.File
	w      *bufio.Writer
	fileID uint32
	off    int64
	files  map[uint32]*os.File // read handles
}

func newBlockStore(dir string) (*blockStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &blockStore{dir: dir, files: map[uint32]*os.File{}}
	return s, s.roll()
}

func (s *blockStore) roll() error {
	if s.w != nil {
		if err := s.w.Flush(); err != nil {
			return err
		}
		if err := s.cur.Sync(); err != nil {
			return err
		}
		s.fileID++
	}
	f, err := os.Create(filepath.Join(s.dir, fmt.Sprintf("block-%06d.dat", s.fileID)))
	if err != nil {
		return err
	}
	s.cur, s.w, s.off = f, bufio.NewWriterSize(f, 1<<20), 0
	s.files[s.fileID] = f
	return nil
}

// append stores val and returns its 13-byte locator.
func (s *blockStore) append(val []byte) ([13]byte, error) {
	var loc [13]byte
	binary.BigEndian.PutUint32(loc[0:], s.fileID)
	binary.BigEndian.PutUint64(loc[4:], uint64(s.off))
	loc[12] = byte(len(val) >> 3) // lengths here fit; real impl uses varint
	if _, err := s.w.Write(val); err != nil {
		return loc, err
	}
	s.off += int64(len(val))
	if s.off >= *fileMax {
		return loc, s.roll()
	}
	return loc, nil
}

func (s *blockStore) read(loc []byte, buf []byte) ([]byte, error) {
	id := binary.BigEndian.Uint32(loc[0:])
	off := int64(binary.BigEndian.Uint64(loc[4:]))
	n := int(loc[12]) << 3
	if cap(buf) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	_, err := s.files[id].ReadAt(buf, off)
	return buf, err
}

// endOfBlock makes appended data readable (flush; real impl fsyncs per major block).
func (s *blockStore) endOfBlock() error { return s.w.Flush() }

func (s *blockStore) diskBytes() int64 {
	var total int64
	ents, _ := os.ReadDir(s.dir)
	for _, e := range ents {
		if i, err := e.Info(); err == nil {
			total += i.Size()
		}
	}
	return total
}

func openLevelDB(path string) (*leveldb.DB, error) {
	return leveldb.OpenFile(path, &opt.Options{
		Filter:                 filter.NewBloomFilter(10),
		BlockCacheCapacity:     *cacheMB * opt.MiB,
		WriteBuffer:            *writeMB * opt.MiB,
		OpenFilesCacheCapacity: 512,
		DisableBufferPool:      true,
	})
}

func dirSize(path string) int64 {
	var total int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func main() {
	flag.Parse()
	switch {
	case *dir == "":
		fmt.Fprintln(os.Stderr, "usage: blockfile-sim -mode=MODE -dir=PATH [...]")
		os.Exit(2)
	case *mode == "real-leveldb", *mode == "real-blockdb":
		runReal(*mode)
		return
	case *mode != "leveldb" && *mode != "blockfile":
		fmt.Fprintln(os.Stderr, "modes: leveldb, blockfile, real-leveldb, real-blockdb")
		os.Exit(2)
	}
	os.RemoveAll(*dir)
	db, err := openLevelDB(filepath.Join(*dir, "leveldb"))
	check(err)
	var bs *blockStore
	if *mode == "blockfile" {
		bs, err = newBlockStore(filepath.Join(*dir, "blocks"))
		check(err)
	}

	// Shared payload buffer — content doesn't matter, bytes written do.
	payload := make([]byte, 4096)
	r := &rng{s: 42}
	for i := range payload {
		payload[i] = byte(r.next())
	}

	var (
		userBytes    int64 // logical bytes the application asked to store
		peakHeap     uint64
		readImmNs    int64
		readImmCount int64
		readBptNs    int64
		readBptCount int64
		readBuf      []byte
		ms           runtime.MemStats
	)
	bptWritten := make([]bool, *bptKeys) // read only keys that exist

	start := time.Now()
	blocks := *txs / *blockTxs
	for b := 0; b < blocks; b++ {
		batch := new(leveldb.Batch)
		for t := 0; t < *blockTxs; t++ {
			tx := b**blockTxs + t
			// Immutable entries
			for slot, size := range immSizes {
				k := immKey(tx, slot)
				val := payload[:size]
				userBytes += int64(size)
				if bs == nil {
					batch.Put(k[:], val)
				} else {
					loc, err := bs.append(val)
					check(err)
					batch.Put(k[:], loc[:])
				}
			}
			// Mutable BPT churn — identical in both modes
			for j := 0; j < bptWritesPerTx; j++ {
				i := r.intn(*bptKeys)
				bptWritten[i] = true
				k := bptKey(i)
				batch.Put(k[:], payload[:bptValSize])
				userBytes += bptValSize
			}
		}
		if bs != nil {
			check(bs.endOfBlock())
		}
		check(db.Write(batch, nil))

		// Read mix: historical immutable + random BPT, once per tx in block
		maxTx := (b + 1) * *blockTxs
		for t := 0; t < *blockTxs; t++ {
			for j := 0; j < *readsImm; j++ {
				k := immKey(r.intn(maxTx), r.intn(len(immSizes)))
				t0 := time.Now()
				v, err := db.Get(k[:], nil)
				if err == nil && bs != nil {
					readBuf, err = bs.read(v, readBuf)
				}
				check(err)
				readImmNs += time.Since(t0).Nanoseconds()
				readImmCount++
			}
			for j := 0; j < *readsBpt; j++ {
				i := r.intn(*bptKeys)
				if !bptWritten[i] {
					continue
				}
				k := bptKey(i)
				t0 := time.Now()
				_, err := db.Get(k[:], nil)
				check(err)
				readBptNs += time.Since(t0).Nanoseconds()
				readBptCount++
			}
		}

		if b%50 == 0 || b == blocks-1 {
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peakHeap {
				peakHeap = ms.HeapInuse
			}
			var s leveldb.DBStats
			check(db.Stats(&s))
			fmt.Printf("block %4d/%d  heapInuse=%4dMB  blockCache=%4dMB  ldbIOWrite=%5dMB  tables=%d\n",
				b+1, blocks, ms.HeapInuse>>20, s.BlockCacheSize>>20, s.IOWrite>>20, s.OpenedTablesCount)
		}
	}
	elapsed := time.Since(start)

	// Final numbers. GC first so HeapInuse approximates the live set.
	runtime.GC()
	runtime.ReadMemStats(&ms)
	var s leveldb.DBStats
	check(db.Stats(&s))
	var levelSum int64
	for _, sz := range s.LevelSizes {
		levelSum += sz
	}
	ldbDisk := dirSize(filepath.Join(*dir, "leveldb"))
	var bfDisk int64
	if bs != nil {
		check(bs.endOfBlock())
		bfDisk = bs.diskBytes()
	}

	fmt.Printf(`
=== %s%s: %d txs, %d tx/block, %.0f tx/s wall (cache %d MB, wbuf %d MB) ===
user bytes written        %6d MB
leveldb IOWrite           %6d MB   (write amp vs user: %.1fx)
leveldb IORead            %6d MB
leveldb on disk           %6d MB   (levels: %d MB, tables open: %d)
block files on disk       %6d MB
block cache in use        %6d MB   (cap 256)
heap in use (post-GC)     %6d MB   (peak during run: %d MB)
mem+L0+L1+ compactions    %d / %d / %d   write-delay: %s
read imm  p_avg           %6.1f µs  (n=%d)
read bpt  p_avg           %6.1f µs  (n=%d)
`,
		*mode, *label, *txs, *blockTxs, float64(*txs)/elapsed.Seconds(), *cacheMB, *writeMB,
		userBytes>>20,
		int64(s.IOWrite)>>20, float64(s.IOWrite)/float64(userBytes),
		int64(s.IORead)>>20,
		ldbDisk>>20, levelSum>>20, s.OpenedTablesCount,
		bfDisk>>20,
		s.BlockCacheSize>>20,
		ms.HeapInuse>>20, peakHeap>>20,
		s.MemComp, s.Level0Comp, s.NonLevel0Comp, s.WriteDelayDuration,
		float64(readImmNs)/1e3/float64(max64(readImmCount, 1)), readImmCount,
		float64(readBptNs)/1e3/float64(max64(readBptCount, 1)), readBptCount)

	check(db.Close())
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
