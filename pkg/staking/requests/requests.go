// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package requests owns the vocabulary of the staking requests account
// (acc://staking.acme/requests on mainnet).
//
// It exists because that vocabulary was implemented twice, independently, and
// the two implementations did not agree. The staking system's production report
// read the pre-contract encoding and rendered conforming entries as
// "informational" (core/staking#449); the wallet read only the contract and
// rendered pre-contract entries as "refused" (core/wallet#272). Same bug,
// opposite directions, one root cause.
//
// It lives here, rather than in either consumer, because core/staking and
// core/wallet both already depend on this module and neither should depend on
// the other. Anything that reads or writes staking requests should use this
// package rather than growing a third dialect.
//
// The package deliberately has no dependencies beyond the standard library:
// URLs are plain strings so a consumer is not forced into a particular URL
// type, and errors are plain errors so no error-code vocabulary is imposed.
//
// The SPEC remains normative: core/staking's spec/ directory (the staking
// distribution specification and its numbered decomposition) defines what a
// request IS; this package implements that definition. Where this package and
// the spec disagree, the spec wins and this package has a bug.
//
// The asymmetry is deliberate and load-bearing:
//
//   - Parse reads EVERY era that exists on chain.
//   - Encode writes ONE, the current contract.
//
// Reading has to be generous because the chain is immutable and the old entries
// are still there. Writing has to be strict because a request outside the
// contract is accepted by the chain, billed, and never fulfilled. One encoder
// means the format cannot fork again.
//
// # The two consumers use opposite halves
//
// This is why both halves live in one package, and which half protects whom:
//
//   - The WALLET writes. A person composes a request and the wallet encodes it
//     into an entry on acc://staking.acme/requests. Encode is strict for the
//     wallet's sake: a malformed request is still accepted by the chain and
//     still billed, and then never fulfilled, so the wallet must be unable to
//     write one. There is one encoder precisely so the wallet cannot invent a
//     dialect.
//
//   - The ASP STAKING APP reads. It parses the requests account and generates
//     the transactions that fulfil each request, with the timing the spec
//     requires. Parse is generous for ASP's sake: the chain is immutable, entries
//     written in every earlier era are still sitting there, and ASP must still
//     act on the ones the old pipeline acted on. Refusing to read them would
//     strand real requests.
//
// Validate therefore belongs to ASP, not to the wallet. Parse answers "what does
// this entry say"; Validate answers "may this be acted on". They are separate
// questions because some entries must be readable — so a person can be shown
// what is on chain — while remaining unsafe to fulfil. A contract entry carrying
// several payloads is the clearest case: it parses, and it must never be acted
// on, because fulfilments are matched back by entry hash and several requests
// sharing one entry would share one fulfilment memo.
package requests

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Kind is what a request asks the staking system to do — the actions spec
// §3.1 defines for acc://staking.acme/requests.
//
// Until 2026-08-27 only withdraw and register existed here, and the parser
// did not merely ignore the rest: an entry whose marker it did not know
// fell through to the "ancient registration" heuristic below and was read
// as a REGISTRATION. A real changeType on acc://saisne.acme/stake parsed as
// a re-registration to stakingValidator; changePayout, changeDelegate and
// cancelRequest did the same. Silently wrong, not silently ignored.
//
// Every action in the spec's table is now a Kind, so an entry the fleet
// does not act on is still READ correctly and can be shown for what it is.
// Acting on them is a separate matter — see ActsOn.
type Kind string

const (
	// The two the validator fleet fulfils.
	KindWithdraw Kind = "withdraw"
	KindRegister Kind = "register"

	// Account lifecycle (spec §3.1). Recognised and reported; the fleet
	// does not yet fulfil them.
	KindUnstake  Kind = "unstakeAccount"
	KindTransfer Kind = "transferTokens"

	// Account changes (spec §3.1). Signer must be the account's authority.
	KindChangePayout          Kind = "changePayout"
	KindChangeDelegate        Kind = "changeDelegate"
	KindChangeType            Kind = "changeType"
	KindChangeDelegatorPayout Kind = "changeDelegatorPayout"
	KindRejectDelegates       Kind = "rejectDelegates"
	KindCancelRequest         Kind = "cancelRequest"
	KindRegisterIdentity      Kind = "registerIdentity"
)

