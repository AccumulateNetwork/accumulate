package validator

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func createTokenIssuanceSubmission(t *testing.T, adiChainPath string) (currentState *state.StateEntry, sub *transactions.GenTransaction) {
	kp := types.CreateKeyPair()

	currentState = &state.StateEntry{}

	//currentstate.ChainState = CreateFakeTokenAccountState(identitychainpath,t)

	currentState.IdentityState, _ = CreateFakeIdentityState(adiChainPath, kp)

	ti := api.NewToken(adiChainPath, "ACME", 1)

	data, err := ti.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	sub.Routing = types.GetAddressFromIdentity(&adiChainPath)
	sub.ChainID = types.GetChainIdFromChainPath(&adiChainPath).Bytes()
	sub.SigInfo = &transactions.SignatureInfo{}
	sub.SigInfo.URL = adiChainPath
	sub.Transaction = data
	ed := new(transactions.ED25519Sig)
	ed.Sign(1, kp.Bytes(), sub.TransactionHash())

	return currentState, sub
}

func TestTokenIssuanceValidator_Check(t *testing.T) {
	tiv := NewTokenIssuanceValidator()
	identitychainpath := "RoadRunner/ACME"
	currentstate, sub := createTokenIssuanceSubmission(t, identitychainpath)

	err := tiv.Check(currentstate, sub)

	if err != nil {
		t.Fatal(err)
	}
}

func TestTokenIssuanceValidator_Validate(t *testing.T) {
	//	kp := types.CreateKeyPair()
	tiv := NewTokenIssuanceValidator()
	identitychainpath := "RoadRunner/ACME"
	currentstate, sub := createTokenIssuanceSubmission(t, identitychainpath)

	resp, err := tiv.Validate(currentstate, sub)

	if err != nil {
		t.Fatal(err)
	}

	if resp.Submissions != nil {
		t.Fatalf("expecting no synthetic transactions")
	}

	if resp.StateData == nil {
		t.Fatal("expecting a state object to be returned to add to a token coinbase chain")
	}

	ti := state.Token{}
	chainid := types.GetChainIdFromChainPath(&identitychainpath)
	if resp.StateData == nil {
		t.Fatal("expecting state object from token transaction")
	}

	val, ok := resp.StateData[*chainid]
	if !ok {
		t.Fatalf("token transaction account chain not found %s", identitychainpath)
	}
	err = ti.UnmarshalBinary(val)

	if err != nil {
		t.Fatal(err)
	}
}
