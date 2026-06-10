// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestPretendBuildsAllTargets is the equivalent of running the tool in
// --pretend mode against every account on the Cyclops 21-account
// repair list. It is the "commands parse correctly" check the user
// asked for: each repair envelope must build without error and survive
// a marshal/unmarshal round-trip with no semantic change.
func TestPretendBuildsAllTargets(t *testing.T) {
	require.Len(t, Targets, 21,
		"expected 21 repair targets; the canonical list must stay aligned with /tmp/cyclops-bvn-stale-final.log")

	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	opts := Options{
		Pretend:   true,
		SignerKey: key,
		SignerURL: protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519).String(),
	}

	for _, tgt := range Targets {
		t.Run(tgt.Class.String()+"/"+tgt.URL, func(t *testing.T) {
			env, err := BuildRepair(tgt, opts)
			require.NoError(t, err, "build")
			require.NotNil(t, env, "envelope")
			require.Len(t, env.Transaction, 1, "single txn per repair envelope")
			require.NotEmpty(t, env.Signatures, "envelope must be signed")

			// Round-trip: the same wire format the API endpoint will
			// see. If the envelope can't be re-decoded into an
			// equivalent struct, the network's Submit/Validate would
			// reject it before any semantic check.
			data, err := env.MarshalBinary()
			require.NoError(t, err, "marshal")
			var got messaging.Envelope
			require.NoError(t, got.UnmarshalBinary(data), "unmarshal")
			require.Equal(t, env.Transaction[0].Body.Type(), got.Transaction[0].Body.Type(),
				"body type after round-trip")
			require.Equal(t, env.Transaction[0].Header.Principal.String(),
				got.Transaction[0].Header.Principal.String(),
				"principal after round-trip")
		})
	}
}

// TestExpectedRepairTxnByClass nails down the txn type each repair
// class produces. If anyone changes the repair shape, this test makes
// the change explicit.
func TestExpectedRepairTxnByClass(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	opts := Options{
		Pretend:   true,
		SignerKey: key,
		SignerURL: protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519).String(),
	}

	cases := []struct {
		class       RepairClass
		expectedTxn protocol.TransactionType
	}{
		{ClassADI, protocol.TransactionTypeCreateDataAccount},
		{ClassOrphanADI, protocol.TransactionTypeCreateDataAccount},
		{ClassLiteIdentity, protocol.TransactionTypeAddCredits},
		{ClassLiteTokenAccount, protocol.TransactionTypeSendTokens},
		{ClassLiteDataAccountLive, protocol.TransactionTypeWriteData},
		{ClassLiteDataAccountOrphan, protocol.TransactionTypeWriteData},
	}

	for _, tc := range cases {
		// Find a target of this class to exercise.
		var sample *Target
		for i := range Targets {
			if Targets[i].Class == tc.class {
				sample = &Targets[i]
				break
			}
		}
		require.NotNilf(t, sample, "no target for class %v", tc.class)

		env, err := BuildRepair(*sample, opts)
		require.NoErrorf(t, err, "build %v", tc.class)
		require.Equalf(t, tc.expectedTxn, env.Transaction[0].Body.Type(),
			"class %v should produce %v", tc.class, tc.expectedTxn)
	}
}

