// kvbench drives one key-value backend through the keyvalue.Beginner
// interface with an Accumulate-shaped write mix, paced at a fixed
// transaction rate, and reports what it cost.  Run it once per backend
// on the same machine for a one-variable comparison, or for hours
// against one backend with the monitor on to watch memory.
//
//	go run ./test/kvbench -backend bcdb -dir /tmp/kv-bcdb -duration 4h -tps 500 -monitor :8098
//
// With -monitor the process serves a live view of itself:
//
//	/            a page that refreshes with the latest samples
//	/status      the latest sample as JSON
//	/samples.csv every sample so far
//	/debug/pprof the standard Go profiles, live
//
// and every -profile-every it writes a heap profile to -profiles, and
// immediately when RSS crosses -alert-mb, so a memory failure is
// attributable to what held it rather than a number on a graph.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// The per-transaction record mix.  Shapes follow the bcdb adapter's
// classification (route.go) so that BlockchainDB routes them the way a
// node's writes are routed; leveldb does not care about shape.
//
//	write-once: Transaction(H).Main, Message(H).Main,
//	            <chain>.Element(I), <chain>.ElementIndex(H)  x2 chains
//	mutable:    Account(U).Main, <chain>.Head x2, Transaction(H).Status,
//	            BPT node x3
const recordsPerTx = 13

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(p*float64(len(s)-1) + 0.5)
	return s[i]
}

// sample is one row of the monitor: what the process and the store
// looked like at one moment
type sample struct {
	At          time.Time `json:"at"`
	Elapsed     float64   `json:"elapsed_s"`
	Blocks      int       `json:"blocks"`
	Tx          uint64    `json:"tx"`
	RSSMB       float64   `json:"rss_mb"`
	HeapInuseMB float64   `json:"heap_inuse_mb"`
	HeapAllocMB float64   `json:"heap_alloc_mb"`
	SysMB       float64   `json:"sys_mb"`
	GCCPUPct    float64   `json:"gc_cpu_pct"`
	NumGC       uint32    `json:"num_gc"`
	Goroutines  int       `json:"goroutines"`
	CommitP50   float64   `json:"commit_p50_ms"`
	CommitP99   float64   `json:"commit_p99_ms"`
	CommitMax   float64   `json:"commit_max_ms"`
	Overruns    int       `json:"overruns"`
	DiskMB      float64   `json:"disk_mb"`
	Files       int64     `json:"files"`
	OpenFDs     int       `json:"open_fds"`
	LastProfile string    `json:"last_profile"`
	Alert       string    `json:"alert,omitempty"`
}

func (s sample) csv() string {
	return fmt.Sprintf("%s,%.0f,%d,%d,%.1f,%.1f,%.1f,%.1f,%.2f,%d,%d,%.2f,%.2f,%.2f,%d,%.1f,%d,%d,%s",
		s.At.UTC().Format(time.RFC3339), s.Elapsed, s.Blocks, s.Tx, s.RSSMB, s.HeapInuseMB, s.HeapAllocMB, s.SysMB,
		s.GCCPUPct, s.NumGC, s.Goroutines, s.CommitP50, s.CommitP99, s.CommitMax, s.Overruns, s.DiskMB, s.Files, s.OpenFDs, s.Alert)
}

const csvHeader = "time,elapsed_s,blocks,tx,rss_mb,heap_inuse_mb,heap_alloc_mb,sys_mb,gc_cpu_pct,num_gc,goroutines,commit_p50_ms,commit_p99_ms,commit_max_ms,overruns,disk_mb,files,open_fds,alert"

// monitor collects samples, serves them, and writes profiles
type monitor struct {
	mu       sync.Mutex
	samples  []sample
	dir      string // Database directory, for disk/file counts
	profiles string // Where heap profiles go
	alertMB  float64
	alerted  bool
	lastProf string
	start    time.Time

	// Fed by the driver
	blocks       int
	tx           uint64
	overruns     int
	recentCommit []float64 // Commit ms since the last sample
}

func readRSSMB() float64 {
	st, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(st), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			var kb float64
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:")), "%f", &kb)
			return kb / 1024
		}
	}
	return 0
}

func countFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func (m *monitor) diskUsage() (mb float64, files int64) {
	var bytes int64
	filepath.Walk(m.dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return float64(bytes) / 1024 / 1024, files
}

func (m *monitor) writeHeapProfile(tag string) string {
	name := filepath.Join(m.profiles, fmt.Sprintf("%s-%s.heap.pb.gz", time.Now().UTC().Format("20060102T150405Z"), tag))
	f, err := os.Create(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	runtime.GC() // A heap profile is of live objects as of the last GC
	if err := pprof.WriteHeapProfile(f); err != nil {
		return ""
	}
	return name
}

// take records one sample.  Every sample also decides whether to alert:
// RSS over the line writes a heap profile at once, so the profile shows
// what held the memory at the moment it was a problem.
func (m *monitor) take() sample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.mu.Lock()
	commits := m.recentCommit
	m.recentCommit = nil
	blocks, tx, overruns := m.blocks, m.tx, m.overruns
	m.mu.Unlock()
	disk, files := m.diskUsage()
	s := sample{
		At: time.Now(), Elapsed: time.Since(m.start).Seconds(), Blocks: blocks, Tx: tx,
		RSSMB: readRSSMB(), HeapInuseMB: float64(ms.HeapInuse) / (1 << 20), HeapAllocMB: float64(ms.HeapAlloc) / (1 << 20),
		SysMB: float64(ms.Sys) / (1 << 20), GCCPUPct: 100 * ms.GCCPUFraction, NumGC: ms.NumGC, Goroutines: runtime.NumGoroutine(),
		CommitP50: percentile(commits, 0.5), CommitP99: percentile(commits, 0.99), CommitMax: percentile(commits, 1),
		Overruns: overruns, DiskMB: disk, Files: files, OpenFDs: countFDs(), LastProfile: filepath.Base(m.lastProf),
	}
	if m.alertMB > 0 && s.RSSMB > m.alertMB && !m.alerted {
		m.alerted = true
		p := m.writeHeapProfile("ALERT")
		s.Alert = fmt.Sprintf("RSS %.0f MB over %.0f MB; heap profile %s", s.RSSMB, m.alertMB, filepath.Base(p))
		fmt.Printf("\n*** ALERT %s ***\n\n", s.Alert)
	} else if m.alerted && s.RSSMB < 0.8*m.alertMB {
		m.alerted = false // Re-arm once it comes back down
	}
	m.mu.Lock()
	m.samples = append(m.samples, s)
	m.mu.Unlock()
	return s
}

func (m *monitor) serve(addr string) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		var s sample
		if n := len(m.samples); n > 0 {
			s = m.samples[n-1]
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	})
	http.HandleFunc("/samples.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprintln(w, csvHeader)
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, s := range m.samples {
			fmt.Fprintln(w, s.csv())
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		rows := m.samples
		if len(rows) > 60 {
			rows = rows[len(rows)-60:]
		}
		last := sample{}
		if len(m.samples) > 0 {
			last = m.samples[len(m.samples)-1]
		}
		var peak float64
		for _, s := range m.samples {
			if s.RSSMB > peak {
				peak = s.RSSMB
			}
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<meta http-equiv="refresh" content="5"><style>body{font:14px monospace;padding:1em}table{border-collapse:collapse}td,th{padding:2px 8px;text-align:right;border-bottom:1px solid #ddd}th{text-align:right}.a{color:#b00;font-weight:bold}</style>`)
		fmt.Fprintf(w, `<h2>kvbench monitor</h2><p>elapsed <b>%.0f min</b> &middot; blocks <b>%d</b> &middot; tx <b>%d</b> &middot; RSS <b>%.0f MB</b> (peak %.0f) &middot; heap in use <b>%.0f MB</b> &middot; GC <b>%.1f%%</b> of CPU &middot; files <b>%d</b> &middot; fds <b>%d</b> &middot; last profile %s</p>`,
			last.Elapsed/60, last.Blocks, last.Tx, last.RSSMB, peak, last.HeapInuseMB, last.GCCPUPct, last.Files, last.OpenFDs, last.LastProfile)
		if last.Alert != "" {
			fmt.Fprintf(w, `<p class=a>ALERT: %s</p>`, last.Alert)
		}
		fmt.Fprintf(w, `<p><a href="/samples.csv">samples.csv</a> &middot; <a href="/debug/pprof/">pprof</a> &middot; <a href="/status">status.json</a></p><table><tr><th>time</th><th>min</th><th>blocks</th><th>RSS MB</th><th>heap MB</th><th>sys MB</th><th>GC %%</th><th>GCs</th><th>commit p50</th><th>p99</th><th>max ms</th><th>overruns</th><th>disk MB</th><th>files</th><th>fds</th><th>alert</th></tr>`)
		for i := len(rows) - 1; i >= 0; i-- {
			s := rows[i]
			cls := ""
			if s.Alert != "" {
				cls = " class=a"
			}
			fmt.Fprintf(w, `<tr%s><td>%s</td><td>%.1f</td><td>%d</td><td>%.0f</td><td>%.0f</td><td>%.0f</td><td>%.1f</td><td>%d</td><td>%.1f</td><td>%.1f</td><td>%.0f</td><td>%d</td><td>%.0f</td><td>%d</td><td>%d</td><td>%s</td></tr>`,
				cls, s.At.UTC().Format("15:04:05"), s.Elapsed/60, s.Blocks, s.RSSMB, s.HeapInuseMB, s.SysMB, s.GCCPUPct, s.NumGC, s.CommitP50, s.CommitP99, s.CommitMax, s.Overruns, s.DiskMB, s.Files, s.OpenFDs, s.Alert)
		}
		fmt.Fprint(w, "</table>")
	})
	go http.ListenAndServe(addr, nil)
}

func main() {
	backend := flag.String("backend", "", "leveldb or bcdb")
	dir := flag.String("dir", "", "database directory (created fresh)")
	duration := flag.Duration("duration", 20*time.Minute, "how long to run")
	tps := flag.Int("tps", 100, "transactions per second")
	block := flag.Duration("block", time.Second, "commit interval")
	accounts := flag.Int("accounts", 20000, "hot account set")
	bptNodes := flag.Int("bpt", 50000, "hot BPT node set")
	out := flag.String("out", "", "write results JSON here")
	monitorAddr := flag.String("monitor", "", "serve the monitor on this address, e.g. :8098")
	sampleEvery := flag.Duration("sample", 5*time.Second, "monitor sample interval")
	profileEvery := flag.Duration("profile-every", 5*time.Minute, "write a heap profile this often")
	profiles := flag.String("profiles", "", "directory for heap profiles (default <dir>-profiles)")
	alertMB := flag.Float64("alert-mb", 2000, "write a heap profile at once when RSS exceeds this")
	flag.Parse()
	if *backend == "" || *dir == "" {
		flag.Usage()
		os.Exit(2)
	}
	os.RemoveAll(*dir)
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		panic(err)
	}
	if *profiles == "" {
		*profiles = *dir + "-profiles"
	}
	os.MkdirAll(*profiles, 0o755)

	var db keyvalue.Beginner
	var closer interface{ Close() error }
	switch *backend {
	case "leveldb":
		d, err := leveldb.Open(*dir)
		if err != nil {
			panic(err)
		}
		db, closer = d, d
	case "bcdb":
		d, err := bcdb.Open(filepath.Join(*dir, "db"))
		if err != nil {
			panic(err)
		}
		db, closer = d, d
	default:
		panic("unknown backend " + *backend)
	}

	mon := &monitor{dir: *dir, profiles: *profiles, alertMB: *alertMB, start: time.Now()}
	if *monitorAddr != "" {
		mon.serve(*monitorAddr)
		fmt.Printf("monitor: http://%s/  (pprof at /debug/pprof)\n", strings.TrimPrefix(*monitorAddr, ":"))
	}
	stopMon := make(chan struct{})
	var monWG sync.WaitGroup
	monWG.Add(1)
	go func() {
		defer monWG.Done()
		tick := time.NewTicker(*sampleEvery)
		prof := time.NewTicker(*profileEvery)
		defer tick.Stop()
		defer prof.Stop()
		for {
			select {
			case <-stopMon:
				return
			case <-prof.C:
				mon.mu.Lock()
				mon.lastProf = mon.writeHeapProfile("periodic")
				mon.mu.Unlock()
			case <-tick.C:
				mon.take()
			}
		}
	}()

	rng := rand.New(rand.NewSource(1))
	accountURLs := make([]*url.URL, *accounts)
	for i := range accountURLs {
		accountURLs[i] = url.MustParse(fmt.Sprintf("acc://account-%06d.acme", i))
	}
	elementCount := make([]uint64, *accounts) // Per-account chain height
	valueBuf := make([]byte, 512)
	rng.Read(valueBuf)
	val := func(n int, seq uint64) []byte {
		b := make([]byte, n)
		copy(b, valueBuf)
		binary.BigEndian.PutUint64(b, seq) // Distinct per write, so dyna overwrites are real
		return b
	}

	type written struct {
		key *record.Key
		at  time.Time
	}
	var history []written // A sample of write-once keys, for the read probe

	var commitMs []float64
	var overruns int
	var recentReadMs, oldReadMs []float64
	var txDone uint64
	txPerBlock := int(float64(*tps) * block.Seconds())

	start := time.Now()
	deadline := start.Add(*duration)
	nextBlock := start
	nextProbe := start.Add(10 * time.Second)
	var seq uint64
	blocks := 0
	fmt.Printf("%s: %d tx/block every %s for %s (%d records/tx)\n", *backend, txPerBlock, *block, *duration, recordsPerTx)

	for time.Now().Before(deadline) {
		if wait := time.Until(nextBlock); wait > 0 {
			time.Sleep(wait)
		} else if wait < -*block {
			overruns++ // A commit took longer than the block interval
		}
		nextBlock = nextBlock.Add(*block)

		batch := db.Begin(nil, true)
		now := time.Now()
		for t := 0; t < txPerBlock; t++ {
			seq++
			a := rng.Intn(*accounts)
			u := accountURLs[a]
			var h [32]byte
			binary.BigEndian.PutUint64(h[:], seq)
			h = sha256.Sum256(h[:])
			elementCount[a]++
			i := elementCount[a]

			put := func(k *record.Key, v []byte, once bool) {
				if err := batch.Put(k, v); err != nil {
					panic(err)
				}
				if once && seq%97 == 0 { // Keep a sample of history for the probe
					history = append(history, written{k, now})
				}
			}
			put(record.NewKey("Transaction", h, "Main"), val(300, seq), true)
			put(record.NewKey("Message", h, "Main"), val(250, seq), true)
			put(record.NewKey("Account", u, "MainChain", "Element", i), val(32, seq), true)
			put(record.NewKey("Account", u, "MainChain", "ElementIndex", h), val(8, seq), true)
			put(record.NewKey("Account", u, "SignatureChain", "Element", i), val(32, seq), true)
			put(record.NewKey("Account", u, "SignatureChain", "ElementIndex", h), val(8, seq), true)
			put(record.NewKey("Account", u, "Main"), val(400, seq), false)
			put(record.NewKey("Account", u, "MainChain", "Head"), val(100, seq), false)
			put(record.NewKey("Account", u, "SignatureChain", "Head"), val(100, seq), false)
			put(record.NewKey("Transaction", h, "Status"), val(150, seq), false)
			for j := 0; j < 3; j++ {
				put(record.NewKey("BPT", uint64(rng.Intn(*bptNodes))), val(200, seq), false)
			}
			txDone++
		}
		c0 := time.Now()
		if err := batch.Commit(); err != nil {
			panic(fmt.Sprintf("commit %d: %v", blocks, err))
		}
		ms := float64(time.Since(c0).Microseconds()) / 1000
		commitMs = append(commitMs, ms)
		blocks++
		mon.mu.Lock()
		mon.blocks, mon.tx, mon.overruns = blocks, txDone, overruns
		mon.recentCommit = append(mon.recentCommit, ms)
		mon.mu.Unlock()

		if time.Now().After(nextProbe) && len(history) > 100 {
			nextProbe = time.Now().Add(10 * time.Second)
			rb := db.Begin(nil, false)
			cut := time.Now().Add(-time.Minute)
			firstRecent := sort.Search(len(history), func(i int) bool { return history[i].at.After(cut) })
			for n := 0; n < 25; n++ {
				if firstRecent < len(history) {
					w := history[firstRecent+rng.Intn(len(history)-firstRecent)]
					r0 := time.Now()
					if _, err := rb.Get(w.key); err != nil {
						panic("recent read: " + err.Error())
					}
					recentReadMs = append(recentReadMs, float64(time.Since(r0).Microseconds())/1000)
				}
				w := history[rng.Intn(len(history))]
				r0 := time.Now()
				if _, err := rb.Get(w.key); err != nil {
					panic("old read: " + err.Error())
				}
				oldReadMs = append(oldReadMs, float64(time.Since(r0).Microseconds())/1000)
			}
			rb.Discard()
		}
		if blocks%60 == 0 {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Printf("  %6.1f min  blocks=%d tx=%d commit p50=%.1fms p99=%.1fms max=%.0fms overruns=%d  rss=%.0fMB heap=%.0fMB gc=%.1f%%\n",
				time.Since(start).Minutes(), blocks, txDone,
				percentile(commitMs[len(commitMs)-60:], 0.5), percentile(commitMs[len(commitMs)-60:], 0.99), percentile(commitMs[len(commitMs)-60:], 1),
				overruns, readRSSMB(), float64(ms.HeapInuse)/(1<<20), 100*ms.GCCPUFraction)
		}
	}
	elapsed := time.Since(start)
	closeStart := time.Now()
	if err := closer.Close(); err != nil {
		panic(err)
	}
	closeMs := float64(time.Since(closeStart).Milliseconds())
	close(stopMon)
	monWG.Wait()

	disk, files := mon.diskUsage()
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	cpu := float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 + float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
	var hwmKB int64
	if st, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(st), "\n") {
			if strings.HasPrefix(line, "VmHWM:") {
				fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "VmHWM:")), "%d", &hwmKB)
			}
		}
	}
	var busy float64
	for _, m := range commitMs {
		busy += m
	}
	res := map[string]any{
		"backend": *backend, "duration_s": elapsed.Seconds(), "tps_target": *tps,
		"tps_achieved": float64(txDone) / elapsed.Seconds(), "blocks": blocks, "tx": txDone,
		"records": txDone * recordsPerTx, "overruns": overruns,
		"commit_ms_p50": percentile(commitMs, 0.5), "commit_ms_p95": percentile(commitMs, 0.95),
		"commit_ms_p99": percentile(commitMs, 0.99), "commit_ms_max": percentile(commitMs, 1),
		"commit_busy_pct":    100 * busy / 1000 / elapsed.Seconds(),
		"read_recent_ms_p50": percentile(recentReadMs, 0.5), "read_recent_ms_p99": percentile(recentReadMs, 0.99),
		"read_old_ms_p50": percentile(oldReadMs, 0.5), "read_old_ms_p99": percentile(oldReadMs, 0.99),
		"reads": len(recentReadMs) + len(oldReadMs),
		"cpu_s": cpu, "cpu_pct_of_wall": 100 * cpu / elapsed.Seconds(),
		"rss_peak_mb": float64(hwmKB) / 1024, "disk_mb": disk, "files": files,
		"close_ms": closeMs,
	}
	fmt.Println()
	keys := make([]string, 0, len(res))
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-22s %v\n", k, res[k])
	}
	if *out != "" {
		j, _ := json.MarshalIndent(res, "", "  ")
		os.WriteFile(*out, j, 0o644)
	}
	if *monitorAddr != "" {
		mon.mu.Lock()
		f, err := os.Create(filepath.Join(*profiles, "samples.csv"))
		if err == nil {
			fmt.Fprintln(f, csvHeader)
			for _, s := range mon.samples {
				fmt.Fprintln(f, s.csv())
			}
			f.Close()
		}
		mon.mu.Unlock()
	}
}
