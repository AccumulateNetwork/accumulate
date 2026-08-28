package requests

// Every action spec §3.1 names must be READ as itself.
//
// The defect these pin, found on mainnet 2026-08-27: an entry on
// acc://staking.acme/requests asking to change acc://saisne.acme/stake to
// stakingValidator was not ignored — it was read as a REGISTRATION to that
// type. The parser knew three legacy markers (addaccount, withdrawtokens,
// withdraw); anything else failed to resolve and then fell into a heuristic
// meant for the oldest marker-LESS registrations, which only checked that
// the kind was unresolved, never that a marker was absent. So an entry
// stating its action had that action overwritten by an inference drawn
// from its own fields.
//
// Silently wrong is worse than silently ignored: the operator sees a
// registration nobody requested, and the request they did make disappears.

import (
	"strings"
	"testing"
)

// The real mainnet entry, block 34555747, 2026-08-27T08:30:36Z.
var saisneChangeType = [][]byte{
	[]byte("changeType"),
	[]byte("account=acc://saisne.acme/stake"),
	[]byte("type=stakingValidator"),
}

func TestChangeTypeIsNotARegistration(t *testing.T) {
	r, err := Parse(saisneChangeType)
	if err != nil {
		t.Fatalf("the real mainnet entry must parse: %v", err)
	}
	if r.Kind == KindRegister {
		t.Fatal("changeType read as a REGISTRATION — the entry states its action; " +
			"a stated action must never be overwritten by a guess from its fields")
	}
	if r.Kind != KindChangeType {
		t.Errorf("kind = %q, want %q", r.Kind, KindChangeType)
	}
	if r.Account != "acc://saisne.acme/stake" {
		t.Errorf("account = %q, want the account the entry names", r.Account)
	}
	if r.Type != "stakingValidator" {
		t.Errorf("type = %q, want stakingValidator", r.Type)
	}
	if r.Kind.ActsOn() {
		t.Error("changeType reports as fulfilled by the fleet; it is recognised, not acted on — " +
			"claiming otherwise is how an unimplemented action looks implemented")
	}
}

// TestEverySpecActionIsRecognised walks the spec §3.1 table. A spelling the
// spec defines and this package does not know is a divergence, and the
// consequence is not "ignored" — it is misread, per the test above.
func TestEverySpecActionIsRecognised(t *testing.T) {
	for _, spelling := range []string{
		"addAccount", "unstakeAccount", "withdrawTokens", "transferTokens",
		"changePayout", "changeDelegate", "changeType", "changeDelegatorPayout",
		"rejectDelegates", "cancelRequest", "registerIdentity",
	} {
		if _, ok := KindOf(spelling); !ok {
			t.Errorf("spec §3.1 defines %q and this package does not know it", spelling)
		}
	}
	if _, ok := KindOf("frobnicate"); ok {
		t.Error("an action the spec does not define must stay unknown")
	}
}

// TestLegacyMarkersCarryTheirAction: each action, written in the legacy
// era as a bare marker plus fields, keeps its own identity.
func TestLegacyMarkersCarryTheirAction(t *testing.T) {
	acct := []byte("account=acc://x.acme/stake")
	for _, tc := range []struct {
		marker string
		want   Kind
		extra  []byte
	}{
		{"changeType", KindChangeType, []byte("type=pure")},
		{"changePayout", KindChangePayout, []byte("rewards=acc://x.acme/tokens")},
		{"changeDelegate", KindChangeDelegate, []byte("delegate=acc://d.acme/stake")},
		{"changeDelegatorPayout", KindChangeDelegatorPayout, []byte("rewards=acc://x.acme/tokens")},
		{"rejectDelegates", KindRejectDelegates, []byte("type=pure")},
		{"cancelRequest", KindCancelRequest, []byte("type=pure")},
		{"registerIdentity", KindRegisterIdentity, []byte("identity=acc://x.acme")},
		{"unstakeAccount", KindUnstake, []byte("type=pure")},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			r, err := Parse([][]byte{[]byte(tc.marker), acct, tc.extra})
			if err != nil {
				t.Fatalf("must parse: %v", err)
			}
			if r.Kind != tc.want {
				t.Errorf("kind = %q, want %q", r.Kind, tc.want)
			}
			if r.Kind == KindRegister {
				t.Error("misread as a registration — the marker-less heuristic fired on a marked entry")
			}
		})
	}
}

