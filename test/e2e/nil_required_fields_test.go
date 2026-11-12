// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestNilRequiredFields submits every user transaction with each of its
// nillable fields cleared, one at a time, and requires that the network
// refuses or executes it without panicking. A panic while validating a
// transaction halts the node; #4646 (TransferCredits with a recipient that has
// no URL) kept Kermit down for a day.
//
// Each variant is sent twice: after a binary round trip, which is what a peer
// or CometBFT delivers (an absent field decodes as nil, but a list element
// never does), and after a JSON round trip, which is what the API decodes (a
// null list element decodes as nil).
func TestNilRequiredFields(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	book := alice.JoinPath("book")
	page := alice.JoinPath("book", "1")
	tokens := alice.JoinPath("tokens")
	data := alice.JoinPath("data")
	issuer := alice.JoinPath("issuer")
	keyHash := make([]byte, 32)
	keyHash[0] = 1
	liteData, err := LiteDataAddress(make([]byte, 32))
	require.NoError(t, err)
	lockedTx := alice.WithTxID([32]byte{1})

	// Each variant gets a fresh network, so one that succeeds (an UpdateKey
	// that replaces alice's key, say) cannot hide the variants after it
	var sim *Sim
	reset := func() {
		sim = NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), page, 1e12)
		MakeAccount(t, sim.DatabaseFor(alice),
			&TokenAccount{Url: tokens, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)},
			&DataAccount{Url: data},
			&TokenIssuer{Url: issuer, Symbol: "FOO", Precision: 2})
	}

	data1 := func() DataEntry { return &DoubleHashDataEntry{Data: [][]byte{[]byte("x")}} }
	spec := func() *KeySpecParams { return &KeySpecParams{KeyHash: keyHash, Delegate: book} }

	cases := []struct {
		principal *url.URL
		body      func() TransactionBody
	}{
		{alice, func() TransactionBody {
			return &CreateIdentity{Url: alice.JoinPath("sub"), KeyHash: keyHash, KeyBookUrl: alice.JoinPath("sub", "book"), Authorities: []*url.URL{book}}
		}},
		{AccountUrl("bob"), func() TransactionBody {
			return &CreateIdentity{Url: AccountUrl("bob"), KeyHash: keyHash, KeyBookUrl: AccountUrl("bob", "book")}
		}},
		{alice, func() TransactionBody {
			return &CreateTokenAccount{Url: alice.JoinPath("tokens2"), TokenUrl: AcmeUrl(), Authorities: []*url.URL{book}}
		}},
		{alice, func() TransactionBody {
			return &CreateTokenAccount{Url: alice.JoinPath("tokens3"), TokenUrl: AccountUrl("other", "tok"),
				Proof: &TokenIssuerProof{Transaction: &CreateToken{Url: AccountUrl("other", "tok"), Symbol: "X"}, Receipt: &merkle.Receipt{Start: keyHash, Anchor: keyHash}}}
		}},
		{tokens, func() TransactionBody {
			return &SendTokens{To: []*TokenRecipient{{Url: alice.JoinPath("tokens2"), Amount: *big.NewInt(1)}}}
		}},
		{alice, func() TransactionBody {
			return &CreateDataAccount{Url: alice.JoinPath("data2"), Authorities: []*url.URL{book}}
		}},
		{data, func() TransactionBody { return &WriteData{Entry: data1()} }},
		{tokens, func() TransactionBody { return &WriteDataTo{Recipient: liteData, Entry: data1()} }},
		{FaucetUrl, func() TransactionBody { return &AcmeFaucet{Url: liteData} }},
		{alice, func() TransactionBody {
			return &CreateToken{Url: alice.JoinPath("tok"), Symbol: "TOK", Properties: alice.JoinPath("props"), SupplyLimit: big.NewInt(1e9), Authorities: []*url.URL{book}}
		}},
		{issuer, func() TransactionBody {
			return &IssueTokens{Recipient: tokens, Amount: *big.NewInt(1), To: []*TokenRecipient{{Url: tokens, Amount: *big.NewInt(1)}}}
		}},
		{book, func() TransactionBody { return &CreateKeyPage{Keys: []*KeySpecParams{spec()}} }},
		{alice, func() TransactionBody {
			return &CreateKeyBook{Url: alice.JoinPath("book2"), PublicKeyHash: keyHash, Authorities: []*url.URL{book}}
		}},
		{tokens, func() TransactionBody { return &AddCredits{Recipient: page, Amount: *big.NewInt(1e8), Oracle: 5000} }},
		{page, func() TransactionBody { return &TransferCredits{To: []*CreditRecipient{{Url: page, Amount: 1}}} }},
		{page, func() TransactionBody { return &UpdateKey{NewKeyHash: keyHash} }},
		{alice, func() TransactionBody {
			return &NetworkMaintenance{Operations: []NetworkMaintenanceOperation{&PendingTransactionGCOperation{Account: tokens}}}
		}},
		{tokens, func() TransactionBody {
			return &SetLiteAccountDelegate{Delegate: book, Proof: &KeyBookProof{KeyBook: &KeyBook{Url: book}, Receipt: &merkle.Receipt{Start: keyHash, Anchor: keyHash}}}
		}},
		{tokens, func() TransactionBody { return &ReleaseLockedOperation{LockedTxID: lockedTx, Preimage: []byte("x")} }},
	}

	// One operation per transaction, so an operation that fails early does not
	// hide a later one
	for _, op := range []func() KeyPageOperation{
		func() KeyPageOperation { return &AddKeyOperation{Entry: *spec()} },
		func() KeyPageOperation { return &RemoveKeyOperation{Entry: KeySpecParams{KeyHash: aliceKey[32:]}} },
		func() KeyPageOperation {
			return &UpdateKeyOperation{OldEntry: KeySpecParams{KeyHash: aliceKey[32:]}, NewEntry: *spec()}
		},
		func() KeyPageOperation { return &SetThresholdKeyPageOperation{Threshold: 1} },
		func() KeyPageOperation {
			return &UpdateAllowedKeyPageOperation{Allow: []TransactionType{TransactionTypeSendTokens}}
		},
	} {
		op := op
		cases = append(cases, struct {
			principal *url.URL
			body      func() TransactionBody
		}{page, func() TransactionBody { return &UpdateKeyPage{Operation: []KeyPageOperation{op()}} }})
	}
	for _, op := range []func() AccountAuthOperation{
		func() AccountAuthOperation { return &EnableAccountAuthOperation{Authority: book} },
		func() AccountAuthOperation { return &DisableAccountAuthOperation{Authority: book} },
		func() AccountAuthOperation { return &AddAccountAuthorityOperation{Authority: alice.JoinPath("book2")} },
		func() AccountAuthOperation { return &RemoveAccountAuthorityOperation{Authority: book} },
	} {
		op := op
		cases = append(cases, struct {
			principal *url.URL
			body      func() TransactionBody
		}{tokens, func() TransactionBody { return &UpdateAccountAuth{Operations: []AccountAuthOperation{op()}} }})
	}

	// Every user transaction type with no fields set at all, which also covers
	// the bodies the list above does not build.
	for typ := TransactionType(1); typ < TransactionTypeRemote; typ++ {
		if !typ.IsUser() {
			continue
		}
		typ := typ
		if _, err := NewTransactionBody(typ); err != nil {
			continue
		}
		cases = append(cases, struct {
			principal *url.URL
			body      func() TransactionBody
		}{tokens, func() TransactionBody { b, _ := NewTransactionBody(typ); return b }})
	}

	var timestamp uint64
	var panics []string
	submit := func(name string, principal *url.URL, mk func() TransactionBody, clear func(TransactionBody)) {
		for _, wire := range []string{"binary", "json"} {
			label := fmt.Sprintf("%v %s (%s)", mk().Type(), name, wire)
			func() {
				stage := "build"
				defer func() {
					if r := recover(); r != nil {
						stack := string(debug.Stack())
						panics = append(panics, fmt.Sprintf("%s: panic during %s: %v\n%s", label, stage, r, firstFrames(stack)))
					}
				}()

				reset()
				sign := func(body TransactionBody) (env *messaging.Envelope, err error) {
					defer func() {
						if r := recover(); r != nil {
							env, err = nil, fmt.Errorf("client panicked: %v", r)
						}
					}()
					timestamp++
					version := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page).Version
					return build.Transaction().For(principal).Body(body).
						SignWith(page).Version(version).Timestamp(timestamp).PrivateKey(aliceKey).Done()
				}

				body := mk()
				if clear != nil {
					clear(body)
				}
				env, err := sign(body)
				if err != nil && clear != nil {
					// The client cannot hash or sign this body. A node can
					// still be sent it, so sign the intact body and clear the
					// field afterwards: the node meets the nil before it
					// finds the signature does not match.
					env, err = sign(mk())
					if err == nil {
						clear(env.Transaction[0].Body)
						label += " [cleared after signing]"
					}
				}
				if err != nil {
					return // The client refused to build it; nothing reaches a node
				}

				stage = "encode"
				env = roundTrip(t, env, wire)
				if env == nil {
					return // Cannot be encoded, so cannot be sent this way
				}

				stage = "submit"
				_, _ = sim.SubmitRaw(env)

				stage = "execute"
				sim.StepN(3)
			}()
		}
	}

	for _, c := range cases {
		submit("as built", c.principal, c.body, nil)
		for _, v := range nilVariants(c.body) {
			submit(v.path+"=nil", c.principal, c.body, v.clear)
		}
	}

	// The network still works
	reset()
	timestamp++
	version := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page).Version
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).TransferCredits(1).To(page).
			SignWith(page).Version(version).Timestamp(timestamp).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	if len(panics) > 0 {
		t.Fatalf("%d variants panicked:\n\n%s", len(panics), strings.Join(panics, "\n"))
	}
}

