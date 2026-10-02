// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"

	btoml "github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMigrate.Flags().BoolVarP(&flagMigrate.Pretend, "pretend", "n", false, "Verify the migration process without touching any files")
	cmdMigrate.Flags().BoolVarP(&flagMigrate.Verbose, "verbose", "v", false, "Output the generated configuration in verbose form")
	cmdMigrate.Flags().BoolVar(&flagMigrate.Try, "try", false, "Launch the node using the migrated configuration without writing it to disk")
	cmdMigrate.Flags().StringSliceVar(&flagMigrate.Bootstrap, "bootstrap", nil, "A partition bootstrap peer, /ip4|dns/{host}/tcp/{port}/p2p/{peer ID} (repeatable)")
	cmdMigrate.Flags().StringVar(&flagMigrate.Peer, "peer", "", "A running node (tcp://host:base-port) whose API v3 node info names the bootstrap peer")
	cmdMigrate.MarkFlagsMutuallyExclusive("pretend", "try")
}

var cmdMigrate = &cobra.Command{
	Use:     "migrate-legacy [node]",
	Short:   "Migrate a node's configuration",
	Args:    cobra.RangeArgs(0, 1),
	PreRunE: preRunMigrate,
	Run:     migrate,
}

var flagMigrate = struct {
	Pretend   bool
	Verbose   bool
	Try       bool
	Bootstrap []string
	Peer      string
}{}

func preRunMigrate(cmd *cobra.Command, args []string) error {
	if cmd.Flag("work-dir").Changed && len(args) > 0 {
		return fmt.Errorf("--work-dir and [node] argument are mutually exclusive")
	}
	return nil
}

func migrate(_ *cobra.Command, args []string) {
	if len(args) > 0 {
		flagMain.WorkDir = args[0]
	}

	// A node that main's `accumulated migrate` already converted has an
	// accumulate.toml and no partition configurations; only its keys still
	// name CometBFT key files.
	if !fileExists(filepath.Join(flagMain.WorkDir, "dnn", "config", "accumulate.toml")) &&
		!fileExists(filepath.Join(flagMain.WorkDir, "bvnn", "config", "accumulate.toml")) &&
		fileExists(filepath.Join(flagMain.WorkDir, "accumulate.toml")) {
		convertConfigFile(filepath.Join(flagMain.WorkDir, "accumulate.toml"))
		return
	}

	cfg := new(run.Config)
	cfg.P2P = new(run.P2P)
	cfg.Logging = new(run.Logging)
	cvc := new(run.CoreValidatorConfiguration)
	cfg.Configurations = []run.Configuration{cvc}

	bvnDir := isDir("bvnn")
	dnDir := isDir("dnn")
	switch {
	case bvnDir != "" && dnDir != "":
		fmt.Fprintln(os.Stderr, color.HiYellowString("Merging DN and BVN node configurations. In the case of a conflict, the current configuration of the DN node takes precedence over that of the BVN node."))
		cvc.Mode = run.CoreValidatorModeDual
	case bvnDir != "":
		cvc.Mode = run.CoreValidatorModeBVN
	case dnDir != "":
		cvc.Mode = run.CoreValidatorModeDN
	default:
		fatalf("expected %v to contain bvnn and/or dnn")
	}

	var bvn, dn *legacyConfig
	var err error
	if bvnDir != "" {
		bvn, err = loadLegacy(bvnDir)
		check(err)
		checkf(migrateCfg(cfg, cvc, flagMain.WorkDir, "bvnn", bvn), "bvn")
	}
	if dnDir != "" {
		dn, err = loadLegacy(dnDir)
		check(err)
		checkf(migrateCfg(cfg, cvc, flagMain.WorkDir, "dnn", dn), "dn")
	}

	peers, err := bootstrapPeers(context.Background())
	check(err)
	if len(peers) == 0 {
		fmt.Fprintln(os.Stderr, color.HiYellowString("No partition bootstrap peers were carried over (CometBFT's persistent peers are not libp2p peers); the node finds peers only through [p2p] bootstrap-peers. Pass --bootstrap <multiaddr> or --peer <url> to name one."))
	}
	if cvc.Mode == run.CoreValidatorModeDN || cvc.Mode == run.CoreValidatorModeDual {
		cvc.DnBootstrapPeers = peers
	}
	if cvc.Mode == run.CoreValidatorModeBVN || cvc.Mode == run.CoreValidatorModeDual {
		cvc.BvnBootstrapPeers = peers
	}
	cfg.P2P.BootstrapPeers = addAddrs(cfg.P2P.BootstrapPeers, peers)

	inst, err := run.New(context.Background(), cfg)
	check(err)
	cfg2, err := inst.Verify()
	check(err)

	if flagMigrate.Verbose {
		b, err := cfg2.Marshal(run.MarshalTOML)
		check(err)
		fmt.Println(string(b))
	} else if flagMigrate.Pretend {
		b, err := cfg.Marshal(run.MarshalTOML)
		check(err)
		fmt.Println(string(b))
	}
	if flagMigrate.Pretend {
		return
	}

	cfg.SetFilePath(filepath.Join(flagMain.WorkDir, "accumulate.toml"))
	if flagMigrate.Try {
		runCfg(cfg)
		return
	}

	// Switch configs. Copy everything and write the new config before deleting
	// so that a failure won't break the node.
	backupDir := filepath.Join(flagMain.WorkDir, "backup")
	check(os.MkdirAll(backupDir, 0700))
	if cvc.Mode == run.CoreValidatorModeBVN || cvc.Mode == run.CoreValidatorModeDual {
		b, err := os.ReadFile(filepath.Join(bvnDir, "config", "accumulate.toml"))
		check(err)
		check(os.WriteFile(filepath.Join(backupDir, "bvn-accumulate.toml"), b, 0600))
	}
	if cvc.Mode == run.CoreValidatorModeDN || cvc.Mode == run.CoreValidatorModeDual {
		b, err := os.ReadFile(filepath.Join(dnDir, "config", "accumulate.toml"))
		check(err)
		check(os.WriteFile(filepath.Join(backupDir, "dn-accumulate.toml"), b, 0600))
	}
	if existing := filepath.Join(flagMain.WorkDir, "accumulate.toml"); fileExists(existing) {
		kept, err := backupFile(existing, backupDir)
		check(err)
		fmt.Fprintln(os.Stderr, color.HiYellowString("Replacing %s; the original is kept as %s", existing, kept))
	}
	check(cfg.Save())
	if cvc.Mode == run.CoreValidatorModeBVN || cvc.Mode == run.CoreValidatorModeDual {
		check(os.Remove(filepath.Join(bvnDir, "config", "accumulate.toml")))
	}
	if cvc.Mode == run.CoreValidatorModeDN || cvc.Mode == run.CoreValidatorModeDual {
		check(os.Remove(filepath.Join(dnDir, "config", "accumulate.toml")))
	}
	color.Green("Migration complete")
}

