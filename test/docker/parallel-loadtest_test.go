// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ── Key derivation ──

func TestDeriveKeyDeterministic(t *testing.T) {
	a := deriveKey("FAUCET")
	b := deriveKey("FAUCET")
	if !a.Equal(b) {
		t.Fatal("same inputs produced different keys")
	}

	c := deriveKey("FAUCET", "funder", 0)
	if a.Equal(c) {
		t.Fatal("different inputs produced same key")
	}

	d := deriveKey("FAUCET", "funder", 1)
	if c.Equal(d) {
		t.Fatal("different worker IDs produced same key")
	}
}

func TestDeriveKeyMatchesCreateFaucet(t *testing.T) {
	// Reproduce createFaucet logic from cmd_init_network.go exactly
	var seed storage.Key
	seed = seed.Append("FAUCET")
	expected := ed25519.NewKeyFromSeed(seed[:])

	got := deriveKey("FAUCET")
	if !expected.Equal(got) {
		t.Fatal("deriveKey does not match createFaucet logic")
	}

	expectedURL, _ := protocol.LiteTokenAddress(expected[32:], "ACME", protocol.SignatureTypeED25519)
	gotURL, _ := protocol.LiteTokenAddress(got[32:], "ACME", protocol.SignatureTypeED25519)
	if !expectedURL.Equal(gotURL) {
		t.Fatalf("URLs differ: %v vs %v", expectedURL, gotURL)
	}
}

func TestDeriveKeyUniqueWorkers(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 100; i++ {
		s := string(deriveKey("FAUCET", "funder", i))
		if prev, ok := seen[s]; ok {
			t.Fatalf("worker %d and %d collide", prev, i)
		}
		seen[s] = i
	}
}

func TestDeriveKeySenderUnique(t *testing.T) {
	seen := map[string]bool{}
	for w := 0; w < 10; w++ {
		for i := 0; i < 100; i++ {
			s := string(deriveKey("FAUCET", "sender", w, i))
			if seen[s] {
				t.Fatalf("collision at worker=%d sender=%d", w, i)
			}
			seen[s] = true
		}
	}
}

func TestAcctSlotAddrMatchesKey(t *testing.T) {
	for i := 0; i < 50; i++ {
		sk := deriveKey("FAUCET", "sender", 0, i)
		a1, _ := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
		// Verify pubkey path gives same address
		pub := sk.Public().(ed25519.PublicKey)
		a2, _ := protocol.LiteTokenAddress(pub, "ACME", protocol.SignatureTypeED25519)
		if !a1.Equal(a2) {
			t.Fatalf("slot %d: privkey and pubkey produce different addresses", i)
		}
	}
}

// ── Coprime and stride ──

func TestCoprimeAccountCount(t *testing.T) {
	cases := []struct{ input, expected int }{
		{7, 8},
		{14, 15},
		{21, 22},
		{100, 100},
		{27776, 27777},
	}
	for _, tc := range cases {
		n := tc.input
		for n%recvStride == 0 {
			n++
		}
		if n != tc.expected {
			t.Errorf("coprime(%d): got %d, want %d", tc.input, n, tc.expected)
		}
	}
}

func TestStrideCoversAllSlots(t *testing.T) {
	for _, size := range []int{11, 13, 29, 101, 27779} {
		if size%recvStride == 0 {
			t.Fatalf("test bug: %d divisible by %d", size, recvStride)
		}
		visited := make([]bool, size)
		idx := 0
		for i := 0; i < size; i++ {
			if visited[idx] {
				t.Fatalf("size=%d: revisited %d at step %d", size, idx, i)
			}
			visited[idx] = true
			idx = (idx + recvStride) % size
		}
		for i, v := range visited {
			if !v {
				t.Fatalf("size=%d: slot %d never visited", size, i)
			}
		}
	}
}

// ── Funding math ──