// allKinds is every action spec §3.1 names, keyed by its lower-cased
// canonical spelling. One table drives contract parsing, legacy markers and
// display, so a kind cannot be recognised in one path and unknown in
// another — which is exactly how changeType came to be misread.
var allKinds = map[string]Kind{
	"withdraw":              KindWithdraw,
	"withdrawtokens":        KindWithdraw,
	"register":              KindRegister,
	"addaccount":            KindRegister,
	"unstakeaccount":        KindUnstake,
	"transfertokens":        KindTransfer,
	"changepayout":          KindChangePayout,
	"changedelegate":        KindChangeDelegate,
	"changetype":            KindChangeType,
	"changedelegatorpayout": KindChangeDelegatorPayout,
	"rejectdelegates":       KindRejectDelegates,
	"cancelrequest":         KindCancelRequest,
	"registeridentity":      KindRegisterIdentity,
}

// Marker is the wallet's spelling of this action — the bare marker it
// writes as the entry's first part. Kind values for the nine account and
// lifecycle actions already ARE the wallet's spelling; the two the fleet
// fulfils carry historical Go names and map back here.
func (k Kind) Marker() string {
	switch k {
	case KindRegister:
		return "addAccount"
	case KindWithdraw:
		return "withdrawTokens"
	default:
		return string(k)
	}
}

// KindOf resolves a spelling to its Kind. The bool reports recognition.
func KindOf(s string) (Kind, bool) {
	k, ok := allKinds[strings.ToLower(strings.TrimSpace(s))]
	return k, ok
}

// ActsOn reports whether the validator fleet FULFILS this kind today.
//
// Stated explicitly rather than left implicit, because "the parser knows
// it" and "the fleet does it" are different claims and conflating them is
// what makes an unimplemented action look implemented. A recognised kind
// the fleet does not act on is reported to the operator and applied to
// nothing.
func (k Kind) ActsOn() bool {
	return k == KindWithdraw || k == KindRegister
}

// Era is the encoding an entry was written in.
type Era int

const (
	// EraContract is the current encoding: one JSON payload per entry.
	EraContract Era = iota + 1
	// EraLegacy is the pre-contract encoding: a bare action marker and
	// key=value parts, several payloads per entry. Read, never written.
	EraLegacy
)

func (e Era) String() string {
	switch e {
	case EraContract:
		return "contract"
	case EraLegacy:
		return "legacy"
	default:
		return "unknown"
	}
}

// ErrNotARequest reports an entry that is not a staking request in any era:
// announcements, test entries, binary blobs. These are not malformed requests —
// they were never requests — so callers should say so rather than reporting a
// refusal.
var ErrNotARequest = errors.New("not a staking request")