func isDir(path string) string {
	path = filepath.Join(flagMain.WorkDir, path)
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ""
		}
		check(err)
	}

	if !st.IsDir() {
		fatalf("%v is not a directory", path)
	}
	return path
}

// migrateCfg merges one CometBFT-era partition directory, root/dir, into cfg.
// The node and validator keys are read from their CometBFT key files and
// written inline.
func migrateCfg(cfg *run.Config, cvc *run.CoreValidatorConfiguration, root, dir string, old *legacyConfig) error {
	// Shared values
	cfg.Network = old.Accumulate.Network.Id
	cfg.P2P.Listen = addAddrs(cfg.P2P.Listen, old.Accumulate.P2P.Listen)
	cfg.P2P.BootstrapPeers = addAddrs(cfg.P2P.BootstrapPeers, old.Accumulate.P2P.BootstrapPeers)
	nodeKey, err := readCometKey(filepath.Join(root, dir, old.NodeKey))
	if err != nil {
		return fmt.Errorf("node key: %w", err)
	}
	// A node is one libp2p identity. Main's `init dual` gave both partitions
	// one node key; keys that differ cannot be merged without dropping one.
	if cfg.P2P.Key == nil {
		cfg.P2P.Key = nodeKey
	} else if other, ok := cfg.P2P.Key.(*run.RawPrivateKey); !ok || !other.Equal(nodeKey) {
		return fmt.Errorf("DN and BVN nodes cannot use different node keys: the node has one peer ID")
	}
	// old.Accumulate.Healing.Enable is deliberately dropped: healing is
	// unconditional now — recovery is not something an operator can switch
	// off (#4138).

	var offset int
	if old.Accumulate.NetworkType == protocol.PartitionTypeBlockValidator {
		offset = -config.PortOffsetBlockValidator
	}
	cvc.Listen = urlToListen("{tendermint} [p2p].laddr", old.P2P.ListenAddress, offset)

	valKey, err := readCometKey(filepath.Join(root, dir, old.PrivValidatorKey))
	if err != nil {
		return fmt.Errorf("validator key: %w", err)
	}
	if cvc.ValidatorKey == nil {
		cvc.ValidatorKey = valKey
	} else if other := cvc.ValidatorKey.(*run.RawPrivateKey); !other.Equal(valKey) {
		return fmt.Errorf("DN and BVN nodes cannot use different validator keys")
	}

	if old.Accumulate.DisableDirectDispatch {
		cvc.EnableDirectDispatch = run.Ptr(false)
	}
	if old.Accumulate.MaxEnvelopesPerBlock != 0 {
		cvc.MaxEnvelopesPerBlock = run.Ptr(uint64(old.Accumulate.MaxEnvelopesPerBlock))
	}

	switch old.Accumulate.Storage.Type {
	case config.MemoryStorage:
		cvc.StorageType = run.Ptr(run.StorageTypeMemory)
	case config.BadgerStorage:
		if old.Accumulate.Storage.Path == filepath.Join("data", "accumulate.db") {
			cvc.StorageType = run.Ptr(run.StorageTypeBadger)
		} else {
			cfg.Services = append(cfg.Services, &run.StorageService{
				Name: old.Accumulate.PartitionId,
				Storage: &run.BadgerStorage{
					Path: filepath.Join(dir, old.Accumulate.Storage.Path),
				},
			})
		}
	default:
		return fmt.Errorf("migration of [storage].type = '%v'", old.Accumulate.Storage.Type)
	}

	if old.Accumulate.AnalysisLog.Enabled {
		return fmt.Errorf("migration of [analysis] not yet supported")
	}

	if old.Accumulate.API.TxMaxWaitTime != run.DefaultHTTPMaxWait {
		fmt.Fprintln(os.Stderr, color.YellowString("(%s) Ignoring [api].tx-max-wait-time = '%v' (hard-coded to '%v')", dir, old.Accumulate.API.TxMaxWaitTime, run.DefaultHTTPMaxWait))
	}

	expectApiListen := urlToListen("{tendermint} [p2p].laddr", old.P2P.ListenAddress, int(config.PortOffsetAccumulateApi))
	actualApiListen := urlToListen("[api].listen-address", old.Accumulate.API.ListenAddress, 0)
	if old.Accumulate.API.ConnectionLimit != run.DefaultHTTPConnectionLimit ||
		old.Accumulate.API.ReadHeaderTimeout != run.DefaultHTTPReadHeaderTimeout ||
		!actualApiListen.Equal(expectApiListen) {
		return fmt.Errorf("migration of non-default [api] values not yet supported")
	}

	if old.Accumulate.Logging.EnableLoki ||
		old.Accumulate.Logging.LokiUsername != "" ||
		old.Accumulate.Logging.LokiPassword != "" ||
		old.Accumulate.Logging.LokiUrl != "" {
		cfg.Logging.Loki = &run.LokiLogging{
			Enable:   old.Accumulate.Logging.EnableLoki,
			Url:      old.Accumulate.Logging.LokiUrl,
			Username: old.Accumulate.Logging.LokiUsername,
			Password: old.Accumulate.Logging.LokiPassword,
		}
	}

	// SnapshotService has been removed; CometBFT snapshot sync is replaced by
	// BPT-level sync. Detect the legacy [snapshots] section in
	// accumulate.toml and warn the operator that scheduled snapshotting is
	// being dropped, so they can verify the new behavior is acceptable.
	// The warning itself is emitted inside warnLegacySnapshotsConfig, so its
	// bool result is not needed here — only a scan failure is worth reporting.
	if _, err := warnLegacySnapshotsConfig(dir); err != nil {
		// A scan failure is non-fatal — log and continue.
		fmt.Fprintln(os.Stderr, color.YellowString("(%s) Could not check for legacy [snapshots] config: %v", dir, err))
	}

	// CometBFT's persistent peers name each peer by its CometBFT node ID,
	// which is not a libp2p identity: no node can be dialed by it, so none is
	// carried over. Partition bootstrap peers come from --bootstrap or --peer.
	if old.P2P.PersistentPeers != "" {
		fmt.Fprintln(os.Stderr, color.YellowString("(%s) Dropping {tendermint} [p2p].persistent-peers: a CometBFT node ID is not a libp2p peer ID", dir))
	}

	// DN-/BVN-specific values
	switch old.Accumulate.NetworkType {
	case protocol.PartitionTypeBlockValidator:
		cvc.BVN = old.Accumulate.PartitionId
		cvc.BvnGenesis = filepath.Join(dir, old.Genesis)

	case protocol.PartitionTypeDirectory:
		cvc.DnGenesis = filepath.Join(dir, old.Genesis)

	default:
		return fmt.Errorf("migration of [describe].type = '%v' not yet supported", old.Accumulate.NetworkType)
	}

	// TODO Check Tendermint parameters?
	return nil
}

