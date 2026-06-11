// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tendermint "gitlab.com/accumulatenetwork/accumulate/exp/tendermint"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

// versionTrackerInterval is how often the version tracker recomputes the
// per-partition consensus report (#4043, §4.8). One poll is also run at
// start so the report converges without waiting a full interval.
const versionTrackerInterval = 30 * time.Second

// versionProbeTimeout bounds each individual CometBFT RPC call so a slow
// or dead node cannot stall a whole pass.
const versionProbeTimeout = 3 * time.Second

// versionProbeConcurrency bounds the number of simultaneous per-peer
// probes per pass.
const versionProbeConcurrency = 8

// ConsensusReport is the answer to "are all validators of a partition
// running the same accumulated version?" (#4043, §4.8). It is the JSON
// returned by GET /consensus/{partition}.
type ConsensusReport struct {
	Partition string `json:"partition"`
	// ValidatorConsensus is true iff every roster validator is present
	// (a live probed peer reported its key hash with voting power > 0)
	// AND all present validators report the same version.
	ValidatorConsensus bool `json:"validatorConsensus"`
	// AgreedVersion is the single agreed version when ValidatorConsensus
	// is true, else "".
	AgreedVersion string `json:"agreedVersion"`
	// Versions maps a version string to the list of validator key hashes
	// reporting it (present validators only).
	Versions map[string][]string `json:"versions"`
	// Validators is the per-roster-validator detail.
	Validators []ValidatorVersion `json:"validators"`
	// Pending is true when no probe pass has completed yet (no roster /
	// no data); ValidatorConsensus is false in that case.
	Pending bool `json:"pending,omitempty"`
	// UpdatedAt is when this report was computed.
	UpdatedAt time.Time `json:"updatedAt"`
}

// ValidatorVersion is one roster validator's probed state.
type ValidatorVersion struct {
	KeyHash    string `json:"keyHash"`
	Version    string `json:"version"`
	Present    bool   `json:"present"`
	CatchingUp bool   `json:"catchingUp"`
}

// probeResult is the outcome of probing a single CometBFT peer.
type probeResult struct {
	keyHash     string // sha256(consensus pubkey), hex
	version     string // accumulated build version from /abci_info
	votingPower int64
	catchingUp  bool
	alive       bool
}

// abciInfoData is the JSON the Accumulator ABCI Info handler packs into
// ResultABCIInfo.Response.Data (see internal/node/abci/accumulator.go).
type abciInfoData struct {
	Version  string `json:"Version"`
	Commit   string `json:"Commit"`
	Panicked bool   `json:"Panicked"`
}

// VersionTracker periodically probes each partition's consensus peers'
// CometBFT RPC, fetches the partition's validator roster from CometBFT's
// /validators RPC, and computes a per-partition ConsensusReport (#4043,
// §4.8).
type VersionTracker struct {
	partitions *PartitionTracker

	mu      sync.RWMutex
	reports map[string]ConsensusReport // keyed by lowercased partition
	rosters map[string][]string        // last-good roster (key hashes) per partition

	stopCh chan struct{}
	ticker *time.Ticker

	// newClient is overridable for tests; defaults to
	// tendermint.NewHTTPClient.
	newClient func(addr string) (rpcClient, error)
}

// rpcClient is the subset of the tendermint HTTP client the version
// tracker uses. Defined as an interface so tests can substitute a fake.
type rpcClient interface {
	Status(ctx context.Context) (*statusResult, error)
	ABCIInfo(ctx context.Context) (*abciInfoResult, error)
	Validators(ctx context.Context) ([]validatorPubKey, error)
}

// statusResult / abciInfoResult / validatorPubKey are the minimal shapes
// the tracker needs, decoupling it from the cometbft response types.
type statusResult struct {
	pubKey      []byte
	votingPower int64
	catchingUp  bool
}

type abciInfoResult struct {
	data string
}

type validatorPubKey struct {
	pubKey []byte
}

// tmHTTPClient adapts the real tendermint.HTTPClient to rpcClient.
type tmHTTPClient struct {
	c *tendermint.HTTPClient
}

func newTMHTTPClient(addr string) (rpcClient, error) {
	c, err := tendermint.NewHTTPClient(addr)
	if err != nil {
		return nil, err
	}
	return &tmHTTPClient{c: c}, nil
}

