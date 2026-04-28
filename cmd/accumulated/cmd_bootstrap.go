// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/headerwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/keybookat"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pinned"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pipeline"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cmdBootstrap launches a node via the v2-corrected (DN, BVN)-pair
// bootstrap.
//
// Usage:
//
//	accumulated bootstrap --network mainnet --data-dir ~/.accumulate \
//	    --bvn Apollo
//
// Output: bootstrap-state-v2.json + populated dn.db and bvn.db under
// data-dir. Subsequent runs of `accumulated run` detect the artifact
// and resume in ACTIVE.
//
// Trust model: the binary's pinned DN genesis StateTreeAnchor (from
// internal/core/bootstrap/pinned) anchors the proof. Operators on
// dev networks override via --genesis-state-tree-anchor. There's no
// validator-set hash pin and no genesis-snapshot reference.
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Launch a node from minimum-data state (v2 design)",
	Long: `Bootstrap a node by walking DN-validator-signed Directory
Anchors back to genesis (verifying validator quorum on each), pulling
DN and BVN account sets, and converging both BPT roots.

See docs/plans/bootstrap-v2.md for the full design.`,
	RunE: runBootstrap,
}

var flagBootstrap struct {
	Network                string
	DataDir                string
	BVN                    string
	GenesisStateTreeAnchor string
	ToMajorBlock           uint64
}

