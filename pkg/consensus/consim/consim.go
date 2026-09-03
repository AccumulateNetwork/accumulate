// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package consim is a small, self-contained consensus simulator: the full
// DAG-BFT stack (workers, primary, DAG, Bullshark, gossip over real
// in-process libp2p) in the SOAK'S TOPOLOGY — every validator hosts a
// Directory node AND its BVN node on one shared libp2p host — but with no
// Docker, no executor, no database. Rounds are paced in milliseconds, so a
// height that takes the soak half an hour takes consim seconds.
//
// Its purpose is analysis, not load: it watches every stage of the pipeline
// (rounds advancing → headers created → votes flowing → certificates formed
// → leaders committed → certificates executed) and, when progress stops, it
// reports WHICH stage stopped, per node — turning "the network stalled" into
// "assumption X broke at stage Y". See #4159: the soak froze at the same
// height for days of engineering because the 12-container reproduction took
// 30 minutes per attempt and pointed at nothing.
package consim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Config shapes a simulation. The zero value is unusable; call Defaults.
type Config struct {
	BVNs             int           // BVN partitions
	ValidatorsPerBVN int           // validators per BVN; all validators also serve the Directory
	TPS              int           // transactions per second submitted to EACH partition
	MinRoundInterval time.Duration // consensus round pacing (soak: 500ms; sim: milliseconds)
	BatchTimeout     time.Duration // worker batch cut interval
	BatchSize        int           // transactions per batch before an early cut
	TargetHeight     uint64        // stop with success once every partition executes this height
	Duration         time.Duration // hard cap on the run
	StallAfter       time.Duration // no executed-height progress on some partition for this long = stall
	BatchCollect     time.Duration // CollectBatches bound (0 = library default)
	Out              io.Writer     // status output; nil = silent

	// NumWorkers per node. The soak runs 4 (cmd_init_network sets it), and the
	// batch-store byte budget is divided among them, so a sim on 1 worker is
	// not exercising the store sizing the real network has.
	NumWorkers int

	// ExecCostPerTx makes execution cost wall time, per transaction in the
	// block. consim's executor is otherwise free, which is why a partition
	// absorbs 25x its peers' load while lagging only ~10%: the load creates no
	// work. The real BVN2 must EXECUTE what it takes in and write it down, and
	// that is the variable the Docker wedge turns on.
	//
	// ExecCostByPartition overrides it per partition.
	ExecCostPerTx       time.Duration
	ExecCostByPartition map[string]time.Duration

	// MaxStoredBatches, MaxPendingCount and MaxPendingSize size the worker's
	// batch store and pending queue.  Zero uses the worker's defaults.
	//
	// A test that wants to ask what happens when the store FILLS has to be
	// able to fill it, and the store's real default (1000 batches) takes a
	// soak's throughput and several minutes to reach.  Shrinking it is what
	// turns "run the network for 20 minutes and see" into a second.
	MaxStoredBatches int
	MaxPendingCount  int
	MaxPendingSize   int

	// TPSByPartition overrides TPS for named partitions ("Directory", "BVN1",
	// ...). Real load is not spread evenly: in soak 20260831T070855Z, BVN2
	// produced 80,991 synthetics to BVN1's 24,608 and 19,429 to the
	// Directory's 1,313, carried twice the database, and was the partition
	// that wedged while BVN1 held 3.0 s/block. Uniform load cannot express
	// that, and therefore cannot ask whether an overloaded partition stops or
	// merely lags.
	TPSByPartition map[string]int
}

// execCost reports the per-transaction execution cost for one partition.
func (c *Config) execCost(part string) time.Duration {
	if d, ok := c.ExecCostByPartition[part]; ok {
		return d
	}
	return c.ExecCostPerTx
}

// tps reports the submission rate for one partition.
func (c *Config) tps(part string) int {
	if n, ok := c.TPSByPartition[part]; ok {
		return n
	}
	return c.TPS
}

