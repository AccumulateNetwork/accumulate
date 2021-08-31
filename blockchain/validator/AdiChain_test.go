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
	"time"
)

func createIdentityCreateSubmission(t *testing.T, identitychainpath string) (*state.StateEntry, *proto.Submission, *ed25519.PrivKey) {
	kp := types.CreateKeyPair()
	identityhash := types.GetIdentityChainFromIdentity(identitychainpath).Bytes()

	currentstate := state.StateEntry{}

	//currentstate.ChainState = CreateFakeTokenAccountState(identitychainpath,t)

	currentstate.IdentityState, identityhash = CreateFakeIdentityState(identitychainpath, kp)

	chainid := types.GetChainIdFromChainPath(identitychainpath).Bytes()

	var keyhash types.Bytes32
	keyhash = types.Bytes32(sha256.Sum256(kp.PubKey().Bytes()))
	ic := api.NewADI("ACME", &keyhash)
	if ic == nil {
		t.Fatalf("Identity Create is nil")
	}

	//build a submission message
	sub := proto.Submission{}
	sub.Data, _ = json.Marshal(ic)

	sub.Instruction = proto.AccInstruction_Identity_Creation
	sub.Chainid = chainid[:]
	sub.Identitychain = identityhash[:]
	sub.Timestamp = time.Now().Unix()

	return &currentstate, &sub, &kp
}

func TestIdentityCreateValidator_Check(t *testing.T) {
	tiv := NewAdiChain()
	identitychainpath := "RoadRunner/ACME"
	currentstate, sub, _ := createIdentityCreateSubmission(t, identitychainpath)

	err := tiv.Check(currentstate, sub.Identitychain, sub.Chainid, 0, 0, sub.Data)

	if err != nil {
		t.Fatal(err)
	}
}

func TestIdentityCreateValidator_Validate(t *testing.T) {
	//	kp := types.CreateKeyPair()
	tiv := NewAdiChain()
	identitychainpath := "RoadRunner/ACME"
	adi, chainpath, err := types.ParseIdentityChainPath(identitychainpath)
	adihash := types.GetIdentityChainFromIdentity(adi)
	chainid := types.GetChainIdFromChainPath(chainpath)

	currentstate, sub, kp := createIdentityCreateSubmission(t, identitychainpath)

	txid := sha256.Sum256(types.MarshalBinaryLedgerChainId(adihash.Bytes(), sub.Data, sub.Timestamp))

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
	err = json.Unmarshal(sub.Data, &isc)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(isc.SourceAdiChain[:], adihash[:]) != 0 {
		t.Fatalf("Invalid source identity in synth tx")
	}

	if bytes.Compare(isc.SourceChainId[:], chainid[:]) != 0 {
		t.Fatalf("Invalid source chain id in synth tx")
	}

	if bytes.Compare(isc.Txid[:], txid[:]) != 0 {
		t.Fatalf("Invalid transaction id in synth tx")
	}

	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	if bytes.Compare(isc.PublicKeyHash[:], keyhash[:]) != 0 {
		t.Fatalf("Invalid public key data stored")
	}
}