func addAddrs(dst, src []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	have := map[string]bool{}
	for _, a := range dst {
		have[a.String()] = true
	}
	for _, a := range src {
		if !have[a.String()] {
			have[a.String()] = true
			dst = append(dst, a)
		}
	}
	return dst
}

// urlToListen converts a URL listen address to multiaddr.
func urlToListen(scope, s string, offset int) multiaddr.Multiaddr {
	addr, port, err := resolveAddrWithPort(s)
	checkf(err, scope)
	port += offset
	if addr == "0.0.0.0" {
		return multiaddr.StringCast(fmt.Sprintf("/tcp/%d", port))
	}
	if net.ParseIP(addr) == nil {
		return multiaddr.StringCast(fmt.Sprintf("/dns/%s/tcp/%d", addr, port))
	}
	return multiaddr.StringCast(fmt.Sprintf("/ip4/%s/tcp/%d", addr, port))
}

// bootstrapPeers returns the partition bootstrap peers the operator named:
// each --bootstrap address, and the node --peer names, by the libp2p identity
// its API v3 node info reports, on its P2P port.
func bootstrapPeers(ctx context.Context) ([]multiaddr.Multiaddr, error) {
	var peers []multiaddr.Multiaddr
	for _, s := range flagMigrate.Bootstrap {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return nil, fmt.Errorf("--bootstrap %q: %w", s, err)
		}
		if _, err := addr.ValueForProtocol(multiaddr.P_P2P); err != nil {
			return nil, fmt.Errorf("--bootstrap %q names no peer ID (/p2p/...)", s)
		}
		peers = append(peers, addr)
	}
	if flagMigrate.Peer == "" {
		return peers, nil
	}

	host, port, err := resolveAddrWithPort(flagMigrate.Peer)
	if err != nil {
		return nil, fmt.Errorf("--peer: %w", err)
	}
	ni, err := jsonrpc.NewClient(fmt.Sprintf("http://%s:%d/v3", host, port+int(config.PortOffsetAccumulateApi))).
		NodeInfo(ctx, v3.NodeInfoOptions{})
	if err != nil {
		return nil, fmt.Errorf("--peer: node info: %w", err)
	}
	proto := "dns"
	if ip := net.ParseIP(host); ip != nil {
		proto = "ip6"
		if ip.To4() != nil {
			proto = "ip4"
		}
	}
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d/p2p/%s", proto, host, port+int(config.PortOffsetAccumulateP2P), ni.PeerID))
	if err != nil {
		return nil, fmt.Errorf("--peer: %w", err)
	}
	return append(peers, addr), nil
}