func (t *tmHTTPClient) Status(ctx context.Context) (*statusResult, error) {
	s, err := t.c.Status(ctx)
	if err != nil {
		return nil, err
	}
	var pub []byte
	if s.ValidatorInfo.PubKey != nil {
		pub = s.ValidatorInfo.PubKey.Bytes()
	}
	return &statusResult{
		pubKey:      pub,
		votingPower: s.ValidatorInfo.VotingPower,
		catchingUp:  s.SyncInfo.CatchingUp,
	}, nil
}

func (t *tmHTTPClient) ABCIInfo(ctx context.Context) (*abciInfoResult, error) {
	r, err := t.c.ABCIInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &abciInfoResult{data: r.Response.Data}, nil
}

func (t *tmHTTPClient) Validators(ctx context.Context) ([]validatorPubKey, error) {
	// page/perPage/height nil => latest, default page. CometBFT caps
	// per-page at 100; partitions have far fewer validators.
	r, err := t.c.Validators(ctx, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]validatorPubKey, 0, len(r.Validators))
	for _, v := range r.Validators {
		if v == nil || v.PubKey == nil {
			continue
		}
		out = append(out, validatorPubKey{pubKey: v.PubKey.Bytes()})
	}
	return out, nil
}

// NewVersionTracker creates a version tracker bound to a partition
// tracker (which supplies the per-partition consensus endpoints).
func NewVersionTracker(pt *PartitionTracker) *VersionTracker {
	return &VersionTracker{
		partitions: pt,
		reports:    make(map[string]ConsensusReport),
		rosters:    make(map[string][]string),
		stopCh:     make(chan struct{}),
		newClient:  newTMHTTPClient,
	}
}

// Start begins the periodic probe loop.
func (vt *VersionTracker) Start() {
	vt.ticker = time.NewTicker(versionTrackerInterval)
	go vt.loop()
}

// Stop stops the probe loop.
func (vt *VersionTracker) Stop() {
	select {
	case <-vt.stopCh:
		// already stopped
	default:
		close(vt.stopCh)
	}
	if vt.ticker != nil {
		vt.ticker.Stop()
	}
}

func (vt *VersionTracker) loop() {
	vt.probeAll()
	for {
		select {
		case <-vt.stopCh:
			return
		case <-vt.ticker.C:
			vt.probeAll()
		}
	}
}

// probeAll recomputes the report for every partition the partition
// tracker currently knows consensus endpoints for.
func (vt *VersionTracker) probeAll() {
	if vt.partitions == nil {
		return
	}
	for _, partition := range vt.partitions.ConsensusPartitions() {
		vt.probePartition(partition)
	}
}