// Request is one request, normalised across eras. A legacy addAccount and a
// contract register expressing the same intent parse to the same value; if they
// did not, this package would just move the divergence up a level.
type Request struct {
	Kind Kind

	// Account is the staking account a withdrawal draws from.
	Account string
	// Destination is where a withdrawal's tokens go. Any token account.
	Destination string
	// Amount is ACME as a plain decimal string, never nanoACME.
	Amount string

	// Stake is the staking account a registration registers.
	Stake string
	// Type is the staking class.
	Type string
	// Rewards is the payout destination. Empty means the stake itself.
	Rewards string
	// Delegate is the delegate's STAKING ACCOUNT url, not its ADI.
	Delegate string

	// Identity is the registration's ADI. Carried by some legacy entries; the
	// contract derives it from the stake instead, so Encode drops it.
	Identity string
	// RequestTx is a legacy cross-reference to another transaction. The
	// contract has no such field; Encode drops it.
	RequestTx string

	// Era is the encoding this request was parsed from. Zero for a request
	// built by Withdraw or Register.
	Era Era

	// Payloads is how many data payloads the parsed entry carried. The
	// contract is one request per entry — several payloads share an entry
	// hash and so a fulfillment memo, making their fulfillments
	// indistinguishable — so Validate refuses more than one. Zero for a
	// request built by Withdraw or Register (Encode always writes one).
	Payloads int

	// UnknownFields are top-level JSON keys a contract-era entry carried
	// that the contract does not define. They are the silent-wrong-outcome
	// trap: {"payout": …} instead of {"rewards": …} parses as a valid
	// register whose payout DEFAULTS TO THE STAKE — well-formed, fulfilled,
	// and pointed somewhere the author did not choose. Validate refuses
	// while any are present; display callers can show the entry anyway.
	UnknownFields []string
}

// Subject is the account a request names — the one whose key book must have
// signed it. Withdrawals name it as the account, registrations as the stake.
func (r *Request) Subject() string {
	if r.Account != "" {
		return r.Account
	}
	return r.Stake
}

// amountPattern is the only amount syntax the fleet accepts. It must stay
// identical to amountPattern in core/staking's cmd/asp/requests.go. A general
// number parser would also accept "1e9", "0x2710" and "1/3", none of which read
// as the amount that would actually move.
var amountPattern = regexp.MustCompile(`^\d{1,12}(\.\d{1,8})?$`)

// Classes are the staking classes a registration may name, canonically spelled.
// Comparison is case-insensitive, matching stakingRegisterClasses in
// core/staking's pkg/genbrowser/requests.go. A class outside this set is
// statically dead: the signers refuse it on every pass, so such a request can
// never be fulfilled.
var Classes = []string{"pure", "delegated", "coreValidator", "coreFollower", "stakingValidator"}

// CanonicalClass returns the canonical spelling of a staking class, accepting
// any case and the hyphenated forms people type on a command line
// ("core-validator"). The second result reports whether it is a class at all.
func CanonicalClass(s string) (string, bool) {
	norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	for _, c := range Classes {
		if strings.ToLower(c) == norm {
			return c, true
		}
	}
	return "", false
}

// Withdraw builds a withdrawal request.
//
// The destination may be any token account. Where it points decides when the
// withdrawal pays — the same pay period to another staking account, two periods
// later to an account outside staking — but that is derived from the entry by
// the fleet. A caller that branches on the destination is deciding something it
// does not own.
//
// amount is ACME as the user wrote it, not nanoACME.
func Withdraw(account, destination, amount string) (*Request, error) {
	r := &Request{
		Kind:        KindWithdraw,
		Account:     strings.TrimSpace(account),
		Destination: strings.TrimSpace(destination),
		Amount:      strings.TrimSpace(amount),
	}
	return r, r.Validate()
}

// Register builds a registration request.
//
// Registration is also how every change is made: there is no change action, so
// re-registering with new values is how the type, the delegate and the payout
// destination are all updated. Because the registry entry the fleet writes is a
// whole record, a re-registration REPLACES — a field left empty is reset, not
// preserved. Callers changing one field must carry the others.
//
// rewards is the payout destination and defaults to the stake when empty.
// delegate is the delegate's staking account url, not its ADI: the signers match
// it against registered stakers' account URLs, so an ADI never resolves.
func Register(stake, class, rewards, delegate string) (*Request, error) {
	canonical, ok := CanonicalClass(class)
	if !ok {
		canonical = strings.TrimSpace(class) // let Validate produce the message
	}
	r := &Request{
		Kind:     KindRegister,
		Stake:    strings.TrimSpace(stake),
		Type:     canonical,
		Rewards:  strings.TrimSpace(rewards),
		Delegate: strings.TrimSpace(delegate),
	}
	return r, r.Validate()
}