// Defaults fills unset fields with values scaled for fast in-process runs.
func (c *Config) Defaults() {
	if c.BVNs == 0 {
		c.BVNs = 3
	}
	if c.ValidatorsPerBVN == 0 {
		c.ValidatorsPerBVN = 4
	}
	if c.TPS == 0 {
		c.TPS = 20
	}
	if c.MinRoundInterval == 0 {
		c.MinRoundInterval = 5 * time.Millisecond
	}
	if c.BatchTimeout == 0 {
		c.BatchTimeout = 10 * time.Millisecond
	}
	if c.BatchSize == 0 {
		c.BatchSize = 20
	}
	if c.NumWorkers <= 0 {
		c.NumWorkers = 4 // what cmd_init_network generates
	}
	if c.Duration == 0 {
		c.Duration = 5 * time.Minute
	}
	if c.StallAfter == 0 {
		c.StallAfter = 20 * time.Second
	}
	if c.BatchCollect == 0 {
		c.BatchCollect = 30 * time.Second
	}
}

// simNode is one consensus.Node plus the minimal executor-side consumer the
// real service runs: read Committed(), collect batches, "execute" (count),
// prune. Faithful to internal/node/dagbft/service.go's essential loop.
type simNode struct {
	part   string
	val    int
	node   *consensus.Node
	height atomic.Uint64
	txs    atomic.Uint64
	fatal  atomic.Value // error that stopped this node's consumer, if any
}

// Sim is a running simulation.
type Sim struct {
	cfg    Config
	hosts  []host.Host
	nodes  []*simNode // all nodes, all partitions
	byPart map[string][]*simNode
	parts  []string // stable order: Directory, BVN1..N
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// submitted and refused count what the load offered and what the network
	// turned away.  A refusal is a stage of the pipeline like any other, and
	// the only one that leaves every downstream gauge looking healthy.
	submitted   atomic.Uint64
	refused     atomic.Uint64
	refusedOnce sync.Once
}

// logf writes to the configured output, if any.
func (s *Sim) logf(format string, args ...any) {
	if s.cfg.Out != nil {
		fmt.Fprintf(s.cfg.Out, format+"\n", args...)
	}
}

// Submitted and Refused report the load offered and the load turned away.
func (s *Sim) Submitted() uint64 { return s.submitted.Load() }
func (s *Sim) Refused() uint64   { return s.refused.Load() }

// Result reports how a run ended.
type Result struct {
	Ok      bool
	Reason  string // "target height", "duration elapsed", "stalled: ..."
	Heights map[string]uint64
	Elapsed time.Duration

	// Submitted and Refused: what the load offered, and what the network
	// turned away before it reached consensus.  Refused > 0 is a finding on
	// its own -- the network declined work it was healthy enough to take.
	Submitted uint64
	Refused   uint64
}

// ErrStalled is returned (wrapped) when a partition stops executing.
var ErrStalled = errors.New("partition stalled")