func roundTrip(t *testing.T, env *messaging.Envelope, wire string) (out *messaging.Envelope) {
	out = new(messaging.Envelope)
	switch wire {
	case "binary":
		defer func() {
			if recover() != nil {
				out = nil // The encoder cannot write it, so no peer can send it
			}
		}()
		b, err := env.MarshalBinary()
		if err != nil {
			return nil
		}
		if out.UnmarshalBinary(b) != nil {
			return nil
		}
	case "json":
		b, err := json.Marshal(env)
		if err != nil {
			return nil
		}
		if json.Unmarshal(b, out) != nil {
			return nil
		}

		// The API forwards what it decoded in binary. If the encoder cannot
		// write it, no node receives it. (The encoder panicking on a nil
		// union element is an API-side failure, followed up on #4646.)
		if !encodes(out) {
			return nil
		}
	default:
		t.Fatalf("unknown wire %q", wire)
	}
	return out
}

func encodes(env *messaging.Envelope) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	_, err := env.MarshalBinary()
	return err == nil
}

type nilVariant struct {
	path  string
	clear func(TransactionBody)
}

// nilVariants returns one variant per nillable location of the body (a
// pointer, interface, or slice field, or a pointer or interface element of a
// slice); its clear function sets that location to nil.
func nilVariants(mk func() TransactionBody) []nilVariant {
	var paths []string
	walkNillable(reflect.ValueOf(mk()), "", func(p string, _ reflect.Value) { paths = append(paths, p) })

	out := make([]nilVariant, len(paths))
	for i, p := range paths {
		i := i
		out[i] = nilVariant{p, func(body TransactionBody) {
			var n int
			walkNillable(reflect.ValueOf(body), "", func(_ string, v reflect.Value) {
				if n == i {
					v.Set(reflect.Zero(v.Type()))
				}
				n++
			})
		}}
	}
	return out
}