// Validate applies the fleet's own acceptance rules, so a caller can refuse
// locally instead of billing a permanent entry the fleet will never act on.
//
// It does not apply the governance gates — the full-stake minimum, and whether
// a delegate is a registered staker. Those are not validity: a request failing
// them is well formed, is re-checked every pass, and starts working once the
// precondition is met, without being refiled. Refusing them locally would
// refuse requests that are merely early.
func (r *Request) Validate() error {
	if r.Payloads > 1 {
		return fmt.Errorf("entry carries %d payloads, want exactly 1 — several payloads would share an entry hash and a fulfillment memo, making their fulfillments indistinguishable", r.Payloads)
	}
	if len(r.UnknownFields) > 0 {
		hints := ""
		for _, f := range r.UnknownFields {
			if want, ok := fieldHints[strings.ToLower(f)]; ok {
				hints += fmt.Sprintf(" (%q is not a contract field — use %q)", f, want)
			}
		}
		return fmt.Errorf("unknown field(s) %s%s — the fleet ignores fields it does not define, so this request would be fulfilled with defaults the author did not choose",
			strings.Join(r.UnknownFields, ", "), hints)
	}
	switch r.Kind {
	case KindWithdraw:
		if r.Account == "" || r.Destination == "" {
			return fmt.Errorf("a withdraw request needs an account and a destination")
		}
		if !amountPattern.MatchString(r.Amount) {
			return fmt.Errorf(
				"amount %q must be a plain decimal such as 12.5 — at most 12 digits and 8 decimal places, no exponent, no symbols",
				r.Amount)
		}
		return nil

	case KindRegister:
		if r.Stake == "" || r.Type == "" {
			return fmt.Errorf("a register request needs a stake account and a type")
		}
		if _, ok := CanonicalClass(r.Type); !ok {
			return fmt.Errorf("%q is not a staking class; the signers refuse it — use one of %s",
				r.Type, strings.Join(Classes, ", "))
		}
		if strings.EqualFold(r.Type, "delegated") && r.Delegate == "" {
			return fmt.Errorf("a delegated registration needs a delegate")
		}
		return nil

	// The wallet's other actions. Each is judged on its OWN required
	// fields — the subject it names and the value it changes.
	//
	// This used to be a blanket refusal: "not a staking request; the fleet
	// acts on withdraw and register only". That described the FULFILMENT
	// set and applied it to validity, so every other action the wallet can
	// write was declared invalid. Whether the fleet acts on a request is
	// Kind.ActsOn; whether the request is well formed is this method.

	case KindTransfer:
		if r.Account == "" || r.Destination == "" {
			return fmt.Errorf("a transfer request needs an account and a recipient")
		}
		if !amountPattern.MatchString(r.Amount) {
			return fmt.Errorf("amount %q must be a plain decimal such as 12.5", r.Amount)
		}
		return nil

	case KindUnstake:
		if r.Account == "" {
			return fmt.Errorf("an unstake request needs an account")
		}
		return nil

	case KindChangeType:
		if r.Account == "" || r.Type == "" {
			return fmt.Errorf("a changeType request needs an account and a type")
		}
		if _, ok := CanonicalClass(r.Type); !ok {
			return fmt.Errorf("%q is not a staking class — use one of %s",
				r.Type, strings.Join(Classes, ", "))
		}
		return nil

	case KindChangePayout:
		if r.Account == "" || r.Destination == "" {
			return fmt.Errorf("a changePayout request needs an account and a destination")
		}
		return nil

	case KindChangeDelegate:
		if r.Account == "" || r.Delegate == "" {
			return fmt.Errorf("a changeDelegate request needs an account and a delegate")
		}
		return nil

	case KindChangeDelegatorPayout:
		if r.Identity == "" || r.Destination == "" {
			return fmt.Errorf("a changeDelegatorPayout request needs an identity and a destination")
		}
		return nil

	case KindRejectDelegates, KindRegisterIdentity:
		if r.Identity == "" {
			return fmt.Errorf("a %s request needs an identity", r.Kind)
		}
		return nil

	case KindCancelRequest:
		// Two ways to name the target (spec §3.1): the transaction, or a
		// description of it — account + amount + destination — which
		// revokes the LAST matching request that has not executed. The
		// description form exists for the case it is named after: a
		// request filed twice and meant once.
		if r.RequestTx != "" {
			return nil
		}
		if r.Account != "" && r.Amount != "" && r.Destination != "" {
			if !amountPattern.MatchString(r.Amount) {
				return fmt.Errorf("amount %q must be a plain decimal such as 12.5", r.Amount)
			}
			return nil
		}
		return fmt.Errorf("a cancelRequest names its target either by request=<txid> " +
			"or by account, amount and destination together")

	default:
		return fmt.Errorf("%q is not an action the wallet defines", r.Kind)
	}
}