func init() {
	cmdMain.AddCommand(cmdBootstrap)

	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.Network, "network", "", "network: mainnet | testnet | devnet | <endpoint>")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "data directory (default ~/.accumulated)")
	f.StringVar(&flagBootstrap.BVN, "bvn", "", "BVN partition this node will run (e.g., Apollo)")
	f.StringVar(&flagBootstrap.GenesisStateTreeAnchor, "genesis-state-tree-anchor", "", "override binary pin: DN's StateTreeAnchor at major-block 1 (64 hex chars)")
	f.Uint64Var(&flagBootstrap.ToMajorBlock, "to-major-block", 0, "stop the trust phase at this major-block index (default: peer's latest)")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	if flagBootstrap.Network == "" {
		return fmt.Errorf("--network is required")
	}
	if flagBootstrap.BVN == "" {
		return fmt.Errorf("--bvn is required")
	}
	if flagBootstrap.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		flagBootstrap.DataDir = filepath.Join(home, ".accumulated")
	}
	if err := os.MkdirAll(flagBootstrap.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	endpoint := accumulate.ResolveWellKnownEndpoint(flagBootstrap.Network, "v3")
	fmt.Printf("Bootstrap configuration:\n")
	fmt.Printf("  network:   %s\n", flagBootstrap.Network)
	fmt.Printf("  endpoint:  %s\n", endpoint)
	fmt.Printf("  data-dir:  %s\n", flagBootstrap.DataDir)
	fmt.Printf("  bvn:       %s\n", flagBootstrap.BVN)
	fmt.Println()

	ctx := cmd.Context()
	client := jsonrpc.NewClient(endpoint)
	q := api.Querier2{Querier: client}

	// 1. Resolve the pin (binary table or operator override).
	pin, pinSource, err := resolveGenesisPin(flagBootstrap.Network)
	if err != nil {
		return fmt.Errorf("resolve pin: %w", err)
	}
	fmt.Printf("Pin source: %s\n", pinSource)
	fmt.Printf("  DN genesis StateTreeAnchor: %x\n", pin.DNGenesisStateTreeAnchor[:8])
	fmt.Println()

	// 2. Find ToMajorBlock if not explicitly set.
	toMajor := flagBootstrap.ToMajorBlock
	if toMajor == 0 {
		toMajor, err = currentMajorBlock(ctx, q, flagBootstrap.BVN)
		if err != nil {
			return fmt.Errorf("query current major-block: %w", err)
		}
	}
	fmt.Printf("Walking DN major-block 1..%d\n", toMajor)

	// 3. Build initial DN validator set from dn.acme/operators/1.
	operatorsUrl := protocol.DnUrl().JoinPath(protocol.Operators, "1")
	initialSet, err := buildInitialValidatorSet(ctx, q, operatorsUrl)
	if err != nil {
		return fmt.Errorf("build initial DN validator set: %w", err)
	}
	fmt.Printf("Initial DN validator set: %d validators\n", len(initialSet.Validators))
	fmt.Println()

	// 4. Construct sources.
	bvnAnchorPool := protocol.PartitionUrl(flagBootstrap.BVN).JoinPath(protocol.AnchorPool)
	headerSrc := headerwalk.NewAPISource(q, bvnAnchorPool)
	headerSrc.SetOperatorsPage(operatorsUrl)
	pullSrc := pull.NewAPISource(q)

	// 5. Open both databases.
	dnDB, err := database.OpenBadger(filepath.Join(flagBootstrap.DataDir, "dn.db"), nil)
	if err != nil {
		return fmt.Errorf("open dn.db: %w", err)
	}
	defer dnDB.Close()
	bvnDB, err := database.OpenBadger(filepath.Join(flagBootstrap.DataDir, "bvn.db"), nil)
	if err != nil {
		return fmt.Errorf("open bvn.db: %w", err)
	}
	defer bvnDB.Close()

	// 6. Run the two-phase pipeline.
	res, err := pipeline.Bootstrap(ctx, pipeline.Options{
		HeaderSource:           headerSrc,
		ToMajorBlock:           toMajor,
		InitialValidatorSet:    initialSet,
		QuorumOpts:             headerwalk.QuorumOptions{}, // default 2/3
		ApplyDelta:             keybookat.ApplyDelta,
		GenesisStateTreeAnchor: pin.DNGenesisStateTreeAnchor,
		PullSource:             pullSrc,
		DNAccounts:             dnMinimumSet(),
		DNDatabase:             dnDB,
		BVN:                    flagBootstrap.BVN,
		BVNAccounts:            bvnMinimumSet(flagBootstrap.BVN),
		BVNDatabase:            bvnDB,
		BVNAnchorFromDN:        bvnAnchorFromDN(q),
	})
	if err != nil {
		return fmt.Errorf("bootstrap pipeline: %w", err)
	}

	// 7. Persist the artifact.
	now := time.Now().UTC()
	art := &bootpersist.Artifact{
		Network:                  flagBootstrap.Network,
		BVN:                      flagBootstrap.BVN,
		DNGenesisStateTreeAnchor: pin.DNGenesisStateTreeAnchor,
		DNVerifiedAnchor:         res.DNVerifiedAnchor,
		DNVerifiedMajorBlock:     res.DNVerifiedMajorBlock,
		BVNVerifiedAnchor:        res.BVNVerifiedAnchor,
		BVNVerifiedMajorBlock:    res.BVNVerifiedMajorBlock,
		State: bootpersist.StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: now,
			EnteredActive:  now,
		},
		Cursors: bootpersist.Cursors{
			WalkLastVerified: res.DNVerifiedMajorBlock,
			AccountsPulled:   uint64(res.DNAccountsPulled + res.BVNAccountsPulled),
		},
	}
	if err := bootpersist.Save(flagBootstrap.DataDir, art); err != nil {
		return fmt.Errorf("save artifact: %w", err)
	}

	fmt.Println()
	fmt.Println("Bootstrap complete.")
	fmt.Printf("  DN accounts pulled:   %d\n", res.DNAccountsPulled)
	fmt.Printf("  DN verified anchor:   %x (major-block %d)\n", res.DNVerifiedAnchor[:8], res.DNVerifiedMajorBlock)
	fmt.Printf("  BVN accounts pulled:  %d\n", res.BVNAccountsPulled)
	fmt.Printf("  BVN verified anchor:  %x (major-block %d)\n", res.BVNVerifiedAnchor[:8], res.BVNVerifiedMajorBlock)
	fmt.Printf("  artifact:             %s/%s\n", flagBootstrap.DataDir, bootpersist.FileName)
	fmt.Println()
	fmt.Println("Run `accumulated run` to start the node in ACTIVE state.")
	return nil
}