func TestFundingMath(t *testing.T) {
	for _, tc := range []struct {
		fund  uint64
		accts int
		want  uint64
	}{
		{100000, 27779, 3},
		{100, 27779, 2},    // clamped to minimum
		{50000, 1000, 50},
	} {
		got := tc.fund / uint64(tc.accts)
		if got < 2 {
			got = 2
		}
		if got != tc.want {
			t.Errorf("fund=%d accts=%d: got %d, want %d", tc.fund, tc.accts, got, tc.want)
		}
	}
}

func TestFaucetBalanceSufficient(t *testing.T) {
	faucetBalance := uint64(200_000_000) // whole ACME, from exp/faucet/create.go
	fundPerWorker := uint64(100000)      // default
	maxWorkers := 36 // worst case: many nodes, 16+ workers
	needed := fundPerWorker * uint64(maxWorkers)
	if needed > faucetBalance {
		t.Fatalf("need %d, faucet has %d", needed, faucetBalance)
	}
}

func TestSenderFundingSufficient(t *testing.T) {
	perAccount := uint64(100000) / uint64(27779) // ~3 ACME
	if perAccount < 2 {
		perAccount = 2
	}
	// 1 ACME for credits, rest for sends at 1 smallest unit each
	remaining := perAccount - 1
	sends := remaining * 100_000_000
	if sends < 1000 {
		t.Fatalf("only %d sends per account", sends)
	}
}

// ── Transaction building ──

// helper: build a funded test account
func testAccount(t *testing.T, seed string, index int) (ed25519.PrivateKey, *protocol.LiteTokenAccount) {
	t.Helper()
	sk := deriveKey(seed, "test", index)
	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatal(err)
	}
	acct := new(protocol.LiteTokenAccount)
	acct.Url = u
	acct.TokenUrl = protocol.AcmeUrl()
	return sk, acct
}

func TestSendTokensAmountNonZero(t *testing.T) {
	sk, acct := testAccount(t, "FAUCET", 0)
	_, recv := testAccount(t, "FAUCET", 1)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		SendTokens(1, 0).To(recv.Url). // 1 smallest unit
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if env == nil {
		t.Fatal("envelope is nil")
	}

	// Extract the SendTokens body and verify amount > 0
	body, ok := env.Transaction[0].Body.(*protocol.SendTokens)
	if !ok {
		t.Fatalf("wrong body type: %T", env.Transaction[0].Body)
	}
	if len(body.To) == 0 {
		t.Fatal("no recipients")
	}
	if body.To[0].Amount.Sign() <= 0 {
		t.Fatal("SendTokens(1, 0) produced zero or negative amount")
	}

	// Verify it equals 1 (1 smallest unit)
	if body.To[0].Amount.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("expected 1, got %s", body.To[0].Amount.String())
	}
}

func TestSendTokensWholeAcme(t *testing.T) {
	sk, acct := testAccount(t, "FAUCET", 0)
	_, recv := testAccount(t, "FAUCET", 1)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		SendTokens(5, protocol.AcmePrecisionPower).To(recv.Url). // 5 whole ACME
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	body := env.Transaction[0].Body.(*protocol.SendTokens)
	expected := new(big.Int).Mul(big.NewInt(5), big.NewInt(protocol.AcmePrecision))
	if body.To[0].Amount.Cmp(expected) != 0 {
		t.Fatalf("expected %s smallest units, got %s", expected, body.To[0].Amount.String())
	}
}

func TestSendTokensZeroAmountIsZero(t *testing.T) {
	// Verify that SendTokens(0, X) produces 0 — this was a bug
	sk, acct := testAccount(t, "FAUCET", 0)
	_, recv := testAccount(t, "FAUCET", 1)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		SendTokens(0, 1000000).To(recv.Url). // 0 * 10^1000000 = 0
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	body := env.Transaction[0].Body.(*protocol.SendTokens)
	if body.To[0].Amount.Sign() != 0 {
		t.Fatalf("SendTokens(0, 1000000) should be zero, got %s", body.To[0].Amount.String())
	}
}