// probePartition probes every peer of one partition, fetches the roster,
// computes the report, and stores it.
func (vt *VersionTracker) probePartition(partition string) {
	peers := vt.partitions.GetConsensusPeers(partition)
	if len(peers) == 0 {
		return
	}

	// Probe every peer's /status and /abci_info concurrently.
	probes := make(map[string]probeResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, versionProbeConcurrency)

	// alive collects peers that answered /status so the roster fetch can
	// use one of them.
	var aliveMu sync.Mutex
	var alive []consensuspeer.Peer

	for _, p := range peers {
		wg.Add(1)
		sem <- struct{}{}
		go func(p consensuspeer.Peer) {
			defer wg.Done()
			defer func() { <-sem }()

			res := vt.probePeer(p)
			if !res.alive || res.keyHash == "" {
				return
			}
			aliveMu.Lock()
			alive = append(alive, p)
			aliveMu.Unlock()

			mu.Lock()
			// If two endpoints report the same key hash (shouldn't
			// normally happen), keep the one with voting power.
			if prev, ok := probes[res.keyHash]; !ok || res.votingPower > prev.votingPower {
				probes[res.keyHash] = res
			}
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	// Fetch the roster from the first peer that answers /validators.
	roster := vt.fetchRoster(partition, alive)

	report := computeConsensus(partition, roster, probes)
	report.UpdatedAt = time.Now()

	vt.mu.Lock()
	vt.reports[strings.ToLower(partition)] = report
	vt.mu.Unlock()
}

// fetchRoster returns the set of validator key hashes for the partition,
// querying CometBFT /validators on the alive peers until one answers. On
// total failure it falls back to the last-good roster.
func (vt *VersionTracker) fetchRoster(partition string, alive []consensuspeer.Peer) []string {
	key := strings.ToLower(partition)
	for _, ap := range alive {
		addr := rpcAddr(ap)
		client, err := vt.newClient(addr)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
		vals, err := client.Validators(ctx)
		cancel()
		if err != nil || len(vals) == 0 {
			continue
		}
		roster := make([]string, 0, len(vals))
		for _, v := range vals {
			roster = append(roster, hashKey(v.pubKey))
		}
		sort.Strings(roster)
		vt.mu.Lock()
		vt.rosters[key] = roster
		vt.mu.Unlock()
		return roster
	}

	// No peer answered: reuse last-good roster if we have one.
	vt.mu.RLock()
	last := vt.rosters[key]
	vt.mu.RUnlock()
	return last
}

// probePeer calls /status and /abci_info on one peer's CometBFT RPC.
func (vt *VersionTracker) probePeer(p consensuspeer.Peer) probeResult {
	addr := rpcAddr(p)
	client, err := vt.newClient(addr)
	if err != nil {
		return probeResult{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	st, err := client.Status(ctx)
	cancel()
	if err != nil || st == nil || len(st.pubKey) == 0 {
		return probeResult{}
	}

	res := probeResult{
		keyHash:     hashKey(st.pubKey),
		votingPower: st.votingPower,
		catchingUp:  st.catchingUp,
		alive:       true,
	}

	ctx, cancel = context.WithTimeout(context.Background(), versionProbeTimeout)
	ai, err := client.ABCIInfo(ctx)
	cancel()
	if err == nil && ai != nil && ai.data != "" {
		var d abciInfoData
		if json.Unmarshal([]byte(ai.data), &d) == nil {
			res.version = d.Version
		}
	}
	return res
}

// GetConsensusReport returns the latest report for a partition.
func (vt *VersionTracker) GetConsensusReport(partition string) (ConsensusReport, bool) {
	vt.mu.RLock()
	defer vt.mu.RUnlock()
	r, ok := vt.reports[strings.ToLower(partition)]
	return r, ok
}

// computeConsensus is the pure aggregation: given a partition's roster
// (validator key hashes) and the per-key-hash probe results, it computes
// the consensus report. Probed peers whose key hash is not in the roster
// are ignored. A roster validator is present iff a live probe reported
// its hash with voting power > 0. ValidatorConsensus is true iff every
// roster validator is present and all present versions are identical.
func computeConsensus(partition string, roster []string, probes map[string]probeResult) ConsensusReport {
	report := ConsensusReport{
		Partition:  partition,
		Versions:   map[string][]string{},
		Validators: []ValidatorVersion{},
	}

	if len(roster) == 0 {
		report.Pending = true
		return report
	}

	// Deduplicate + sort the roster for deterministic output.
	seen := map[string]bool{}
	uniq := make([]string, 0, len(roster))
	for _, h := range roster {
		if !seen[h] {
			seen[h] = true
			uniq = append(uniq, h)
		}
	}
	sort.Strings(uniq)

	allPresent := true
	versionSet := map[string]bool{}

	for _, h := range uniq {
		vv := ValidatorVersion{KeyHash: h}
		if pr, ok := probes[h]; ok && pr.alive && pr.votingPower > 0 {
			vv.Present = true
			vv.Version = pr.version
			vv.CatchingUp = pr.catchingUp
			report.Versions[pr.version] = append(report.Versions[pr.version], h)
			versionSet[pr.version] = true
		} else {
			allPresent = false
		}
		report.Validators = append(report.Validators, vv)
	}

	// Sort each version's key-hash list for determinism.
	for v := range report.Versions {
		sort.Strings(report.Versions[v])
	}

	report.ValidatorConsensus = allPresent && len(versionSet) == 1
	if report.ValidatorConsensus {
		for v := range versionSet {
			report.AgreedVersion = v
		}
	}
	return report
}

// rpcAddr builds the CometBFT RPC base URL for a peer. RPCPort is
// advertised explicitly; if 0 (older node), derive RPC = P2P + 1.
func rpcAddr(p consensuspeer.Peer) string {
	port := p.RPCPort
	if port == 0 {
		port = p.Port + 1
	}
	return fmt.Sprintf("http://%s:%d", p.Host, port)
}

// hashKey returns the hex sha256 of a consensus public key — the match
// key against the on-chain / roster validator hash.
func hashKey(pubKey []byte) string {
	h := sha256.Sum256(pubKey)
	return hex.EncodeToString(h[:])
}
