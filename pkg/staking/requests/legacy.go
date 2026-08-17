// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package requests

import "strings"

// The pre-contract encoding. Entries in this shape are all over
// acc://staking.acme/requests: a bare action marker followed by key=value
// parts, several payloads per entry, with the field names drifting between
// eras. They were acted on by the pipeline of their day, so reading them is not
// archaeology — it is how a caller shows a staker their own history without
// calling it malformed.
//
// This reader mirrors parseRequestEntry in core/staking
// pkg/mcpserver/chain_requests.go, including its era heuristics. Where the two
// disagree the staking side wins and this should be corrected.
//
// Nothing here is ever written. See Encode.

// legacyAliases are the field names each era used for the same thing, in
// priority order — earlier names win when an entry carries several.
var (
	aliasAccount   = []string{"account", "stake", "staking_acc", "staking_account", "stakingaccount"}
	aliasPayout    = []string{"payout", "rewards", "awards", "rewardsaccount"}
	aliasDelegate  = []string{"delegate", "delegate_to"}
	aliasRecipient = []string{"recipient", "destination"}
	aliasRequestTx = []string{"request_txid", "request_tx_hash", "ref_transaction", "transaction"}
	aliasAction    = []string{"actiontype", "action"}
)

// legacyMarkers maps a bare action marker to the kind it meant. The oldest era
// wrote "withdraw" for what later became "withdrawTokens".
var legacyMarkers = map[string]Kind{
	"addaccount":     KindRegister,
	"withdrawtokens": KindWithdraw,
	"withdraw":       KindWithdraw,
}

// parseLegacy reads the pre-contract encoding. The bool reports whether the
// entry is a request at all.
func parseLegacy(parts [][]byte) (*Request, bool) {
	fields, markers := splitLegacy(parts)

	get := func(keys ...string) string {
		for _, k := range keys {
			// "<nil>" is a real value on chain: an era that formatted a nil
			// pointer into the entry rather than omitting the field.
			if v := fields[k]; v != "" && v != "<nil>" {
				return v
			}
		}
		return ""
	}

	kind, ok := legacyKind(get(aliasAction...), markers)
	account := get(aliasAccount...)

	// The oldest registrations carry no action marker at all — just identity,
	// type, stake, rewards and delegate. A stake plus a type or a delegate is a
	// registration.
	if !ok && account != "" && (get("type") != "" || get(aliasDelegate...) != "") {
		kind, ok = KindRegister, true
	}
	if !ok || account == "" {
		return nil, false
	}

	switch kind {
	case KindRegister:
		return &Request{
			Kind:      KindRegister,
			Stake:     account,
			Type:      canonicalOrRaw(get("type")),
			Rewards:   get(aliasPayout...),
			Delegate:  get(aliasDelegate...),
			Identity:  get("identity"),
			RequestTx: get(aliasRequestTx...),
			Era:       EraLegacy,
		}, true

	case KindWithdraw:
		// A withdrawal with no amount is not a withdrawal — it is a notice
		// about one. The staking side draws the same line.
		amount := get("amount")
		if amount == "" {
			return nil, false
		}
		return &Request{
			Kind:        KindWithdraw,
			Account:     account,
			Amount:      NormalizeAmount(amount),
			Destination: get(aliasRecipient...),
			RequestTx:   get(aliasRequestTx...),
			Era:         EraLegacy,
		}, true
	}
	return nil, false
}

// splitLegacy divides an entry's parts into key=value fields and bare markers.
// Keys are lower-cased; values and markers keep their case but lose the
// surrounding quotes some eras wrote.
func splitLegacy(parts [][]byte) (fields map[string]string, markers []string) {
	fields = map[string]string{}
	for _, raw := range parts {
		part := cleanLegacy(string(raw))
		if part == "" {
			continue
		}
		// A key must be non-empty and space-free, or "hello = world" would read
		// as a field.
		if i := strings.Index(part, "="); i > 0 && !strings.ContainsAny(part[:i], " \t") {
			fields[strings.ToLower(part[:i])] = cleanLegacy(part[i+1:])
			continue
		}
		markers = append(markers, part)
	}
	return fields, markers
}

func cleanLegacy(s string) string {
	return strings.Trim(strings.TrimSpace(s), `“”"`)
}

// legacyKind resolves the action from an explicit actionType field or, failing
// that, from a bare marker.
func legacyKind(action string, markers []string) (Kind, bool) {
	if action != "" {
		// Some eras wrote the contract's own names as a field value.
		switch a := strings.ToLower(action); {
		case strings.HasPrefix(a, "add"), a == string(KindRegister):
			return KindRegister, true
		case strings.HasPrefix(a, "withdraw"):
			return KindWithdraw, true
		}
	}
	for _, m := range markers {
		if k, ok := legacyMarkers[strings.ToLower(m)]; ok {
			return k, true
		}
	}
	return "", false
}

// canonicalOrRaw canonicalises a staking class, keeping the original text when
// it is not one. Parsing records what the entry said; Validate judges it.
func canonicalOrRaw(class string) string {
	if c, ok := CanonicalClass(class); ok {
		return c
	}
	return class
}

// NormalizeAmount renders the withdrawal-amount eras as an ACME decimal string.
// Mirrors normalizeRequestAmount in core/staking's pkg/mcpserver/chain_requests.go:
//
//	"500024.5136254"  already ACME
//	"3,950,000"       ACME with thousands separators
//	"10000000000000"  base units — an integer of 11+ digits can only be base
//	                  units, since no single withdrawal approaches 10 billion ACME
func NormalizeAmount(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || strings.Contains(s, ".") {
		return s
	}
	if len(s) < 11 {
		return s
	}
	whole, frac := s[:len(s)-8], s[len(s)-8:]
	if frac = strings.TrimRight(frac, "0"); frac == "" {
		return whole
	}
	return whole + "." + frac
}