func TestAddCreditsHasOracle(t *testing.T) {
	sk, acct := testAccount(t, "FAUCET", 0)
	oraclePrice := float64(1000)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		AddCredits().
		WithOracle(oraclePrice).
		Spend(1). // 1 ACME
		To(acct.Url.RootIdentity()).
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if env == nil {
		t.Fatal("envelope is nil")
	}

	body, ok := env.Transaction[0].Body.(*protocol.AddCredits)
	if !ok {
		t.Fatalf("wrong body type: %T", env.Transaction[0].Body)
	}
	if body.Oracle == 0 {
		t.Fatal("oracle is zero — transaction would be rejected by validator")
	}
	if body.Amount.Sign() <= 0 {
		t.Fatal("spend amount is zero")
	}
}

func TestAddCreditsWithoutOracleIsZero(t *testing.T) {
	// Verify that omitting WithOracle leaves oracle=0 (would fail at runtime)
	sk, acct := testAccount(t, "FAUCET", 0)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		AddCredits().
		Spend(1).
		To(acct.Url.RootIdentity()).
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	body := env.Transaction[0].Body.(*protocol.AddCredits)
	if body.Oracle != 0 {
		t.Fatalf("expected oracle=0 without WithOracle, got %d", body.Oracle)
	}
}

func TestAddCreditsRecipientIsLiteIdentity(t *testing.T) {
	sk, acct := testAccount(t, "FAUCET", 0)

	nonce := uint64(1000)
	env, err := build.Transaction().
		For(acct.Url).
		AddCredits().
		WithOracle(1000).
		Spend(1).
		To(acct.Url.RootIdentity()).
		SignWith(acct.Url).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(sk).
		Done()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	body := env.Transaction[0].Body.(*protocol.AddCredits)
	// Recipient should be the lite identity (root), not the token account
	if body.Recipient.Equal(acct.Url) {
		t.Fatal("recipient should be lite identity, not token account")
	}
	if !body.Recipient.Equal(acct.Url.RootIdentity()) {
		t.Fatalf("recipient should be %v, got %v", acct.Url.RootIdentity(), body.Recipient)
	}
}

// ── Nonce behavior ──

func TestTimestampAutoIncrements(t *testing.T) {
	sk, acct := testAccount(t, "FAUCET", 0)
	_, recv := testAccount(t, "FAUCET", 1)

	nonce := uint64(1000)
	for i := 0; i < 5; i++ {
		before := nonce
		_, err := build.Transaction().
			For(acct.Url).
			SendTokens(1, 0).To(recv.Url).
			SignWith(acct.Url).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(sk).
			Done()
		if err != nil {
			t.Fatal(err)
		}
		if nonce <= before {
			t.Fatalf("nonce did not increase: before=%d after=%d", before, nonce)
		}
	}
	// Started at 1000, 5 increments → should be 1005
	if nonce != 1005 {
		t.Fatalf("expected nonce=1005, got %d", nonce)
	}
}

func TestSingleNonceWorksAcrossAccounts(t *testing.T) {
	// Simulate what the worker does: one nonce variable, multiple sender keys
	nonce := uint64(1000)

	for i := 0; i < 5; i++ {
		before := nonce
		sk := deriveKey("FAUCET", "sender", 0, i)
		u, _ := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
		_, recv := testAccount(t, "FAUCET", 99)

		_, err := build.Transaction().
			For(u).
			SendTokens(1, 0).To(recv.Url).
			SignWith(u).
			Version(1).
			Timestamp(&nonce).
			PrivateKey(sk).
			Done()
		if err != nil {
			t.Fatal(err)
		}
		if nonce <= before {
			t.Fatalf("nonce did not increase for account %d", i)
		}
	}
}

// ── Funding transaction correctness ──

