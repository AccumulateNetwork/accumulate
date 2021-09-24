package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func createIdentityCreateSubmission(t *testing.T, adiChainPath string) (*state.StateEntry, *transactions.GenTransaction, *ed25519.PrivKey) {
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
	data, err := ic.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	gtx := &transactions.GenTransaction{}
	gtx.ChainID = chainid[:]
	gtx.Routing = types.GetAddressFromIdentityChain(identityhash)
	gtx.Routing = types.GetAddressFromIdentity(&adiChainPath)
	gtx.ChainID = types.GetChainIdFromChainPath(&adiChainPath).Bytes()
	gtx.SigInfo = &transactions.SignatureInfo{}
	gtx.SigInfo.URL = adiChainPath
	gtx.Transaction = data
	ed := new(transactions.ED25519Sig)
	ed.Sign(1, kp.Bytes(), gtx.TransactionHash())
	gtx.Signature = append([]*transactions.ED25519Sig{}, ed)
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

	txid := sub.TransactionHash()

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