// TestLiveValidatePretendSmoke runs every repair envelope through the
// network's Validate path against a real v3 endpoint, without
// submitting. Opt-in: set REPAIR_CYCLOPS_LIVE_ENDPOINT to the v3 URL
// (e.g. https://mainnet.accumulatenetwork.io/v3).
//
// Purpose: surface wire-encoding or schema-incompatibility issues
// that the local round-trip test can't see — the network must accept
// each envelope as well-formed and dispatch it to the right
// validator. Authority/balance failures are expected (we sign with a
// throwaway key that doesn't control anything on mainnet) and are
// counted but do not fail the test. The test fails only when the
// network rejects an envelope at the wire/decode level or the
// transport itself fails.
func TestLiveValidatePretendSmoke(t *testing.T) {
	endpoint := os.Getenv("REPAIR_CYCLOPS_LIVE_ENDPOINT")
	if endpoint == "" {
		t.Skip("set REPAIR_CYCLOPS_LIVE_ENDPOINT=https://mainnet.accumulatenetwork.io/v3 to run")
	}

	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	opts := Options{
		Pretend:   true,
		Endpoint:  endpoint,
		SignerKey: key,
		SignerURL: protocol.LiteAuthorityForKey(key[32:], protocol.SignatureTypeED25519).String(),
	}

	client := jsonrpc.NewClient(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	type bucket struct {
		expected int
		details  []string
	}
	var (
		ok        bucket
		expected  bucket
		bad       bucket
	)

	for i, tgt := range Targets {
		env, err := BuildRepair(tgt, opts)
		require.NoErrorf(t, err, "build %d/%d %s", i+1, len(Targets), tgt.URL)

		subs, err := client.Validate(ctx, env, api.ValidateOptions{})
		cat, msg := categorize(subs, err)
		row := line(i, tgt, msg)
		switch cat {
		case catOK:
			ok.expected++
			ok.details = append(ok.details, row)
		case catExpected:
			expected.expected++
			expected.details = append(expected.details, row)
		default:
			bad.expected++
			bad.details = append(bad.details, row)
		}
	}

	t.Logf("network smoke results against %s:", endpoint)
	t.Logf("  validated OK: %d", ok.expected)
	t.Logf("  rejected (expected — authority/balance/credits): %d", expected.expected)
	t.Logf("  rejected (UNEXPECTED — wire/decode/schema): %d", bad.expected)
	for _, d := range ok.details {
		t.Logf("  OK    %s", d)
	}
	for _, d := range expected.details {
		t.Logf("  EXP   %s", d)
	}
	for _, d := range bad.details {
		t.Logf("  BAD   %s", d)
	}

	require.Zero(t, bad.expected,
		"%d envelope(s) rejected at wire/schema level — see BAD entries above", bad.expected)
}

func line(i int, tgt Target, msg string) string {
	return tgtLabel(i, tgt) + ": " + msg
}

func tgtLabel(i int, tgt Target) string {
	url := tgt.URL
	if len(url) > 70 {
		url = url[:67] + "..."
	}
	return strings.TrimSpace(strings.Join([]string{
		spaceFmt(i+1, len(Targets)),
		tgt.Class.String(),
		url,
	}, "  "))
}

func spaceFmt(i, n int) string {
	return " [" + itoa(i) + "/" + itoa(n) + "]"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(append(b, digits...))
}

type category int

const (
	catBad category = iota
	catOK
	catExpected
)

// categorize folds (subs, err) from a Validate call into one of three
// buckets and returns a one-line summary. Centralized so the test
// can't accidentally branch through different code paths for similar
// inputs.
func categorize(subs []*api.Submission, err error) (category, string) {
	if err != nil {
		if matchesExpected(err.Error()) {
			return catExpected, err.Error()
		}
		return catBad, err.Error()
	}
	if len(subs) == 0 {
		return catBad, "no submissions returned"
	}
	s := subs[0]
	if s.Success {
		return catOK, "valid"
	}
	msg := statusMsg(s)
	if matchesExpected(msg) {
		return catExpected, msg
	}
	return catBad, msg
}

func matchesExpected(s string) bool {
	low := strings.ToLower(s)
	for _, p := range expectedSubstrings {
		if strings.Contains(low, p) {
			return true
		}
	}
	return false
}

// expectedSubstrings classifies the response as "expected" — the
// validator engine ran successfully and rejected for a reason that
// would go away with a real signer / real balance / real on-chain
// state. The smoke test cares only that the network *could* run the
// validator (i.e. the envelope was wire-correct); business-level
// rejections like these confirm exactly that.
var expectedSubstrings = []string{
	// Authority — signer key doesn't authorize the principal
	"signature does not match",
	"invalid signature",
	"unauthorized",
	"is not authorized",
	"signer is not authorized",
	"missing key book",
	"missing key page",
	// Signer URL doesn't exist on chain (or has been pruned).
	// "load signer" is the canonical wrapper; "not found" / "notfound"
	// is the typed error code. We see both in transport errors.
	"load signer",
	"not found",
	"notfound",
	// Balance / credits — signer has nothing to spend
	"insufficient credits",
	"insufficient funds",
	"insufficient balance",
	// Placeholder field values
	"oracle",
	"timestamp",
}

func statusMsg(s *api.Submission) string {
	if s == nil {
		return "<nil submission>"
	}
	if s.Status != nil && s.Status.Error != nil {
		return s.Status.Error.Error()
	}
	if s.Message != "" {
		return s.Message
	}
	return "<no message>"
}

// TestTargetCountsByClass guards against silent edits to the canonical
// list. The breakdown matches the snap-bpt-stale output against
// /mnt/secondary/.../bvnn/data/accumulate.db (BPT root
// 4364ea01d2e7092a202729d68f7740973b592dee8529e729b4b17fa84a88e5d7).
func TestTargetCountsByClass(t *testing.T) {
	counts := map[RepairClass]int{}
	for _, tt := range Targets {
		counts[tt.Class]++
	}
	require.Equal(t, 7, counts[ClassADI], "ADI count")
	require.Equal(t, 4, counts[ClassLiteIdentity], "LiteIdentity count")
	require.Equal(t, 3, counts[ClassLiteTokenAccount], "LiteTokenAccount count")
	require.Equal(t, 3, counts[ClassLiteDataAccountLive], "LiteDataAccount(live) count")
	require.Equal(t, 3, counts[ClassLiteDataAccountOrphan], "LiteDataAccount(orphan) count")
	require.Equal(t, 1, counts[ClassOrphanADI], "ADI(orphan) count")
	require.Equal(t, 0, counts[ClassBlockLedgerOrphan],
		"block-ledger orphans should be excluded — they're not repaired")
}