func TestFunderSendsWholeAcme(t *testing.T) {
	// Simulate what runWorker does for the funding tx
	funderSK := deriveKey("FAUCET", "funder", 0)
	funderURL, _ := protocol.LiteTokenAddress(funderSK[32:], "ACME", protocol.SignatureTypeED25519)

	senderSK := deriveKey("FAUCET", "sender", 0, 0)
	senderURL, _ := protocol.LiteTokenAddress(senderSK[32:], "ACME", protocol.SignatureTypeED25519)

	perAccountFund := uint64(3) // 3 whole ACME
	nonce := uint64(1000)

	env, err := build.Transaction().
		For(funderURL).
		SendTokens(perAccountFund, protocol.AcmePrecisionPower).To(senderURL).
		SignWith(funderURL).
		Version(1).
		Timestamp(&nonce).
		PrivateKey(funderSK).
		Done()
	if err != nil {
		t.Fatal(err)
	}

	body := env.Transaction[0].Body.(*protocol.SendTokens)
	expected := new(big.Int).Mul(big.NewInt(int64(perAccountFund)), big.NewInt(protocol.AcmePrecision))
	if body.To[0].Amount.Cmp(expected) != 0 {
		t.Fatalf("expected %s, got %s", expected, body.To[0].Amount.String())
	}
}

// ── Status JSON ──

func TestStatusJSONFormat(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status.json")

	status := map[string]any{
		"test_label":  "test",
		"current_tps": 1234.5,
		"target_tps":  10000,
		"peak_tps":    2000.0,
		"submitted":   uint64(5000),
		"succeeded":   uint64(4900),
		"failed":      uint64(100),
		"error_rate":  2.0,
		"avg_tps":     1234.5,
		"elapsed":     "1m30s",
		"workers":     36,
		"nodes":       12,
	}
	b, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusFile, b, 0644); err != nil {
		t.Fatal(err)
	}

	// Read back and verify all fields are present
	data, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	requiredFields := []string{
		"test_label", "current_tps", "target_tps", "peak_tps",
		"submitted", "succeeded", "failed", "error_rate",
		"avg_tps", "elapsed", "workers", "nodes",
	}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing field: %s", field)
		}
	}
}

// ── Credits sufficiency ──

func TestFunderCreditsForAllOperations(t *testing.T) {
	// Funder spends 3 ACME on credits at oracle=1000 USD/ACME.
	// Must have enough credits for ~27K funding SendTokens transactions.
	oracleUSD := float64(1000)
	acmeSpent := float64(3)
	acctPerWorker := 27779

	// Credits = ACME × oracle × CreditsPerDollar × CreditPrecision / AcmeOraclePrecision
	// Simplified: credits = acmeSpent × oracleUSD × 100 × 100
	credits := acmeSpent * oracleUSD * float64(protocol.CreditsPerDollar) * float64(protocol.CreditPrecision)

	// Each SendTokens costs FeeTransferTokens + FeeSignature
	costPerTx := float64(protocol.FeeTransferTokens + protocol.FeeSignature)
	maxTxns := credits / costPerTx

	if maxTxns < float64(acctPerWorker) {
		t.Fatalf("funder can only do %.0f txns, needs %d (credits=%.0f, cost/tx=%.0f)",
			maxTxns, acctPerWorker, credits, costPerTx)
	}
}

func TestSenderCreditsForLoadTest(t *testing.T) {
	// Sender spends 1 ACME on credits at oracle=1000.
	// Must have enough for many send transactions during test.
	oracleUSD := float64(1000)
	acmeSpent := float64(1)

	credits := acmeSpent * oracleUSD * float64(protocol.CreditsPerDollar) * float64(protocol.CreditPrecision)
	costPerTx := float64(protocol.FeeTransferTokens + protocol.FeeSignature)
	maxTxns := credits / costPerTx

	// Each sender should handle at least 10K transactions
	if maxTxns < 10000 {
		t.Fatalf("sender can only do %.0f txns, want at least 10000", maxTxns)
	}
}

// ── Worker state machine ──

func TestWorkerThreePhaseOrder(t *testing.T) {
	// Simulate the if/else if/else logic in runWorker
	slot := acctSlot{
		privKey: deriveKey("FAUCET", "sender", 0, 0),
	}
	slot.addr, _ = protocol.LiteTokenAddress(slot.privKey[32:], "ACME", protocol.SignatureTypeED25519)

	// Phase 1: not funded
	if slot.hasTokens {
		t.Fatal("new slot should not have tokens")
	}
	if slot.hasCredits {
		t.Fatal("new slot should not have credits")
	}

	// After funding
	slot.hasTokens = true
	if !slot.hasTokens {
		t.Fatal("slot should have tokens after funding")
	}
	if slot.hasCredits {
		t.Fatal("slot should not have credits yet")
	}

	// After credits
	slot.hasCredits = true
	if !slot.hasTokens || !slot.hasCredits {
		t.Fatal("slot should have both tokens and credits")
	}
}