// New builds the network: one libp2p host per validator, a Directory node on
// every host, and a BVN node on each host for its validator's BVN — the
// soak's dual topology, where all partitions share each validator's host,
// pubsub, and process.
func New(cfg Config) (*Sim, error) {
	cfg.Defaults()
	nVals := cfg.BVNs * cfg.ValidatorsPerBVN

	s := &Sim{cfg: cfg, byPart: map[string][]*simNode{}}
	s.parts = append(s.parts, "Directory")
	for i := 1; i <= cfg.BVNs; i++ {
		s.parts = append(s.parts, fmt.Sprintf("BVN%d", i))
	}

	// Keys and committees. Every validator is a Directory validator; each
	// BVN's committee is its own ValidatorsPerBVN keys.
	keys := make([]ed25519.PrivateKey, nVals)
	dirVals := make([]types.ValidatorInfo, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		keys[i] = priv
		dirVals[i] = types.ValidatorInfo{PublicKey: pub, Stake: 100}
	}

	// Hosts: real libp2p on loopback, fully connected. Real GossipSub is the
	// point — mesh formation, scoring, and per-topic behaviour are part of
	// the system under study.
	s.hosts = make([]host.Host, nVals)
	for i := range s.hosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("host %d: %w", i, err)
		}
		s.hosts[i] = h
	}
	ctx := context.Background()
	for i := range s.hosts {
		for j := i + 1; j < len(s.hosts); j++ {
			err := s.hosts[i].Connect(ctx, peer.AddrInfo{ID: s.hosts[j].ID(), Addrs: s.hosts[j].Addrs()})
			if err != nil {
				s.Close()
				return nil, fmt.Errorf("connect %d<->%d: %w", i, j, err)
			}
		}
	}

	// One GossipSub per host, shared by that validator's Directory and BVN
	// nodes — as in production.
	pss := make([]*pubsub.PubSub, nVals)
	for i, h := range s.hosts {
		ps, err := pubsub.NewGossipSub(ctx, h)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("pubsub %d: %w", i, err)
		}
		pss[i] = ps
	}

	mkNode := func(part string, committee *types.Committee, val int) (*simNode, error) {
		n, err := consensus.NewNode(consensus.NodeConfig{
			Partition:  part,
			KeyPair:    keys[val],
			NumWorkers: cfg.NumWorkers,
			WorkerConfig: worker.Config{
				Partition:        part,
				BatchSize:        cfg.BatchSize,
				BatchTimeout:     cfg.BatchTimeout,
				MaxStoredBatches: cfg.MaxStoredBatches,
				MaxPendingCount:  cfg.MaxPendingCount,
				MaxPendingSize:   cfg.MaxPendingSize,
			},
			MinRoundInterval:    cfg.MinRoundInterval,
			BatchCollectTimeout: cfg.BatchCollect,
		}, committee, s.hosts[val], pss[val])
		if err != nil {
			return nil, err
		}
		sn := &simNode{part: part, val: val, node: n}
		s.nodes = append(s.nodes, sn)
		s.byPart[part] = append(s.byPart[part], sn)
		return sn, nil
	}

	dirCommittee := types.NewCommittee(dirVals, 1)
	for v := 0; v < nVals; v++ {
		if _, err := mkNode("Directory", dirCommittee, v); err != nil {
			s.Close()
			return nil, err
		}
	}
	for b := 0; b < cfg.BVNs; b++ {
		part := fmt.Sprintf("BVN%d", b+1)
		vals := make([]types.ValidatorInfo, cfg.ValidatorsPerBVN)
		for k := 0; k < cfg.ValidatorsPerBVN; k++ {
			vals[k] = dirVals[b*cfg.ValidatorsPerBVN+k]
		}
		committee := types.NewCommittee(vals, 1)
		for k := 0; k < cfg.ValidatorsPerBVN; k++ {
			if _, err := mkNode(part, committee, b*cfg.ValidatorsPerBVN+k); err != nil {
				s.Close()
				return nil, err
			}
		}
	}

	// Genesis per partition.
	for _, part := range s.parts {
		nodes := s.byPart[part]
		pkeys := make([]ed25519.PrivateKey, len(nodes))
		for i, sn := range nodes {
			pkeys[i] = keys[sn.val]
		}
		for _, sn := range nodes {
			if err := sn.node.InsertGenesisForAll(pkeys); err != nil {
				s.Close()
				return nil, fmt.Errorf("genesis %s: %w", part, err)
			}
		}
	}
	return s, nil
}

// Close releases hosts. Safe after a failed New.
func (s *Sim) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	for _, sn := range s.nodes {
		if sn != nil && sn.node != nil {
			sn.node.Stop()
		}
	}
	for _, h := range s.hosts {
		if h != nil {
			_ = h.Close()
		}
	}
}

// consume is the executor stand-in: exactly the real service's essential
// loop — collect the certificate's batches, "execute" them, prune them.
func (s *Sim) consume(ctx context.Context, sn *simNode) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case group, ok := <-sn.node.Committed():
			if !ok {
				return
			}
			if len(group) == 0 {
				continue
			}
			// One committed leader group = one block, mirroring the real
			// service (#4164).
			executedAny := false
			for _, cert := range group {
				batches, err := sn.node.CollectBatches(ctx, cert)
				if errors.Is(err, consensus.ErrAlreadyExecuted) {
					continue
				}
				if err != nil {
					if ctx.Err() == nil {
						sn.fatal.Store(err)
					}
					return
				}
				executedAny = true
				digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
				for _, e := range cert.Header.Payload {
					digests = append(digests, e.Digest)
				}
				n := 0
				for _, b := range batches {
					n += len(b.Transactions)
				}
				sn.txs.Add(uint64(n))
				if cost := s.cfg.execCost(sn.part); cost > 0 && n > 0 {
					// Charge for the work on the goroutine that reads
					// Committed() — where the real service pays it, and
					// therefore where back-pressure into consensus would show
					// up if it shows up at all.
					t := time.NewTimer(time.Duration(n) * cost)
					select {
					case <-ctx.Done():
						t.Stop()
						return
					case <-t.C:
					}
				}
				for _, w := range sn.node.Workers() {
					w.PruneCommitted(digests, worker.CommitInfo{Detail: fmt.Sprintf("block %d", sn.height.Load()+1), Cert: cert.Digest().String()})
				}
			}
			if executedAny {
				sn.height.Add(1)
			}
		}
	}
}