// isCometPeer reports whether addr names its peer by a CometBFT node ID
// zero-padded into a sha256 multihash, as main's `accumulated migrate` (and
// this tool, until #4490) wrote them. No libp2p node has that identity.
func isCometPeer(addr multiaddr.Multiaddr) bool {
	v, err := addr.ValueForProtocol(multiaddr.P_P2P)
	if err != nil {
		return false
	}
	id, err := peer.Decode(v)
	if err != nil {
		return false
	}
	dm, err := multihash.Decode([]byte(id))
	if err != nil {
		return false
	}
	return dm.Code == multihash.SHA2_256 && len(dm.Digest) == 32 && bytes.Equal(dm.Digest[20:], make([]byte, 12))
}

// dropCometPeers removes, from a node configuration decoded to a generic
// document, every bootstrap peer that isCometPeer, and reports whether it
// removed any.
func dropCometPeers(doc map[string]any) bool {
	var changed bool
	filter := func(m map[string]any, field string) {
		list, ok := m[field].([]any)
		if !ok {
			return
		}
		var keep []any
		for _, v := range list {
			s, _ := v.(string)
			if addr, err := multiaddr.NewMultiaddr(s); err == nil && isCometPeer(addr) {
				fmt.Fprintln(os.Stderr, color.YellowString("Dropping %s %s: a CometBFT node ID is not a libp2p peer ID", field, s))
				changed = true
				continue
			}
			keep = append(keep, v)
		}
		m[field] = keep
	}
	if p2p, ok := doc["p2p"].(map[string]any); ok {
		filter(p2p, "bootstrap-peers")
	}
	switch v := doc["configurations"].(type) {
	case []map[string]any:
		for _, c := range v {
			filter(c, "dn-bootstrap-peers")
			filter(c, "bvn-bootstrap-peers")
		}
	case []any:
		for _, c := range v {
			if c, ok := c.(map[string]any); ok {
				filter(c, "dn-bootstrap-peers")
				filter(c, "bvn-bootstrap-peers")
			}
		}
	}
	return changed
}