// contractEntry is the wire form of the current encoding. Field order and names
// are the contract; core/staking's cmd/asp/requests.go unmarshals into the
// equivalent struct.
type contractEntry struct {
	ActionType  string `json:"actionType"`
	Account     string `json:"account,omitempty"`
	Destination string `json:"destination,omitempty"`
	Amount      string `json:"amount,omitempty"`
	Type        string `json:"type,omitempty"`
	Stake       string `json:"stake,omitempty"`
	Rewards     string `json:"rewards,omitempty"`
	Delegate    string `json:"delegate,omitempty"`
}

// Encode renders a request in the current contract encoding: the data payloads
// of a WriteData entry, always exactly one, a JSON object.
//
// Legacy encodings are never produced. Parsing a legacy entry and encoding it
// again yields a CONTRACT entry, not the original bytes — this is a one-way
// migration, not a round trip, and anything depending on byte-identical
// re-encoding has to say so.
//
// Identity and RequestTx are dropped: the contract has no such fields, and the
// fleet derives the identity from the stake.
func (r *Request) Encode() ([][]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	// The WALLET's encoding, because the wallet is what writes requests:
	// a bare action marker followed by its fields as sorted key=value
	// parts (core/wallet internal/staking.FormatActions). Its
	// action_test.go golden vectors are the byte-level contract and are
	// mirrored in wallet_vectors_test.go here.
	//
	// This used to emit a JSON object — {"actionType":"register",...} — a
	// dialect nothing else spoke. Round-tripping a wallet entry through
	// Parse then Encode produced bytes the wallet could not read, which is
	// the fork this package exists to prevent.
	fields := map[string]string{}
	put := func(k, v string) {
		if v != "" {
			fields[k] = v
		}
	}
	switch r.Kind {
	case KindRegister:
		put("account", r.subjectAccount())
		put("type", r.Type)
		put("payout", r.Rewards)
		put("delegate", r.Delegate)
	case KindWithdraw, KindTransfer:
		put("account", r.Account)
		put("recipient", r.Destination)
		put("amount", r.Amount)
	case KindUnstake:
		put("account", r.Account)
	case KindChangePayout:
		put("account", r.Account)
		put("destination", r.Destination)
	case KindChangeDelegate:
		put("account", r.Account)
		put("delegate", r.Delegate)
	case KindChangeType:
		put("account", r.Account)
		put("type", r.Type)
	case KindChangeDelegatorPayout:
		put("identity", r.Identity)
		put("destination", r.Destination)
	case KindRejectDelegates, KindRegisterIdentity:
		put("identity", r.Identity)
	case KindCancelRequest:
		// Whichever form the caller supplied; Validate has already
		// established that one of them is complete.
		put("request", r.RequestTx)
		put("account", r.Account)
		put("amount", r.Amount)
		put("destination", r.Destination)
	default:
		return nil, fmt.Errorf("cannot encode %q: not an action the wallet defines", r.Kind)
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := [][]byte{[]byte(r.Kind.Marker())}
	for _, k := range keys {
		parts = append(parts, []byte(k+"="+fields[k]))
	}
	return parts, nil
}

// subjectAccount is the account a registration names. Legacy entries put
// it in Stake, the wallet in Account; either is the same account.
func (r *Request) subjectAccount() string {
	if r.Stake != "" {
		return r.Stake
	}
	return r.Account
}

// Parse decodes an entry of the requests account in any era.
//
// It returns ErrNotARequest for entries that were never requests — the
// historical announcements, test entries and binary blobs that do not lead with
// a recognisable staking request. Those are not malformed requests and should
// not be reported as refusals.
//
// A returned Request may still fail Validate: parsing recognises the shape,
// validation judges whether the fleet will act on it. The two are separate on
// purpose, so a caller can show a badly-formed request AND say what is wrong
// with it.
func Parse(parts [][]byte) (*Request, error) {
	if r, ok := parseContract(parts); ok {
		r.Payloads = len(parts)
		return r, nil
	}
	if r, ok := parseLegacy(parts); ok {
		// Legacy entries spread one request across several key=value
		// payloads by design; the one-payload rule is the CONTRACT's.
		r.Payloads = 1
		return r, nil
	}
	return nil, ErrNotARequest
}

// parseContract reads the current encoding: exactly one payload, a JSON object
// with a recognised actionType.
func parseContract(parts [][]byte) (*Request, bool) {
	if len(parts) != 1 {
		// Several contract payloads in one entry is a refusal, but it is still
		// the contract era, so report it as such rather than falling through to
		// the legacy reader.
		if r, ok := parseMultiContract(parts); ok {
			return r, true
		}
		return nil, false
	}
	return parseOneContract(parts[0])
}

// contractFields are the only top-level keys the contract defines.
// fieldHints map the near-misses people actually type to the field they
// meant — the legacy aliases, mostly.
var (
	contractFields = map[string]bool{"actiontype": true, "account": true, "destination": true,
		"amount": true, "type": true, "stake": true, "rewards": true, "delegate": true}
	fieldHints = map[string]string{
		"payout": "rewards", "awards": "rewards", "rewardsaccount": "rewards",
		"recipient": "destination", "staking_account": "account", "stakingaccount": "account",
		"delegate_to": "delegate", "action": "actionType",
	}
)

func parseOneContract(part []byte) (*Request, bool) {
	var e contractEntry
	if err := json.Unmarshal(part, &e); err != nil {
		return nil, false
	}
	kind, known := KindOf(e.ActionType)
	if !known {
		return nil, false
	}
	// Collect keys the contract does not define. json.Unmarshal ignores
	// them silently, which is exactly the trap: see Request.UnknownFields.
	var unknown []string
	var raw map[string]json.RawMessage
	if json.Unmarshal(part, &raw) == nil {
		for k := range raw {
			if !contractFields[strings.ToLower(k)] {
				unknown = append(unknown, k)
			}
		}
		sort.Strings(unknown)
	}
	r := &Request{
		Kind:          kind,
		Account:       e.Account,
		Destination:   e.Destination,
		Amount:        e.Amount,
		Stake:         e.Stake,
		Type:          e.Type,
		Rewards:       e.Rewards,
		Delegate:      e.Delegate,
		Era:           EraContract,
		UnknownFields: unknown,
	}
	return r, true
}

// parseMultiContract recognises an entry of several contract payloads. The
// fleet refuses these, but they belong to the contract era; reporting the first
// payload lets a caller say what was attempted.
func parseMultiContract(parts [][]byte) (*Request, bool) {
	if len(parts) < 2 {
		return nil, false
	}
	for _, p := range parts {
		if _, ok := parseOneContract(p); !ok {
			return nil, false
		}
	}
	return parseOneContract(parts[0])
}