// load submits cfg.TPS unique transactions per second to a partition,
// round-robin across its nodes, mirroring the soak's per-partition load.
func (s *Sim) load(ctx context.Context, part string) {
	defer s.wg.Done()
	nodes := s.byPart[part]
	tps := s.cfg.tps(part)
	if tps <= 0 || len(nodes) == 0 {
		return
	}
	interval := time.Second / time.Duration(tps)
	if interval <= 0 {
		interval = time.Millisecond
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var n uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			n++
			tx := []byte(fmt.Sprintf("consim-%s-%d", part, n))

			// COUNT the refusals; do not discard them.
			//
			// This used to be `_ =` with a comment saying backpressure is
			// part of the system under study -- and that is exactly why the
			// simulator could not see the failure it was built to find.  A
			// network that REFUSES work is not a network that is merely slow:
			// the refused transaction never enters the pipeline, so every
			// stage gauge downstream looks healthy while nothing progresses.
			// In soak 20260903T035139Z that shape wedged a stream for its
			// whole life -- BVN2 produced 46,428 messages for BVN1, BVN1
			// received 10,378, both frozen -- while blocks were being
			// produced at 6/s the entire time.
			//
			// Keep submitting after a refusal: the question is whether the
			// network recovers, not whether the load backs off.
			s.submitted.Add(1)
			if err := nodes[int(n)%len(nodes)].node.SubmitTransaction(tx); err != nil {
				s.refused.Add(1)
				s.refusedOnce.Do(func() {
					s.logf("REFUSED: %s refused a submission: %v", part, err)
				})
			}
		}
	}
}

// snapshot is one node's pipeline gauges at an instant.
type snapshot struct {
	height, txs                       uint64
	round, lastCommit                 types.Round
	headers, certs, votesIn, votesOut uint64
}

func (sn *simNode) snap() snapshot {
	h, c, vi, vo := sn.node.Primary().Metrics()
	return snapshot{
		height:     sn.height.Load(),
		txs:        sn.txs.Load(),
		round:      sn.node.CurrentRound(),
		lastCommit: sn.node.LastCommitRound(),
		headers:    h, certs: c, votesIn: vi, votesOut: vo,
	}
}

// gauges, in pipeline order. The ONSET ORDER of their freezes is the
// diagnosis: the earliest-frozen gauge is closest to the broken assumption.
var gaugeNames = []string{"round", "headers", "votesOut", "votesIn", "certs", "commit", "height"}

func (s snapshot) gauges() []uint64 {
	return []uint64{uint64(s.round), s.headers, s.votesOut, s.votesIn, s.certs, uint64(s.lastCommit), s.height}
}

// lastMoved tracks, per node, when each gauge last changed.
type lastMoved struct {
	vals [7]uint64
	at   [7]time.Time
}

func (l *lastMoved) update(now time.Time, s snapshot) {
	g := s.gauges()
	for i, v := range g {
		if v != l.vals[i] || l.at[i].IsZero() {
			l.vals[i] = v
			l.at[i] = now
		}
	}
}