// warnLegacySnapshotsConfig scans <dir>/config/accumulate.toml for the
// legacy [snapshots] section. If present and enabled, it prints a warning
// to stderr informing the operator that scheduled snapshotting has been
// removed (replaced by BPT-level sync) and the section is being dropped.
// Returns true iff a warning was emitted.
func warnLegacySnapshotsConfig(dir string) (bool, error) {
	path := filepath.Join(dir, "config", "accumulate.toml")
	tree, err := toml.LoadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	snap := tree.Get("snapshots")
	if snap == nil {
		return false, nil
	}
	subTree, ok := snap.(*toml.Tree)
	if !ok {
		return false, nil
	}

	enable, _ := subTree.Get("enable").(bool)
	schedule, _ := subTree.Get("schedule").(string)
	directory, _ := subTree.Get("directory").(string)
	retain, _ := subTree.Get("retain").(int64)

	if !enable && schedule == "" && directory == "" && retain == 0 {
		// Section present but no meaningful content; silent skip is fine.
		return false, nil
	}

	fmt.Fprintln(os.Stderr, color.YellowString(
		"(%s) WARNING: legacy [snapshots] config detected (enable=%v, schedule=%q, directory=%q, retain=%d). "+
			"CometBFT-style scheduled snapshotting has been removed; this section will not be migrated. "+
			"Snapshot synchronization is now handled at the BPT level. If you depend on scheduled snapshots, "+
			"file an issue before upgrading.",
		dir, enable, schedule, directory, retain))
	return true, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// convertConfigFile converts the CometBFT key-file references in a node
// configuration written by main's `accumulated migrate` into inline keys,
// keeping a copy of the original beside it, and checks that the result loads
// as the node loads it.
func convertConfigFile(file string) {
	b, err := os.ReadFile(file)
	check(err)
	doc := map[string]any{}
	check(btoml.Unmarshal(b, &doc))

	changed, err := convertKeyRefs(doc, filepath.Dir(file))
	checkf(err, "convert keys")
	if dropCometPeers(doc) {
		changed = true
	}
	if !changed {
		color.Green("%s names no CometBFT key file: nothing to convert", file)
		return
	}

	out := new(bytes.Buffer)
	check(btoml.NewEncoder(out).Encode(doc))
	cfg := new(run.Config)
	checkf(cfg.Load(out.Bytes(), btoml.Unmarshal), "the converted configuration does not load")

	if flagMigrate.Pretend || flagMigrate.Verbose {
		fmt.Println(out.String())
	}
	if flagMigrate.Pretend {
		return
	}

	kept, err := backupFile(file, filepath.Join(filepath.Dir(file), "backup"))
	check(err)
	check(os.WriteFile(file, out.Bytes(), 0600))
	color.Green("Keys converted; the original is kept as %s", kept)
}

// backupFile copies file into dir, under its own name or, when an earlier
// backup has that name, the first free name.N, and returns the copy's path. A
// backup is never overwritten: the first is the node's original.
func backupFile(file, dir string) (string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return "", err
	}
	name := filepath.Base(file)
	dst := filepath.Join(dir, name)
	for i := 1; fileExists(dst); i++ {
		dst = filepath.Join(dir, fmt.Sprintf("%s.%d", name, i))
	}
	return dst, os.WriteFile(dst, b, 0600)
}
