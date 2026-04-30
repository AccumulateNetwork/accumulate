// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// promote-validator submits the canonical AddOperator envelope pair
// (UpdateKeyPage on operators/1 + WriteData on network) signed by
// enough existing operators to meet the page threshold. After it
// commits, the new validator's pubkey is in the network's operator
// keypage, and CometBFT's validator-set update will route its votes
// in subsequent blocks.
//
// Usage:
//   promote-validator \
//     --endpoint ws://host:port/v3 \
//     --partition Directory \
//     --new-validator-key <hex-32-byte-pubkey> \
//     --operator-key <AS1...|AS1...|AS1...>
//
// Where each --operator-key is an Accumulate-encoded ed25519 private
// key (the "address" field from a docker validator's accumulate.toml).
// At least quorum-many operator keys must be provided; the threshold
// is read from the live network's operators/1 keypage.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func ed25519KeyHash(pub []byte) []byte {
	h := sha256.Sum256(pub)
	return h[:]
}

func ptrBool(b bool) *bool { return &b }

var flags struct {
	Endpoint         string
	Network          string
	Partition        string
	NewValidatorKey  string
	OperatorKeys     []string
	WaitTimeout      time.Duration
}

func main() {
	cmd := &cobra.Command{
		Use:   "promote-validator",
		Short: "Submit AddOperator envelopes to promote a node to validator",
		RunE:  run,
	}
	cmd.Flags().StringVar(&flags.Endpoint, "endpoint", "", "v3 WebSocket endpoint (e.g. ws://localhost:26680/v3)")
	cmd.Flags().StringVar(&flags.Network, "network", "", "network ID (e.g. BootstrapV3Test)")
	cmd.Flags().StringVar(&flags.Partition, "partition", "Directory", "target partition (Directory or BVN-name)")
	cmd.Flags().StringVar(&flags.NewValidatorKey, "new-validator-key", "", "new validator's 32-byte ed25519 public key (hex)")
	cmd.Flags().StringSliceVar(&flags.OperatorKeys, "operator-key", nil, "existing operator private keys (AS1... format), repeatable; at least quorum required")
	cmd.Flags().DurationVar(&flags.WaitTimeout, "wait", 60*time.Second, "max time to wait for each envelope to commit")
	_ = cmd.MarkFlagRequired("endpoint")
	_ = cmd.MarkFlagRequired("network")
	_ = cmd.MarkFlagRequired("new-validator-key")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(_ *cobra.Command, _ []string) error {
	if len(flags.OperatorKeys) < 1 {
		return fmt.Errorf("at least one --operator-key required")
	}

	// Decode the new validator's pubkey. Accept either:
	//   - 64 hex chars (32-byte ed25519 pubkey)
	//   - AS1... (Accumulate-encoded ed25519 private key — we derive pubkey)
	var newPubKey []byte
	if strings.HasPrefix(flags.NewValidatorKey, "AS1") {
		addr, err := address.Parse(flags.NewValidatorKey)
		if err != nil {
			return fmt.Errorf("--new-validator-key parse %q: %w", flags.NewValidatorKey, err)
		}
		priv, ok := addr.GetPrivateKey()
		if !ok || len(priv) != ed25519.PrivateKeySize {
			return fmt.Errorf("--new-validator-key %q: not an ed25519 private key", flags.NewValidatorKey)
		}
		newPubKey = ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	} else {
		var err error
		newPubKey, err = hex.DecodeString(strings.TrimPrefix(flags.NewValidatorKey, "0x"))
		if err != nil || len(newPubKey) != 32 {
			return fmt.Errorf("--new-validator-key must be 64 hex chars (32-byte ed25519 pubkey) or AS1... format; got %d bytes / err=%v", len(newPubKey), err)
		}
	}
	newKeyHash := ed25519KeyHash(newPubKey)
	fmt.Printf("[promote] new validator pubkey: %x\n", newPubKey)
	fmt.Printf("[promote] new validator keyhash: %x\n", newKeyHash)

	// Decode operator private keys.
	var opKeys [][]byte
	for i, ks := range flags.OperatorKeys {
		addr, err := address.Parse(ks)
		if err != nil {
			return fmt.Errorf("--operator-key[%d]: parse %q: %w", i, ks, err)
		}
		priv, ok := addr.GetPrivateKey()
		if !ok || len(priv) != ed25519.PrivateKeySize {
			return fmt.Errorf("--operator-key[%d]: not an ed25519 private key (got %d bytes)", i, len(priv))
		}
		opKeys = append(opKeys, priv)
	}
	fmt.Printf("[promote] %d operator keys provided\n", len(opKeys))

	// Connect to the network.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ws, err := websocket.NewClient(flags.Endpoint, flags.Network)
	if err != nil {
		return fmt.Errorf("dial %s: %w", flags.Endpoint, err)
	}
	defer ws.Close()

	// Query current network globals (operator keypage version + count, network definition).
	q := api.Querier2{Querier: ws}
	operatorsURL := protocol.DnUrl().JoinPath(protocol.Operators, "1")
	var page *protocol.KeyPage
	pageRec, err := q.QueryAccountAs(ctx, operatorsURL, nil, &page)
	if err != nil {
		return fmt.Errorf("load operators/1: %w", err)
	}
	_ = pageRec
	fmt.Printf("[promote] operators/1: %d keys, threshold %d, version %d\n", len(page.Keys), page.AcceptThreshold, page.Version)

	if uint64(len(opKeys)) < page.AcceptThreshold {
		return fmt.Errorf("need at least %d operator keys to meet threshold, got %d", page.AcceptThreshold, len(opKeys))
	}

	// Verify each provided operator key is actually in the page (else its signature would be ignored).
	for i, priv := range opKeys {
		pub := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
		hash := ed25519KeyHash(pub)
		found := false
		for _, k := range page.Keys {
			if string(k.PublicKeyHash) == string(hash) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("--operator-key[%d] (hash %x) is not in operators/1 keypage", i, hash[:8])
		}
	}
	fmt.Printf("[promote] all %d operator keys are valid signers\n", len(opKeys))

	// Load current network globals so we can update them. Both
	// dn.acme/network and dn.acme/globals are data accounts; their
	// current entry contains the encoded value.
	// Load the data entries directly. ParseNetwork/ParseGlobals on
	// GlobalValues both enforce a "version must increase" check that's
	// for state-update semantics, not for one-shot reads. We unmarshal
	// the binary payload directly.
	values := new(core.GlobalValues)
	values.Network = new(protocol.NetworkDefinition)
	values.Globals = new(protocol.NetworkGlobals)
	if err := loadDataEntryRaw(ctx, q, protocol.DnUrl().JoinPath(protocol.Network), values.Network); err != nil {
		return fmt.Errorf("load network definition: %w", err)
	}
	if err := loadDataEntryRaw(ctx, q, protocol.DnUrl().JoinPath(protocol.Globals), values.Globals); err != nil {
		return fmt.Errorf("load globals: %w", err)
	}
	// Read the live executor version from the system ledger so
	// FormatNetwork uses the right data-entry kind (DoubleHashDataEntry
	// for V2+, AccumulateDataEntry for V1). Without this, V2 networks
	// reject our submission with "accumulate data entries are not
	// accepted."
	var sysLedger *protocol.SystemLedger
	_, err = q.QueryAccountAs(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), nil, &sysLedger)
	if err != nil {
		return fmt.Errorf("load system ledger: %w", err)
	}
	values.ExecutorVersion = sysLedger.ExecutorVersion
	fmt.Printf("[promote] executor version: %v\n", values.ExecutorVersion)
	fmt.Printf("[promote] network has %d validators, version %d\n", len(values.Network.Validators), values.Network.Version)

	// Build signers from operator keys.
	timestamp := uint64(time.Now().UnixMilli())
	signers := make([]*signing.Builder, len(opKeys))
	for i, priv := range opKeys {
		signers[i] = new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			UseSimpleHash().
			SetPrivateKey(ed25519.PrivateKey(priv)).
			SetUrl(operatorsURL).
			SetVersion(page.Version).
			SetTimestampWithVar(&timestamp)
	}

	// Construct envelope 1: UpdateKeyPage to add new key + adjust threshold.
	env1, err := buildAddToOperatorPage(values, len(page.Keys), newKeyHash[:], signers)
	if err != nil {
		return fmt.Errorf("build keypage update: %w", err)
	}
	fmt.Printf("[promote] envelope 1 (keypage update) txid: %x\n", env1.Transaction[0].GetHash()[:8])

	// Bump signer versions for envelope 2 (it'll commit at version+1).
	for _, s := range signers {
		s.Version++
	}

	// Construct envelope 2: WriteData on network with new validator entry.
	values.Network.AddValidator(newPubKey, flags.Partition, true)
	env2, err := buildUpdateNetworkDefinition(values, signers)
	if err != nil {
		return fmt.Errorf("build network update: %w", err)
	}
	fmt.Printf("[promote] envelope 2 (network update) txid: %x\n", env2.Transaction[0].GetHash()[:8])

	// Submit env1 first.
	if err := submitAndWait(ctx, ws, env1, "keypage update"); err != nil {
		return err
	}

	// Poll the keypage until its version actually bumps. Submit's
	// Wait=true returns when the transaction is in a block, but the
	// keypage account state may not be readable at the new version
	// from every peer for another tick. Without this poll, env2's
	// signers (using version+1) hit "invalid version: have 1, got 2".
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var p *protocol.KeyPage
		_, err := q.QueryAccountAs(ctx, operatorsURL, nil, &p)
		if err == nil && p != nil && p.Version > page.Version {
			fmt.Printf("[promote] keypage version bumped to %d\n", p.Version)
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Submit env2.
	if err := submitAndWait(ctx, ws, env2, "network update"); err != nil {
		return err
	}

	fmt.Println("[promote] both envelopes committed; validator promotion complete")
	return nil
}

func buildAddToOperatorPage(values *core.GlobalValues, opCount int, newKeyHash []byte, signers []*signing.Builder) (*messaging.Envelope, error) {
	addKey := new(protocol.AddKeyOperation)
	addKey.Entry.KeyHash = newKeyHash
	setThreshold := new(protocol.SetThresholdKeyPageOperation)
	setThreshold.Threshold = values.Globals.OperatorAcceptThreshold.Threshold(opCount + 1)
	updatePage := new(protocol.UpdateKeyPage)
	updatePage.Operation = []protocol.KeyPageOperation{addKey, setThreshold}
	return initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Operators, "1"), updatePage)
}

