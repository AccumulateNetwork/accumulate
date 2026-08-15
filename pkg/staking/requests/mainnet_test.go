package requests

// The era claims, proven against the chain instead of asserted in
// comments: testdata/mainnet-entries.json holds REAL entries captured
// from acc://staking.acme/requests (2026-08-14, chain indices recorded).
// Every era the package documents appears: the oldest no-marker
// registrations (idx 3), bare-marker withdrawals with base-unit amounts
// (idx 200), bare-marker addAccount (idx 280), and the marker+actionType
// form with the `payout` alias and an undocumented leading "Accu2"
// marker (idx 340+). If the chain taught us a new era, it gets a new
// fixture here — not a new comment.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type mainnetFixture struct {
	ChainIndex int      `json:"chainIndex"`
	Parts      []string `json:"parts"`
	Expect     struct {
		Kind        string `json:"kind"`
		Era         string `json:"era"`
		Account     string `json:"account"`
		Destination string `json:"destination"`
		Amount      string `json:"amount"`
		Stake       string `json:"stake"`
		Rewards     string `json:"rewards"`
		Delegate    string `json:"delegate"`
		Type        string `json:"type"`
	} `json:"expect"`
}

func TestParsesEveryMainnetEra(t *testing.T) {
	raw, err := os.ReadFile("testdata/mainnet-entries.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []mainnetFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 6 {
		t.Fatalf("corpus shrank: %d fixtures", len(fixtures))
	}
	for _, f := range fixtures {
		var parts [][]byte
		for _, p := range f.Parts {
			parts = append(parts, []byte(p))
		}
		r, err := Parse(parts)
		if err != nil {
			t.Errorf("chain idx %d: real mainnet entry did not parse: %v", f.ChainIndex, err)
			continue
		}
		check := func(name, got, want string) {
			if want != "" && !strings.EqualFold(got, want) {
				t.Errorf("chain idx %d: %s = %q, want %q", f.ChainIndex, name, got, want)
			}
		}
		check("kind", string(r.Kind), f.Expect.Kind)
		check("era", r.Era.String(), f.Expect.Era)
		check("account", r.Account, f.Expect.Account)
		check("destination", r.Destination, f.Expect.Destination)
		check("amount", r.Amount, f.Expect.Amount)
		check("stake", r.Stake, f.Expect.Stake)
		check("rewards", r.Rewards, f.Expect.Rewards)
		check("delegate", r.Delegate, f.Expect.Delegate)
		check("type", r.Type, f.Expect.Type)
	}
}

// The trap the strict contract parse exists for: a typo'd field on a
// contract entry must FAIL validation, not silently default.
func TestUnknownContractFieldRefusesLoudly(t *testing.T) {
	entry := []byte(`{"actionType":"register","type":"pure","stake":"acc://a.acme/stake","payout":"acc://a.acme/rewards"}`)
	r, err := Parse([][]byte{entry})
	if err != nil {
		t.Fatalf("must still parse for display: %v", err)
	}
	verr := r.Validate()
	if verr == nil {
		t.Fatal("a register with 'payout' validated — it would fulfill with rewards defaulted to the stake, a destination the author did not choose")
	}
	for _, want := range []string{"payout", "rewards"} {
		if !strings.Contains(verr.Error(), want) {
			t.Fatalf("the refusal must name the typo and the fix; missing %q in: %v", want, verr)
		}
	}
}