var (
	leafURL  = reflect.TypeOf((*url.URL)(nil))
	leafTxID = reflect.TypeOf((*url.TxID)(nil))
	leafBig  = reflect.TypeOf((*big.Int)(nil))
)

func walkNillable(v reflect.Value, path string, visit func(string, reflect.Value)) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		if v.Kind() == reflect.Pointer && (v.Type() == leafURL || v.Type() == leafTxID || v.Type() == leafBig) {
			return
		}
		walkNillable(v.Elem(), path, visit)

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			fv := v.Field(i)
			p := strings.TrimPrefix(path+"."+f.Name, ".")
			switch fv.Kind() {
			case reflect.Pointer, reflect.Interface:
				if !fv.IsNil() {
					visit(p, fv)
				}
				walkNillable(fv, p, visit)
			case reflect.Slice:
				if fv.IsNil() {
					continue
				}
				visit(p, fv)
				for j := 0; j < fv.Len(); j++ {
					ev := fv.Index(j)
					ep := fmt.Sprintf("%s[%d]", p, j)
					if ev.Kind() == reflect.Pointer || ev.Kind() == reflect.Interface {
						if !ev.IsNil() {
							visit(ep, ev)
						}
						walkNillable(ev, ep, visit)
					}
				}
			case reflect.Struct:
				walkNillable(fv, p, visit)
			}
		}
	}
}

// firstFrames returns the frames of a stack inside the module, skipping the
// runtime and this test's recover.
func firstFrames(stack string) string {
	var out []string
	lines := strings.Split(stack, "\n")
	for i := 0; i < len(lines) && len(out) < 8; i++ {
		l := strings.TrimSpace(lines[i])
		if strings.Contains(l, "accumulate/") && strings.Contains(l, ".go:") &&
			!strings.Contains(l, "nil_required_fields_test.go") && !strings.Contains(l, "/test/") {
			out = append(out, "    "+l)
		}
	}
	return strings.Join(out, "\n")
}

// TestNilHeaderAuthority sends a transaction whose header names an additional
// authority with no URL. A nil in that list survives a binary round trip, and
// checking whether the transaction is ready used to read it.
func TestNilHeaderAuthority(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	page := alice.JoinPath("book", "1")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), page, 1e12)

	txn := &Transaction{
		Header: TransactionHeader{Principal: page, Authorities: []*url.URL{nil}},
		Body:   &TransferCredits{To: []*CreditRecipient{{Url: page, Amount: 1}}},
	}
	env, err := build.SignatureForTransaction(txn).
		Url(page).Version(1).Timestamp(1).PrivateKey(aliceKey).Done()
	require.NoError(t, err)
	b, err := env.MarshalBinary()
	require.NoError(t, err)
	env = new(messaging.Envelope)
	require.NoError(t, env.UnmarshalBinary(b))
	require.Len(t, env.Transaction[0].Header.Authorities, 1)
	require.Nil(t, env.Transaction[0].Header.Authorities[0])

	_, _ = sim.SubmitRaw(env)
	sim.StepN(5)
}
