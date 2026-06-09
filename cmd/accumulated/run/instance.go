// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/exp/slices"
)

var meter = otel.Meter("gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run")
var serviceUp = must(meter.Int64Counter("accumulated_service_up"))

type Instance struct {
	config  *Config
	rootDir string
	id      string

	running  *sync.WaitGroup    // tracks jobs that want a graceful shutdown
	context  context.Context    // canceled when the instance shuts down
	shutdown context.CancelFunc // shuts down the instance
	logger   *slog.Logger
	p2p      *p2p.Node
	services ioc.Registry

	// parentInstance is set for subnodes to allow registering halt controllers
	// on the parent instance (where the HTTP service runs)
	parentInstance *Instance

	haltControllersMu sync.RWMutex
	haltControllers   map[string]*HaltController

	// consensusAdv serves this node's CometBFT endpoints (one per
	// partition) to the bootstrap server over consensuspeer.ProtocolID
	// (#4043). Populated by each ConsensusService as it starts.
	consensusAdvMu sync.Mutex
	consensusAdv   *consensusAdvertiser

	// partitionState is the per-partition handler the HTTP listener
	// uses to serve atomic bootstrap snapshots over GET
	// /v3/partition-state/<partition>. The bootstrap-v3 launcher hits
	// this endpoint, restores the bytes, and verifies BPT root against
	// the signed anchor pool. The Querier service registers an entry
	// on start; HttpService reads it during request handling.
	//
	// partitionStateLastReq is the rate-limit ledger — recording the
	// instant a request was admitted, per partition. Combined with
	// PartitionStateMinInterval below, it caps how often this node
	// services the db.View walk, so a flood of bootstraps can't pin
	// the executor's read snapshot machinery.
	partitionStateMu      sync.RWMutex
	partitionState        map[string]partitionStateHandler
	partitionStateLastReq map[string]time.Time
}

// PartitionStateMinInterval is the minimum gap between two
// /v3/partition-state/<partition> requests admitted on this node, per
// partition. Snapshot building holds a single db.View for several
// seconds and produces a 25–30 MB body; once per minute is enough for
// realistic bootstrap demand and small enough to absorb retry storms.
const PartitionStateMinInterval = time.Minute

// partitionStateHandler builds an atomic snapshot of a partition's
// state under a single database read view. Returns the minor block
// the snapshot reflects, the BPT root at that block, and the
// snapshot v2 body.
type partitionStateHandler interface {
	BuildPartitionState() (blockIndex uint64, bptRoot [32]byte, body []byte, err error)
}

// registerPartitionStateHandler maps a partition (case-insensitive) to
// the Querier instance that owns its database. Called once per
// partition from the Querier service's start hook.
func (inst *Instance) registerPartitionStateHandler(partition string, h partitionStateHandler) {
	inst.partitionStateMu.Lock()
	defer inst.partitionStateMu.Unlock()
	if inst.partitionState == nil {
		inst.partitionState = map[string]partitionStateHandler{}
	}
	inst.partitionState[strings.ToLower(partition)] = h
}

// PartitionStateHandler looks up the handler for a partition.
func (inst *Instance) PartitionStateHandler(partition string) (partitionStateHandler, bool) {
	inst.partitionStateMu.RLock()
	defer inst.partitionStateMu.RUnlock()
	h, ok := inst.partitionState[strings.ToLower(partition)]
	return h, ok
}

// BootstrapStateView is the JSON shape returned by GET
// /admin/bootstrap-state. Aggregates the bootstrap-state.json artifacts
// found under the Instance's rootDir, keyed by partition.
type BootstrapStateView struct {
	// Partitions maps partition ID (case preserved from the artifact)
	// to its loaded record. Empty when no artifact is present, e.g.
	// nodes that never went through bootstrap-v3.
	Partitions map[string]*BootstrapStateEntry `json:"partitions"`
}

// BootstrapStateEntry is the per-partition projection of an artifact
// returned by /admin/bootstrap-state. Mirrors the persisted record but
// omits resume credentials so the endpoint is safe to expose.
type BootstrapStateEntry struct {
	Network        string    `json:"network,omitempty"`
	Partition      string    `json:"partition"`
	State          string    `json:"state"`
	SinceBlock     uint64    `json:"sinceBlock,omitempty"`
	VerifiedAnchor string    `json:"verifiedAnchor,omitempty"`
	HistoryDepth   uint64    `json:"historyDepth,omitempty"`
	EnteredActive  time.Time `json:"enteredActive,omitzero"`
}

