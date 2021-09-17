package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"
	//"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"testing"
)

func createIdentityCreateSubmission(t *testing.T, adiChainPath string) (*state.StateEntry, *proto.GenTransaction, *ed25519.PrivKey) {
	kp := types.CreateKeyPair()
	identityhash := types.GetIdentityChainFromIdentity(&adiChainPath).Bytes()

	currentstate := state.StateEntry{}

	//currentstate.ChainState = CreateFakeTokenAccountState(adiChainPath,t)

	currentstate.IdentityState, identityhash = CreateFakeIdentityState(adiChainPath, kp)

	chainid := types.GetChainIdFromChainPath(&adiChainPath).Bytes()

	var keyhash types.Bytes32
	keyhash = types.Bytes32(sha256.Sum256(kp.PubKey().Bytes()))
	tokenSymbol := "ACME"
	ic := api.NewADI(&tokenSymbol, &keyhash)
	if ic == nil {
		t.Fatalf("Identity Create is nil")
	}

	//build a submission message
	sub := proto.Submission{}
	sub.Data, _ = json.Marshal(ic)

	gtx := &proto.GenTransaction{}
	gtx.ChainID = chainid[:]
	gtx.Routing = types.GetAddressFromIdentityChain(identityhash)

	return &currentstate, gtx, &kp
}

func TestIdentityCreateValidator_Check(t *testing.T) {
	tiv := NewAdiChain()
	adiChainPath := "RoadRunner/ACME"
	currentstate, sub, _ := createIdentityCreateSubmission(t, adiChainPath)

	err := tiv.Check(currentstate, sub)

	if err != nil {
		t.Fatal(err)
	}
}

func TestIdentityCreateValidator_Validate(t *testing.T) {
	//	kp := types.CreateKeyPair()
	tiv := NewAdiChain()
	adiChainPath := "RoadRunner/ACME"

	currentstate, sub, kp := createIdentityCreateSubmission(t, adiChainPath)

	txid := sub.TxId()

	resp, err := tiv.Validate(currentstate, sub)

	if err != nil {
		t.Fatal(err)
	}

	if resp.Submissions == nil {
		t.Fatalf("expecting a synthetic transactions")
	}

	if resp.StateData != nil {
		t.Fatal("Not expecting a state object to be returned to create an identity")
	}

	if len(resp.Submissions) != 1 {
		t.Fatalf("expecting only 1 synthetic transaction request")
	}

	sub = resp.Submissions[0]

	isc := synthetic.AdiStateCreate{}
	err = json.Unmarshal(sub.Transaction, &isc)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(isc.Txid[:], txid[:]) != 0 {
		t.Fatalf("Invalid transaction id in synth tx")
	}

	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	if bytes.Compare(isc.PublicKeyHash[:], keyhash[:]) != 0 {
		t.Fatalf("Invalid public key data stored")
	}
}
