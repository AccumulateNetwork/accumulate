// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	btoml "github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMigrate.Flags().BoolVarP(&flagMigrate.Pretend, "pretend", "n", false, "Verify the migration process without touching any files")
	cmdMigrate.Flags().BoolVarP(&flagMigrate.Verbose, "verbose", "v", false, "Output the generated configuration in verbose form")
	cmdMigrate.Flags().BoolVar(&flagMigrate.Try, "try", false, "Launch the node using the migrated configuration without writing it to disk")
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
	Pretend bool
	Verbose bool
	Try     bool
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
	cfg.P2P.Key = nodeKey
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

	// DN-/BVN-specific values
	switch old.Accumulate.NetworkType {
	case protocol.PartitionTypeBlockValidator:
		cvc.BVN = old.Accumulate.PartitionId
		cvc.BvnGenesis = filepath.Join(dir, old.Genesis)
		if len(old.P2P.PersistentPeers) == 0 {
			break
		}
		for _, p := range strings.Split(old.P2P.PersistentPeers, ",") {
			addr, err := cometPeerToMultiaddr(p)
			if err != nil {
				return fmt.Errorf("{tendermint} [p2p].persistent-peers: %w", err)
			}
			cvc.BvnBootstrapPeers = append(cvc.BvnBootstrapPeers, addr)
		}

	case protocol.PartitionTypeDirectory:
		cvc.DnGenesis = filepath.Join(dir, old.Genesis)
		if len(old.P2P.PersistentPeers) == 0 {
			break
		}
		for _, p := range strings.Split(old.P2P.PersistentPeers, ",") {
			addr, err := cometPeerToMultiaddr(p)
			if err != nil {
				return fmt.Errorf("{tendermint} [p2p].persistent-peers: %w", err)
			}
			cvc.DnBootstrapPeers = append(cvc.DnBootstrapPeers, addr)
		}

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

// cometPeerToMultiaddr converts a cometbft peer address ({id}@{host}:{port}) to
// a multiaddr peer address (/dns/{host}/tcp/{port}/p2p/{id}).
func cometPeerToMultiaddr(s string) (multiaddr.Multiaddr, error) {
	// Prepend tcp://
	if !strings.Contains(s, "://") {
		s = "tcp://" + s
	}

	// Parse as a URL
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("cometbft peer address: %w", err)
	}

	// Parse peer ID
	b, err := hex.DecodeString(u.User.Username())
	if err != nil {
		return nil, fmt.Errorf("cometbft peer address: %w", err)
	}

	// Pad the hash. The result is not a valid hash, but it's sufficient for our
	// purposes.
	if len(b) < 32 {
		b = append(b, make([]byte, 32-len(b))...)
	}

	// Build a CID from the hash
	b, err = multihash.Encode(b, multihash.SHA2_256)
	if err != nil {
		return nil, fmt.Errorf("construct multihash: %w", err)
	}
	id := cid.NewCidV1(uint64(multicodec.Libp2pKey), multihash.Multihash(b)).
		Encode(multibase.MustNewEncoder(multibase.Base32))

	// Domain name or IP?
	var proto string
	if ip := net.ParseIP(u.Hostname()); ip == nil {
		proto = "dns"
	} else {
		proto = "ip4"
	}

	// Format a multiaddr
	a, err := multiaddr.NewMultiaddr(
		fmt.Sprintf("/%s/%s/tcp/%s/p2p/%s",
			proto, u.Hostname(),
			u.Port(),
			id))
	if err != nil {
		return nil, fmt.Errorf("construct multiaddr: %w", err)
	}
	return a, nil
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

	backup := filepath.Join(filepath.Dir(file), "backup")
	check(os.MkdirAll(backup, 0700))
	check(os.WriteFile(filepath.Join(backup, "accumulate.toml"), b, 0600))
	check(os.WriteFile(file, out.Bytes(), 0600))
	color.Green("Keys converted; the original is in %s", backup)
}