// collectBootstrapStates scans rootDir and one level of subdirectories
// for bootstrap-state.json files, loads each via bootpersist.Load, and
// returns the aggregate. Subdir scope is intentional: the dual-node
// layout is `<rootDir>/{dnn,bvnn}/bootstrap-state.json`. Errors loading
// individual files are logged but don't fail the response — the goal
// is observability, not strict consistency.
//
// Defined in instance.go (and not http.go) so it lives next to the
// Instance fields it reads, even though it's only invoked from the
// HTTP admin handler.
func (inst *Instance) collectBootstrapStates() BootstrapStateView {
	out := BootstrapStateView{Partitions: map[string]*BootstrapStateEntry{}}

	considerDir := func(dir string) {
		art, err := bootpersist.Load(dir)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			inst.logger.Warn("collect bootstrap state", "dir", dir, "error", err)
			return
		}
		entry := &BootstrapStateEntry{
			Network:        art.Network,
			Partition:      art.Partition,
			State:          art.State.Current,
			SinceBlock:     art.State.SinceBlock,
			VerifiedAnchor: bootpersist.HexKey(art.State.VerifiedAnchor),
			HistoryDepth:   art.State.HistoryDepth,
			EnteredActive:  art.State.EnteredActive,
		}
		key := art.Partition
		if key == "" {
			key = filepath.Base(dir)
		}
		out.Partitions[key] = entry
	}

	considerDir(inst.rootDir)
	if entries, err := os.ReadDir(inst.rootDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				considerDir(filepath.Join(inst.rootDir, e.Name()))
			}
		}
	}
	return out
}

// ReservePartitionState atomically applies the rate limit for a
// partition-state request. Returns (true, 0) if the request may proceed
// — and records the start time as a side effect — or (false, retryAfter)
// if a previous request was admitted within PartitionStateMinInterval.
//
// The reservation is taken on entry, not on completion, so a long-
// running snapshot walk does not allow a parallel walk to start before
// the interval elapses. A failed snapshot still consumes the slot — the
// db.View it held was the cost we are pacing.
func (inst *Instance) ReservePartitionState(partition string) (bool, time.Duration) {
	p := strings.ToLower(partition)
	inst.partitionStateMu.Lock()
	defer inst.partitionStateMu.Unlock()
	if last, ok := inst.partitionStateLastReq[p]; ok {
		if since := time.Since(last); since < PartitionStateMinInterval {
			return false, PartitionStateMinInterval - since
		}
	}
	if inst.partitionStateLastReq == nil {
		inst.partitionStateLastReq = map[string]time.Time{}
	}
	inst.partitionStateLastReq[p] = time.Now()
	return true, 0
}

const minDiskSpace = 0.05

func Start(ctx context.Context, cfg *Config) (*Instance, error) {
	inst, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return inst, inst.Start()
}

func New(ctx context.Context, cfg *Config) (*Instance, error) {
	inst := new(Instance)
	inst.config = cfg
	inst.running = new(sync.WaitGroup)
	inst.context, inst.shutdown = context.WithCancel(ctx)
	inst.services = ioc.Registry{}

	var err error
	if cfg.file != "" {
		inst.rootDir, err = filepath.Abs(filepath.Dir(cfg.file))
	} else {
		inst.rootDir, err = os.Getwd()
	}
	if err != nil {
		return nil, err
	}

	// Setup logging
	err = cfg.Logging.start(inst)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start logging: %w", err)
	}

	// Set the ID
	setDefaultVal(&cfg.P2P, new(P2P))
	setDefaultVal[PrivateKey](&cfg.P2P.Key, new(TransientPrivateKey))
	if key, err := getPrivateKey(cfg.P2P.Key, inst); err != nil {
		return nil, errors.UnknownError.WithFormat("load key: %w", err)
	} else {
		inst.id = uuid.NewSHA1(uuid.Nil, key[32:]).String()
	}

	return inst, nil
}

func (i *Instance) Done() <-chan struct{} { return i.context.Done() }

func (i *Instance) P2P() *p2p.Node { return i.p2p }

func (inst *Instance) Reset() error {
	for _, c := range inst.config.Configurations {
		c, ok := c.(resetable)
		if !ok {
			continue
		}
		err := c.reset(inst)
		if err != nil {
			return errors.UnknownError.WithFormat("reset %T: %w", c, err)
		}
	}

	for _, s := range inst.services {
		s, ok := s.(resetable)
		if !ok {
			continue
		}
		err := s.reset(inst)
		if err != nil {
			return errors.UnknownError.WithFormat("reset %T: %w", s, err)
		}
	}
	return nil
}

func (inst *Instance) Start() error {
	return inst.StartFiltered(func(s Service) bool { return true })
}