// Run starts everything and blocks until the target height, the duration, or
// a stall. The returned Result says which, and on a stall the diagnosis has
// already been written to cfg.Out.
func (s *Sim) Run(parent context.Context) (*Result, error) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	start := time.Now()

	for _, sn := range s.nodes {
		sn := sn
		s.wg.Add(1)
		go func() { defer s.wg.Done(); _ = sn.node.Start(ctx) }()
		s.wg.Add(1)
		go s.consume(ctx, sn)
	}
	// Let the gossip mesh form before load; production waits too.
	time.Sleep(2 * time.Second)
	for _, part := range s.parts {
		s.wg.Add(1)
		go s.load(ctx, part)
	}

	logf := s.logf

	prev := map[*simNode]snapshot{}
	moved := map[*simNode]*lastMoved{}
	for _, sn := range s.nodes {
		moved[sn] = &lastMoved{}
	}
	lastProgress := map[string]time.Time{}
	lastHeight := map[string]uint64{}
	for _, p := range s.parts {
		lastProgress[p] = time.Now()
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	deadline := time.After(s.cfg.Duration)

	for {
		select {
		case <-parent.Done():
			return s.finish(start, false, "cancelled"), parent.Err()
		case <-deadline:
			return s.finish(start, true, "duration elapsed"), nil
		case <-tick.C:
		}

		// Per-partition status + stall detection on EXECUTED height, the
		// same signal the soak monitor uses.
		var line []string
		allAtTarget := s.cfg.TargetHeight > 0
		for _, part := range s.parts {
			maxH, maxR := uint64(0), types.Round(0)
			for _, sn := range s.byPart[part] {
				if h := sn.height.Load(); h > maxH {
					maxH = h
				}
				if r := sn.node.CurrentRound(); r > maxR {
					maxR = r
				}
			}
			if maxH > lastHeight[part] {
				lastHeight[part] = maxH
				lastProgress[part] = time.Now()
			}
			if s.cfg.TargetHeight > 0 && maxH < s.cfg.TargetHeight {
				allAtTarget = false
			}
			line = append(line, fmt.Sprintf("%s h=%d r=%d", part, maxH, maxR))

			if time.Since(lastProgress[part]) > s.cfg.StallAfter {
				logf("STALL on %s: no executed-height progress for %s", part, s.cfg.StallAfter)
				s.diagnose(logf, prev, moved)
				return s.finish(start, false, fmt.Sprintf("stalled: %s frozen at height %d", part, maxH)),
					fmt.Errorf("%w: %s at height %d", ErrStalled, part, maxH)
			}
		}
		logf("%8s  %s", time.Since(start).Truncate(time.Second), strings.Join(line, " | "))

		now := time.Now()
		for _, sn := range s.nodes {
			snap := sn.snap()
			prev[sn] = snap
			moved[sn].update(now, snap)
		}

		if allAtTarget {
			return s.finish(start, true, "target height"), nil
		}
	}
}

// diagnose prints, for every node, when each pipeline gauge LAST MOVED
// (seconds ago). The onset order across gauges and nodes is the payoff: the
// earliest freeze is nearest the broken assumption, and comparing nodes says
// whether one validator died first and starved the rest or all stopped
// together.
func (s *Sim) diagnose(logf func(string, ...any), prev map[*simNode]snapshot, moved map[*simNode]*lastMoved) {
	now := time.Now()
	logf("stage freeze ages in seconds (bigger = stopped earlier; pipeline order %v):", gaugeNames)
	logf("%-10s %-3s | %6s %8s %9s %8s %6s %7s %7s | %6s %7s %5s | fatal", "part", "val",
		"round", "headers", "votesOut", "votesIn", "certs", "commit", "height", "atRnd", "atCmt", "hdrQ")
	for _, part := range s.parts {
		for _, sn := range s.byPart[part] {
			lm := moved[sn]
			cur := sn.snap()
			ages := make([]string, 7)
			for i := range ages {
				if lm.at[i].IsZero() {
					ages[i] = "-"
				} else {
					ages[i] = fmt.Sprintf("%d", int(now.Sub(lm.at[i]).Seconds()))
				}
			}
			r := cur.round
			inRound := len(sn.node.DAG().GetRound(r)) + len(sn.node.DAG().GetRound(r-1))
			var fatal string
			if v := sn.fatal.Load(); v != nil {
				fatal = fmt.Sprint(v)
			}
			logf("%-10s %-3d | %6s %8s %9s %8s %6s %7s %7s | %6d %7d %5d | %s",
				part, sn.val,
				ages[0], ages[1], ages[2], ages[3], ages[4], ages[5], ages[6],
				cur.round, cur.lastCommit, inRound, fatal)
		}
	}
}

func (s *Sim) finish(start time.Time, ok bool, reason string) *Result {
	r := &Result{Ok: ok, Reason: reason, Heights: map[string]uint64{}, Elapsed: time.Since(start),
		Submitted: s.submitted.Load(), Refused: s.refused.Load()}
	for _, part := range s.parts {
		for _, sn := range s.byPart[part] {
			if h := sn.height.Load(); h > r.Heights[part] {
				r.Heights[part] = h
			}
		}
	}
	// Stable ordering for callers that print the map.
	sort.Strings(s.parts[1:])
	return r
}