func buildUpdateNetworkDefinition(values *core.GlobalValues, signers []*signing.Builder) (*messaging.Envelope, error) {
	values.Network.Version++
	writeData := new(protocol.WriteData)
	writeData.WriteToState = true
	writeData.Entry = values.FormatNetwork()
	return initiateTransaction(signers, protocol.DnUrl().JoinPath(protocol.Network), writeData)
}

func initiateTransaction(signers []*signing.Builder, principal *url.URL, body protocol.TransactionBody) (*messaging.Envelope, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = body
	env := new(messaging.Envelope)
	env.Transaction = append(env.Transaction, txn)
	for i, signer := range signers {
		var sig protocol.Signature
		var err error
		if i == 0 {
			sig, err = signer.Initiate(txn)
		} else {
			sig, err = signer.Sign(txn.GetHash())
		}
		if err != nil {
			return nil, errors.UnknownError.WithFormat("sign[%d]: %w", i, err)
		}
		env.Signatures = append(env.Signatures, sig)
	}
	return env, nil
}

func submitAndWait(ctx context.Context, ws api.Submitter, env *messaging.Envelope, label string) error {
	subs, err := ws.Submit(ctx, env, api.SubmitOptions{Wait: ptrBool(true)})
	if err != nil {
		return fmt.Errorf("submit %s: %w", label, err)
	}
	// The network-update envelope produces multiple sub-responses
	// (transaction, derived signatures, downstream effects). Some may
	// still show Success=false momentarily during multi-stage routing.
	// What we care about: any HARD rejection — non-empty Error or
	// non-ok Status (which would indicate a delivery failure, not a
	// pending step).
	for i, s := range subs {
		if s.Status != nil && s.Status.Error != nil {
			return fmt.Errorf("%s rejected (sub %d): %v", label, i, s.Status.Error)
		}
	}
	hardRejects := 0
	for _, s := range subs {
		if !s.Success && s.Status != nil && s.Status.Error != nil {
			hardRejects++
		}
	}
	if hardRejects > 0 {
		return fmt.Errorf("%s had %d hard rejects", label, hardRejects)
	}
	fmt.Printf("[promote] %s submitted (%d sub-responses)\n", label, len(subs))
	return nil
}

// loadDataEntryRaw reads the current Entry of a system data account
// and binary-unmarshals its first data chunk into target. Bypasses
// the version-bump check in ParseNetwork/ParseGlobals.
func loadDataEntryRaw(ctx context.Context, q api.Querier2, scope *url.URL, target interface {
	UnmarshalBinary([]byte) error
}) error {
	var acct *protocol.DataAccount
	_, err := q.QueryAccountAs(ctx, scope, nil, &acct)
	if err != nil {
		return err
	}
	if acct == nil || acct.Entry == nil {
		return fmt.Errorf("no entry on data account %s", scope)
	}
	parts := acct.Entry.GetData()
	if len(parts) == 0 {
		return fmt.Errorf("data entry at %s is empty", scope)
	}
	return target.UnmarshalBinary(parts[0])
}