func (inst *Instance) StartFiltered(predicate func(Service) bool) (err error) {
	// Cleanup if boot fails
	defer func() {
		if err != nil {
			inst.shutdown()
		}
	}()

	// Start instrumentation and telemetry
	setDefaultVal(&inst.config.Instrumentation, new(Instrumentation))
	err = inst.config.Instrumentation.start(inst)
	if err != nil {
		return err
	}

	setDefaultVal(&inst.config.Telemetry, new(Telemetry))
	err = inst.config.Telemetry.start(inst)
	if err != nil {
		return err
	}

	// Ensure the disk does not fill up (and is not currently full; requires
	// logging)
	free, err := diskUsage(inst.rootDir)
	if err != nil {
		return err
	} else if free < minDiskSpace {
		return errors.FatalError.With("disk is full")
	}
	go inst.checkDiskSpace()

	// Apply configurations
	for _, c := range inst.config.Configurations {
		err = c.apply(inst, inst.config)
		if err != nil {
			return err
		}
	}

	// Filter
	allServices := inst.config.Services
	if predicate != nil {
		allServices = slices.DeleteFunc(allServices, func(s Service) bool { return !predicate(s) })
	}

	// Determine initialization order
	services, err := ioc.Solve(allServices)
	if err != nil {
		return err
	}

	// Start the P2P node
	err = inst.config.P2P.start(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("start p2p: %w", err)
	}

	// Prestart
	for _, services := range services {
		for _, svc := range services {
			svc, ok := svc.(prestarter)
			if !ok {
				continue
			}
			err = svc.prestart(inst)
			if err != nil {
				return errors.UnknownError.WithFormat("prestart service %T: %w", svc, err)
			}
		}
	}

	// Start services
	for _, services := range services {
		for _, svc := range services {
			slog.InfoContext(inst.context, "Starting", "module", "run", "service", svc.Type())
			err := svc.start(inst)
			if err != nil {
				return errors.UnknownError.WithFormat("start service %v: %w", svc.Type(), err)
			}

			serviceUp.Add(inst.context, 1, metric.WithAttributes(
				attribute.String("type", svc.Type().String())))

			inst.cleanup("service metrics", func(ctx context.Context) error {
				serviceUp.Add(inst.context, -1, metric.WithAttributes(
					attribute.String("type", svc.Type().String())))
				return nil
			})
		}
	}

	return nil
}

// Verify validates the configuration and returns it with all services expanded.
func (i *Instance) Verify() (*Config, error) {
	cfg := i.config.Copy()

	// Apply configurations
	for _, c := range cfg.Configurations {
		err := c.apply(i, cfg)
		if err != nil {
			return nil, err
		}
	}

	// Verify initialization is solvable
	_, err := ioc.Solve(cfg.Services)
	if err != nil {
		return nil, err
	}

	var errs []error
	for _, svc := range cfg.Services {
		svc, ok := svc.(interface{ Verify() error })
		if !ok {
			continue
		}
		err := svc.Verify()
		if err != nil {
			errs = append(errs, err)
		}
	}

	return cfg, errors.Join(errs...)
}

func (i *Instance) Stop() {
	i.shutdown()
	i.running.Wait()
}

func (i *Instance) run(fn func()) {
	i.running.Add(1)
	go func() {
		defer i.running.Done()
		fn()
	}()
}

func (i *Instance) cleanup(name string, fn func(context.Context) error) {
	i.running.Add(1)
	go func() {
		defer i.running.Done()
		<-i.context.Done()

		slog.Debug("Stopping", "process", name)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := fn(ctx)
		if err != nil {
			slog.Error("Error during shutdown", "error", err, "process", name)
		} else {
			slog.Debug("Stopped", "process", name)
		}
	}()
}

func (i *Instance) path(path ...string) string {
	if len(path) == 0 {
		return i.rootDir
	}
	if filepath.IsAbs(path[0]) {
		return filepath.Join(path...)
	}
	return filepath.Join(append([]string{i.rootDir}, path...)...)
}

func (i *Instance) checkDiskSpace() {
	for {
		free, err := diskUsage(i.rootDir)
		if err != nil {
			i.logger.Error("Failed to get disk size, shutting down", "error", err, "module", "node")
			return
		}

		if free < 0.05 {
			i.logger.Error("Less than 5% disk space available, shutting down", "free", free, "module", "node")
			return
		}

		i.logger.Info("Disk usage", "free", free, "module", "node")

		time.Sleep(10 * time.Minute)
	}
}

// RegisterHaltController registers a halt controller for a partition.
// If this instance has a parent (i.e., it's a subnode), the controller is
// registered on the parent instance so the HTTP handler can access it.
func (i *Instance) RegisterHaltController(hc *HaltController) {
	target := i
	if i.parentInstance != nil {
		target = i.parentInstance
	}
	target.haltControllersMu.Lock()
	defer target.haltControllersMu.Unlock()
	if target.haltControllers == nil {
		target.haltControllers = make(map[string]*HaltController)
	}
	target.haltControllers[hc.Partition()] = hc
}

// RequestHaltAll requests halt for all registered partitions.
func (i *Instance) RequestHaltAll() {
	i.haltControllersMu.RLock()
	defer i.haltControllersMu.RUnlock()
	for _, hc := range i.haltControllers {
		hc.RequestHalt()
	}
}

// CancelHaltAll cancels halt for all registered partitions.
func (i *Instance) CancelHaltAll() {
	i.haltControllersMu.RLock()
	defer i.haltControllersMu.RUnlock()
	for _, hc := range i.haltControllers {
		hc.CancelHalt()
	}
}

// GetHaltStatus returns the halt status for all partitions.
func (i *Instance) GetHaltStatus() HaltStatus {
	i.haltControllersMu.RLock()
	defer i.haltControllersMu.RUnlock()

	status := HaltStatus{}
	for _, hc := range i.haltControllers {
		if hc.IsHaltPending() {
			status.Pending = true
			status.Partitions = append(status.Partitions, hc.Partition())
		}
	}
	return status
}