// TestMarkerlessRegistrationStillWorks: the heuristic the fix narrowed must
// keep doing its real job. The oldest registrations carry no marker at all,
// and they are still on chain.
func TestMarkerlessRegistrationStillWorks(t *testing.T) {
	r, err := Parse([][]byte{
		[]byte("identity=acc://old.acme"),
		[]byte("stake=acc://old.acme/staking"),
		[]byte("type=pure"),
		[]byte("rewards=acc://old.acme/tokens"),
	})
	if err != nil {
		t.Fatalf("a marker-less ancient registration must still parse: %v", err)
	}
	if r.Kind != KindRegister {
		t.Errorf("kind = %q, want register — narrowing the heuristic must not disable it", r.Kind)
	}
	if r.Stake != "acc://old.acme/staking" {
		t.Errorf("stake = %q", r.Stake)
	}
}

// TestRecognisedKindsAreWritable: every action the WALLET defines must be
// writable by this package.
//
// This test previously asserted the opposite — that kinds the fleet does
// not fulfil must be unwritable — on the reasoning that writing an entry
// nobody acts on wastes credits. That reasoning inverted the dependency:
// core/wallet writes all eleven actions today, so a package that refuses
// to write them is not the definitive encoder, it is a second dialect.
// Whether the FLEET acts on a request (Kind.ActsOn) is a separate
// question from whether the request is well formed and writable.
func TestRecognisedKindsAreWritable(t *testing.T) {
	for _, tc := range []struct {
		r    *Request
		want string
	}{
		{&Request{Kind: KindChangeType, Account: "acc://a.acme/stake", Type: "pure"},
			"changeType|account=acc://a.acme/stake|type=pure"},
		{&Request{Kind: KindUnstake, Account: "acc://a.acme/stake"},
			"unstakeAccount|account=acc://a.acme/stake"},
		{&Request{Kind: KindRejectDelegates, Identity: "acc://a.acme"},
			"rejectDelegates|identity=acc://a.acme"},
		{&Request{Kind: KindRegisterIdentity, Identity: "acc://a.acme"},
			"registerIdentity|identity=acc://a.acme"},
		{&Request{Kind: KindChangeDelegatorPayout, Identity: "acc://a.acme", Destination: "acc://a.acme/rewards"},
			"changeDelegatorPayout|destination=acc://a.acme/rewards|identity=acc://a.acme"},
		{&Request{Kind: KindCancelRequest, RequestTx: "acc://abc@a.acme/x"},
			"cancelRequest|request=acc://abc@a.acme/x"},
	} {
		out, err := tc.r.Encode()
		if err != nil {
			t.Errorf("%s: the wallet writes this and we refuse to: %v", tc.r.Kind, err)
			continue
		}
		got := make([]string, len(out))
		for i, p := range out {
			got[i] = string(p)
		}
		if j := strings.Join(got, "|"); j != tc.want {
			t.Errorf("%s encoded to %q, want %q", tc.r.Kind, j, tc.want)
		}
	}
}

// TestActsOnIsAboutFulfilmentNotValidity: the fleet fulfils two actions.
// That is a statement about the FLEET, and must not leak into whether an
// entry can be read or written — conflating them is what made nine of the
// wallet's actions unreadable and then unwritable.
func TestActsOnIsAboutFulfilmentNotValidity(t *testing.T) {
	if !KindWithdraw.ActsOn() || !KindRegister.ActsOn() {
		t.Error("the fleet fulfils withdraw and register")
	}
	for _, k := range []Kind{KindChangeType, KindUnstake, KindTransfer, KindCancelRequest} {
		if k.ActsOn() {
			t.Errorf("%s reports as fulfilled by the fleet; it is not (yet)", k)
		}
	}
}
