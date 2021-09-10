package validator

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/martinlindhe/base36"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func CreateFakeSyntheticDeposit(t *testing.T, tokenUrl string, from string, to string, kp ed25519.PrivKey) *proto.Submission {
	deposit := synthetic.NewTokenTransactionDeposit()
	deposit.SourceChainId = *types.GetIdentityChainFromIdentity(&from)
	deposit.SourceAdiChain = *types.GetChainIdFromChainPath(&from)
	deposit.Txid = sha256.Sum256([]byte("generictxid"))
	deposit.DepositAmount.SetInt64(5000)
	deposit.SetTokenInfo(tokenUrl)

	data, err := json.Marshal(&deposit)
	if err != nil {
		return nil
	}

	ts := time.Now().Unix()
	ledger := types.MarshalBinaryLedgerChainId(deposit.SourceAdiChain.Bytes(), data, ts)
	sig, err := kp.Sign(ledger)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Data(data).
		AdiUrl(to).
		Timestamp(ts).
		PubKey(kp.PubKey().Bytes()).
		Signature(sig).Instruction(proto.AccInstruction_Synthetic_Token_Deposit).
		Build()

	if err != nil {
		t.Fatalf("Failed to make a token rpc call %v", err)
	}

	return sub
}

func CreateFakeAnonymousTokenChain(addressUrl string) *state.Object {
	adi, _, _ := types.ParseIdentityChainPath(&addressUrl)

	anonTokenChain := state.NewChain(types.String(adi), api.ChainTypeAnonTokenAccount[:])

	so := state.Object{}
	so.Entry, _ = anonTokenChain.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so
}
func MakeAnonymousAddress(key []byte) string {

	keyHash := sha256.Sum256(key)
	checkSum := sha256.Sum256(keyHash[:20])
	addrBytes := append(keyHash[:20], checkSum[:4]...)
	//generate the address from the key hash.
	address := fmt.Sprintf("0x%x", addrBytes)
	return address
}

func CreateFakeAnonTokenState(adiChainPath string, key ed25519.PrivKey) (*state.Object, []byte) {

	id, _, _ := types.ParseIdentityChainPath(&adiChainPath)

	idhash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	ids.SetKeyData(state.KeyTypeSha256, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idhash[:]
}

func TestAnonTokenChain_BVC(t *testing.T) {
	appId := sha256.Sum256(([]byte("anon")))

	bvc := NewBlockValidatorChain()
	kp := types.CreateKeyPair()
	address := MakeAnonymousAddress(kp.PubKey().Bytes())
	//anon := NewAnonTokenChain()
	tokenUrl := "roadrunner/MyAcmeTokens" //coinbase

	//todo: create fake deposit. or set an initial account state.
	//subDeposit := CreateFakeSyntheticDeposit(t, tokenUrl, tokenUrl, address, kp)
	//stateObject := CreateFakeAnonymousTokenChain(address)

	anonAccountUrl := address + "/" + tokenUrl
	subTx := CreateFakeTokenTransaction(t, anonAccountUrl, kp)

	stateDB := &state.StateDB{}
	stateDB.Open("/var/tmp", appId[:], true, true)

	se, err := state.NewStateEntry(nil, nil, stateDB)
	if err != nil {
		t.Fatal(err)
	}

	_, err = bvc.Validate(se, subTx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnonTokenChain_Validate(t *testing.T) {
	appId := sha256.Sum256(([]byte("anon")))
	kp := types.CreateKeyPair()
	address := MakeAnonymousAddress(kp.PubKey().Bytes())
	anon := NewAnonTokenChain()
	tokenUrl := "roadrunner/MyAcmeTokens" //coinbase
	subDeposit := CreateFakeSyntheticDeposit(t, tokenUrl, tokenUrl, address, kp)
	stateObject := CreateFakeAnonymousTokenChain(address)

	anonAccountUrl := address + "/" + tokenUrl
	subTx := CreateFakeTokenTransaction(t, anonAccountUrl, kp)
	currentState := state.StateEntry{}
	stateDB := &state.StateDB{}
	stateDB.Open("/var/tmp", appId[:], true, true)
	currentState.DB = stateDB
	currentState.IdentityState = stateObject
	resp, err := anon.Validate(&currentState, subTx)

	if err == nil {
		t.Fatalf("expecting an error on state object not existing")
	}
	resp, err = anon.Validate(&currentState, subDeposit)

	if len(resp.StateData) == 0 {
		t.Fatalf("expecting state changes to be returned")
	}

	if resp.Submissions != nil {
		t.Fatalf("not expecting submissions")
	}

	//store the returned states
	for k, v := range resp.StateData {
		err := stateDB.AddStateEntry(k[:], v)
		if err != nil {
			t.Fatalf("error storing state for chainId %x, %v", k, v)
		}
	}

	fmt.Printf("address = %s, chain spec %s\n", address, *anon.GetChainSpec())
	resp, err = anon.Validate(&currentState, subTx)

	if err != nil {
		t.Fatalf("fail for subTx, %v", err)
	}
	//now we can try the transaction again to see if it works now...

	//anon.Check()
}

//GenerateTestAddresses generate some test addresses to make sure the base36 prefix is fixed
func GenerateTestAddresses(seed uint32, checkLen uint32, t *testing.T) {
	sum := 0.0
	sqSum := 0.0

	j := 0
	var bucketsBig [30]byte

	numNetworks := uint64(len(bucketsBig))

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], seed)

	maxAddresses := 20000
	for j < maxAddresses {
		kp := types.CreateKeyPair()
		keyhash := sha256.Sum256(kp.PubKey().Bytes())

		result := append(prefix[:], keyhash[:]...)
		hash1 := sha256.Sum256(result)
		hash2 := sha256.Sum256(hash1[:])
		encodeThis := append(result, hash2[:checkLen]...)

		addr := base36.EncodeBytes(encodeThis)

		//idx := base36.Decode(addr) & 0xFFFF
		//acc45if1
		bn := big.NewInt(0)
		bn.SetBytes(keyhash[:])
		mod := big.NewInt(int64(numNetworks))

		bn.Mod(bn, mod)

		indexBig := bn.Uint64()

		addr = strings.ToLower(addr)

		if !strings.HasPrefix(addr, "acc45") {
			t.Fatalf("expecting acc45 prefix, but received %s", []byte(addr)[:5])
		}
		sum += float64(indexBig)
		sqSum += float64(indexBig) * float64(indexBig)

		bucketsBig[indexBig]++

		j++
	}

	mean := sum / float64(maxAddresses)
	variance := sqSum/float64(maxAddresses) - mean*mean
	stddev := math.Sqrt(variance)

	fmt.Printf("Distribution Big   : %v\n", bucketsBig)

	fmt.Printf("Big: sigma %f, mean %f, variance %f, 3sigma %f\n", stddev, mean, variance, 3*stddev)
}