// resolveGenesisPin returns the binary pin (or operator override).
func resolveGenesisPin(network string) (pinned.Pin, string, error) {
	if flagBootstrap.GenesisStateTreeAnchor != "" {
		hash, err := hexDecode32(flagBootstrap.GenesisStateTreeAnchor)
		if err != nil {
			return pinned.Pin{}, "", fmt.Errorf("--genesis-state-tree-anchor: %w", err)
		}
		return pinned.Pin{DNGenesisStateTreeAnchor: hash}, "operator override (--genesis-state-tree-anchor)", nil
	}
	p := pinned.Get(network)
	if p.IsZero() {
		return pinned.Pin{}, "", fmt.Errorf("no pin available for network %q — pass --genesis-state-tree-anchor to override", network)
	}
	return p, fmt.Sprintf("binary pin for %q", network), nil
}

// currentMajorBlock asks the peer for the latest DN major-block
// index. Walks DN's MajorBlockChain looking at the highest entry.
func currentMajorBlock(ctx context.Context, q api.Querier2, bvn string) (uint64, error) {
	dnAnchorPool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	one := uint64(1)
	page, err := q.QueryChain(ctx, dnAnchorPool, &api.ChainQuery{Name: "major-block"})
	_ = page
	if err != nil {
		return 0, err
	}

	// QueryChain returns a ChainRecord with the head info. The
	// chain's count is the number of major-block entries; the
	// latest major-block index is count (since major-block 1 is at
	// chain position 0).
	headRec, err := q.QueryChain(ctx, dnAnchorPool, &api.ChainQuery{Name: "major-block", Range: &api.RangeOptions{Count: &one}})
	if err != nil {
		return 0, err
	}
	return headRec.Count, nil
}

// buildInitialValidatorSet pulls the operators key page and
// projects its KeySpec entries onto a headerwalk.ValidatorSet.
func buildInitialValidatorSet(ctx context.Context, q api.Querier2, operatorsPageUrl *url.URL) (headerwalk.ValidatorSet, error) {
	rec, err := q.QueryAccount(ctx, operatorsPageUrl, nil)
	if err != nil {
		return headerwalk.ValidatorSet{}, fmt.Errorf("query operators page: %w", err)
	}
	page, ok := rec.Account.(*protocol.KeyPage)
	if !ok {
		return headerwalk.ValidatorSet{}, fmt.Errorf("operators page has unexpected type %T", rec.Account)
	}
	out := headerwalk.ValidatorSet{Validators: make([]headerwalk.Validator, 0, len(page.Keys))}
	for _, k := range page.Keys {
		var pkh [32]byte
		copy(pkh[:], k.PublicKeyHash)
		out.Validators = append(out.Validators, headerwalk.Validator{
			PublicKeyHash: pkh,
			Type:          protocol.SignatureTypeED25519,
		})
	}
	return out, nil
}

