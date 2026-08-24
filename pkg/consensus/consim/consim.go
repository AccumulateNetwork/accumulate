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
}

// Result reports how a run ended.
type Result struct {
	Ok      bool
	Reason  string // "target height", "duration elapsed", "stalled: ..."
	Heights map[string]uint64
	Elapsed time.Duration
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
			NumWorkers: 1,
			WorkerConfig: worker.Config{
				Partition:    part,
				BatchSize:    cfg.BatchSize,
				BatchTimeout: cfg.BatchTimeout,
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
		case cert, ok := <-sn.node.Committed():
			if !ok {
				return
			}
			if cert == nil {
				continue
			}
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
			digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
			for _, e := range cert.Header.Payload {
				digests = append(digests, e.Digest)
			}
			h := sn.height.Add(1)
			for _, b := range batches {
				sn.txs.Add(uint64(len(b.Transactions)))
			}
			for _, w := range sn.node.Workers() {
				w.PruneCommitted(digests, worker.CommitInfo{Detail: fmt.Sprintf("block %d", h), Cert: cert.Digest().String()})
			}
		}
	}
}

// load submits cfg.TPS unique transactions per second to a partition,
// round-robin across its nodes, mirroring the soak's per-partition load.
func (s *Sim) load(ctx context.Context, part string) {
	defer s.wg.Done()
	nodes := s.byPart[part]
	if s.cfg.TPS <= 0 || len(nodes) == 0 {
		return
	}
	interval := time.Second / time.Duration(s.cfg.TPS)
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
			// Backpressure and transient submit errors are part of the
			// system under study; keep submitting.
			_ = nodes[int(n)%len(nodes)].node.SubmitTransaction(tx)
		}
	}
}

// snapshot is one node's pipeline gauges at an instant.
type snapshot struct {
	height, txs                     uint64
	round, lastCommit               types.Round
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

	out := s.cfg.Out
	logf := func(format string, args ...any) {
		if out != nil {
			fmt.Fprintf(out, format+"\n", args...)
		}
	}

	prev := map[*simNode]snapshot{}
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
				s.diagnose(logf, prev)
				return s.finish(start, false, fmt.Sprintf("stalled: %s frozen at height %d", part, maxH)),
					fmt.Errorf("%w: %s at height %d", ErrStalled, part, maxH)
			}
		}
		logf("%8s  %s", time.Since(start).Truncate(time.Second), strings.Join(line, " | "))

		for _, sn := range s.nodes {
			prev[sn] = sn.snap()
		}

		if allAtTarget {
			return s.finish(start, true, "target height"), nil
		}
	}
}

// diagnose prints, for every node, which pipeline stage moved in the last
// second and which did not — the payoff: a stall names its stage.
func (s *Sim) diagnose(logf func(string, ...any), prev map[*simNode]snapshot) {
	logf("stage diagnosis (Δ over the last tick; the FIRST all-zero column from the left is where the pipeline stopped):")
	logf("%-10s %-4s | %6s %8s %7s %8s %8s %7s %7s %5s | fatal", "part", "val",
		"Δround", "Δheaders", "ΔvotesOut", "ΔvotesIn", "Δcerts", "Δcommit", "Δheight", "hdrQ")
	for _, part := range s.parts {
		for _, sn := range s.byPart[part] {
			cur := sn.snap()
			p, ok := prev[sn]
			if !ok {
				p = snapshot{}
			}
			// Certificates present in the node's current and previous round —
			// is the DAG still filling where the node currently is?
			r := cur.round
			inRound := len(sn.node.DAG().GetRound(r)) + len(sn.node.DAG().GetRound(r-1))
			var fatal string
			if v := sn.fatal.Load(); v != nil {
				fatal = fmt.Sprint(v)
			}
			logf("%-10s %-4d | %6d %8d %7d %8d %8d %7d %7d %5d | %s",
				part, sn.val,
				int64(cur.round-p.round), int64(cur.headers-p.headers),
				int64(cur.votesOut-p.votesOut), int64(cur.votesIn-p.votesIn),
				int64(cur.certs-p.certs), int64(cur.lastCommit-p.lastCommit),
				int64(cur.height-p.height), inRound, fatal)
		}
	}
}

func (s *Sim) finish(start time.Time, ok bool, reason string) *Result {
	r := &Result{Ok: ok, Reason: reason, Heights: map[string]uint64{}, Elapsed: time.Since(start)}
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