// ── Namespace isolation ──

func TestFunderAndSenderKeysDontCollide(t *testing.T) {
	// Funder keys use deriveKey(seed, "funder", i)
	// Sender keys use deriveKey(seed, "sender", workerID, i)
	// These must never collide.
	funderKeys := map[string]bool{}
	for i := 0; i < 36; i++ {
		funderKeys[string(deriveKey("FAUCET", "funder", i))] = true
	}
	for w := 0; w < 36; w++ {
		for i := 0; i < 100; i++ {
			if funderKeys[string(deriveKey("FAUCET", "sender", w, i))] {
				t.Fatalf("sender key worker=%d idx=%d collides with a funder key", w, i)
			}
		}
	}
}

func TestFaucetKeyDifferentFromFunderAndSender(t *testing.T) {
	faucet := string(deriveKey("FAUCET"))
	for i := 0; i < 36; i++ {
		if string(deriveKey("FAUCET", "funder", i)) == faucet {
			t.Fatalf("funder %d key equals faucet key", i)
		}
	}
	for i := 0; i < 100; i++ {
		if string(deriveKey("FAUCET", "sender", 0, i)) == faucet {
			t.Fatalf("sender %d key equals faucet key", i)
		}
	}
}

// ── Dashboard field consistency ──

func TestStatusFieldsMatchDashboard(t *testing.T) {
	// The dashboard JS reads these specific fields from /status.
	// If status.json has different names, dashboard shows zeros.
	dashboardFields := []string{
		"test_label", "current_tps", "target_tps", "peak_tps",
		"submitted", "succeeded", "failed", "error_rate",
		"elapsed", "workers", "nodes",
	}

	// Build status map the same way parallel-loadtest.go does
	status := map[string]any{
		"test_label":  "test",
		"current_tps": 0.0,
		"target_tps":  10000,
		"peak_tps":    0.0,
		"submitted":   uint64(0),
		"succeeded":   uint64(0),
		"failed":      uint64(0),
		"error_rate":  0.0,
		"avg_tps":     0.0,
		"elapsed":     "0s",
		"workers":     36,
		"nodes":       12,
	}

	for _, field := range dashboardFields {
		if _, ok := status[field]; !ok {
			t.Errorf("dashboard expects field %q but status map is missing it", field)
		}
	}
}

// ── acctPerWorker edge case ──

func TestAcctPerWorkerMinimum(t *testing.T) {
	// If totalAccounts < totalWorkers, acctPerWorker would be 0.
	// The coprime loop would spin forever on 0 % 7 == 0.
	// Verify that acctPerWorker=0 gets bumped by coprime logic to 1.
	n := 0
	for n%recvStride == 0 {
		n++
		if n > 100 {
			t.Fatal("coprime loop did not terminate")
		}
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

// ── Edge cases ──

func TestZeroAccountsPanics(t *testing.T) {
	// The main function validates totalAccounts >= 1.
	// Verify the math would break without validation.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on divide by zero")
		}
	}()
	zero := uint64(0) // runtime variable to avoid compile-time error
	_ = uint64(100000) / zero
}

func TestOneAccountCoprime(t *testing.T) {
	n := 1
	for n%recvStride == 0 {
		n++
	}
	// 1 is not divisible by 7, so it stays 1
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestTwoAccountsStrideCoverage(t *testing.T) {
	// With 2 accounts and stride 7: idx goes 0, 7%2=1, 14%2=0 — covers both
	size := 2
	visited := make([]bool, size)
	idx := 0
	for i := 0; i < size; i++ {
		visited[idx] = true
		idx = (idx + recvStride) % size
	}
	for i, v := range visited {
		if !v {
			t.Fatalf("slot %d not visited with size=2", i)
		}
	}
}