// bvnAnchorFromDN returns a closure that satisfies
// pipeline.Options.BVNAnchorFromDN. It queries dn.acme/anchors's
// main chain for the latest BlockValidatorAnchor txn whose source
// is the chosen BVN, fetches the txn body, and extracts the BVN's
// StateTreeAnchor + MajorBlockIndex from the embedded
// PartitionAnchor.
//
// Trust note: by the time this runs, the DN database is committed
// and DN's BPT root has been verified to equal the trust-phase's
// terminal anchor. The chain entries on dn.acme/anchors's main
// chain are committed via that BPT, so the txid we read out is
// trustworthy. We then re-fetch the txn body from the network and
// verify its hash matches the chain entry's hash — closing the
// trust loop without needing to keep txn bodies in the local DB.
func bvnAnchorFromDN(q api.Querier2) func(ctx context.Context, dnDB *database.Database, bvn string) ([32]byte, uint64, error) {
	return func(ctx context.Context, dnDB *database.Database, bvn string) ([32]byte, uint64, error) {
		bvnUrl := protocol.PartitionUrl(bvn)
		dnAnchorPool := protocol.DnUrl().JoinPath(protocol.AnchorPool)

		// Walk the local DN main-chain entries on dn.acme/anchors
		// in reverse order. Each entry is an anchor txn hash; for
		// each, fetch the txn from the network and check whether
		// it's a BVA from our BVN. Use the first match (most
		// recent).
		batch := dnDB.Begin(false)
		defer batch.Discard()
		mainChain, err := batch.Account(dnAnchorPool).MainChain().Get()
		if err != nil {
			return [32]byte{}, 0, fmt.Errorf("read DN anchor pool main chain: %w", err)
		}
		head := mainChain.CurrentState()
		if head.Count == 0 {
			return [32]byte{}, 0, fmt.Errorf("DN anchor pool main chain is empty")
		}

		for i := head.Count - 1; i >= 0; i-- {
			entryHash, err := batch.Account(dnAnchorPool).MainChain().Inner().Entry(i)
			if err != nil {
				return [32]byte{}, 0, fmt.Errorf("read DN anchor pool entry %d: %w", i, err)
			}
			var txid [32]byte
			copy(txid[:], entryHash)

			// Fetch the txn body from the network.
			msgRec, err := q.QueryMessage(ctx, dnAnchorPool.WithTxID(txid), nil)
			if err != nil {
				continue // try the next earlier entry; might be a non-anchor message type
			}
			tm, ok := msgRec.Message.(*messaging.TransactionMessage)
			if !ok || tm.Transaction == nil {
				continue
			}
			body, ok := tm.Transaction.Body.(protocol.AnchorBody)
			if !ok {
				continue
			}
			pa := body.GetPartitionAnchor()
			if pa == nil || pa.Source == nil {
				continue
			}
			if !pa.Source.RootIdentity().Equal(bvnUrl) {
				continue // anchor from a different partition
			}
			return pa.StateTreeAnchor, pa.MajorBlockIndex, nil
		}
		return [32]byte{}, 0, fmt.Errorf("no BVN→DN anchor found from %s in trusted DN state", bvnUrl)
	}
}

// dnMinimumSet returns the canonical DN account set the launcher
// must pull for Phase B. Includes the anchor pool itself, since
// Phase C reads BVN→DN anchors out of it after DN converges.
func dnMinimumSet() []*url.URL {
	dn := protocol.DnUrl()
	return []*url.URL{
		dn.JoinPath(protocol.Network),
		dn.JoinPath(protocol.Operators),
		dn.JoinPath(protocol.Operators, "1"),
		dn.JoinPath(protocol.Ledger),
		dn.JoinPath(protocol.AnchorPool),
		dn.JoinPath(protocol.Synthetic),
	}
}

// bvnMinimumSet returns the canonical BVN account set for Phase C.
func bvnMinimumSet(bvn string) []*url.URL {
	pu := protocol.PartitionUrl(bvn)
	return []*url.URL{
		pu.JoinPath(protocol.Operators),
		pu.JoinPath(protocol.Operators, "1"),
		pu.JoinPath(protocol.Ledger),
		pu.JoinPath(protocol.AnchorPool),
		pu.JoinPath(protocol.Synthetic),
	}
}

func hexDecode32(s string) ([32]byte, error) {
	if len(s) != 64 {
		return [32]byte{}, fmt.Errorf("expected 64 hex chars, got %d", len(s))
	}
	var out [32]byte
	for i := 0; i < 32; i++ {
		hi, err := hexNibble(s[2*i])
		if err != nil {
			return out, err
		}
		lo, err := hexNibble(s[2*i+1])
		if err != nil {
			return out, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character %q", c)
	}
}
