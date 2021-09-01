package validator

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/martinlindhe/base36"
	"github.com/spaolacci/murmur3"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"
)

func CreateFakeSyntheticDeposit(t *testing.T, tokenUrl string, from string, to string, kp ed25519.PrivKey) *proto.Submission {
	deposit := synthetic.NewTokenTransactionDeposit()
	deposit.SourceChainId = *types.GetIdentityChainFromIdentity(from)
	deposit.SourceAdiChain = *types.GetChainIdFromChainPath(from)
	deposit.Txid = sha256.Sum256([]byte("generictxid"))
	deposit.DepositAmount.SetInt64(5000)
	deposit.SetTokenInfo(types.UrlChain(tokenUrl))

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
	adi, _, _ := types.ParseIdentityChainPath(addressUrl)

	anonTokenChain := state.NewChain(types.UrlChain(adi), api.ChainTypeAnonTokenAccount[:])

	so := state.Object{}
	so.Entry, _ = anonTokenChain.MarshalBinary()
	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
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

func CreateFakeAnonTokenState(identitychainpath string, key ed25519.PrivKey) (*state.Object, []byte) {

	id, _, _ := types.ParseIdentityChainPath(identitychainpath)

	idhash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	ids.SetKeyData(state.KeyTypeSha256, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idhash[:]
}

func TestAnonTokenChain_Validate(t *testing.T) {
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
	stateDB.Open("/var/tmp", true, true)
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
	var bucketsBig [32]byte
	var bucketsLittle [32]byte

	numNetworks := uint64(len(bucketsBig))

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], seed)

	maxAddresses := 20000
	for j < maxAddresses {
		kp := types.CreateKeyPair()
		keyhash := sha256.Sum256(kp.PubKey().Bytes())
		m := murmur3.New64()
		m.Write(kp.PubKey().Bytes())

		result := append(prefix[:], keyhash[:]...)
		hash1 := sha256.Sum256(result)
		hash2 := sha256.Sum256(hash1[:])
		encodeThis := append(result, hash2[:checkLen]...)
		//fmt.Printf("%v\n", encodeThis)

		addr := base36.EncodeBytes(encodeThis)

		//idx := base36.Decode(addr) & 0xFFFF
		//acc45if1
		bn := big.NewInt(0)
		bn.SetBytes(keyhash[:])
		mod := big.NewInt(int64(numNetworks))

		bn.Mod(bn, mod)

		indexBig := bn.Uint64() //binary.BigEndian.Uint64(networkid[:])
		indexLittle := uint64(indexBig)

		addr = strings.ToLower(addr)

		if !strings.HasPrefix(addr, "acc45") {
			t.Fatalf("expecting acc45 prefix, but received %s", []byte(addr)[:5])
		}
		sum += float64(indexBig)
		sqSum += float64(indexBig) * float64(indexBig)

		bucketsBig[indexBig]++
		bucketsLittle[indexLittle%numNetworks]++

		j++
	}

	mean := sum / float64(maxAddresses)
	variance := sqSum/float64(maxAddresses) - mean*mean
	stddev := math.Sqrt(variance)

	fmt.Printf("Distribution Big   : %v\n", bucketsBig)
	fmt.Printf("Distribution Little: %v\n", bucketsLittle)

	fmt.Printf("Big: sigma %f, mean %f, variance %f, 3sigma %f\n", stddev, mean, variance, 3*stddev)
}

func TestBase36AddressGenerationFromPubKeyHash(t *testing.T) {
	var pubkey types.Bytes32

	//generate base36 ADI from public key hash
	hex.Decode(pubkey[:], []byte("8301843BA7F7DE82901547EC1DC4F6505AB2BF078DB8F3460AE72D7B4250AD78"))
	keyhash := sha256.Sum256(pubkey[:])

	//all prefixes are little endian
	//34996a accumv 4 byte checksum len=61h
	//180d03 anon  4 byte checksum len=61h
	//1a5e6f at0kn 4 byte checksum 61h
	//15bc95 acctn 2 byte checksum 59h
	//2a0e15 acc45 3 byte checksum len=60h
	seed := [][]uint32{
		{uint32(0x2a0e15), 3}, //prefix=acc45 3 byte checksum len=60h
		{uint32(0x180d03), 4}, //prefix=anon  4 byte checksum len=61h
		{uint32(0x15bc95), 2}, //prefix=acctn 2 byte checksum len=59h
	}

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], seed[1][0])
	//	prefix := []byte{0x15, 0x0e, 0x2a, 0x00}
	result := append(prefix[:], keyhash[:]...)
	hash1 := sha256.Sum256(result)
	hash2 := sha256.Sum256(hash1[:])
	//our checksum is first 3 bytes of hash.
	encodeThis := append(result, hash2[:seed[1][1]]...)
	addr := base36.EncodeBytes(encodeThis)
	fmt.Println(strings.ToLower(addr))

	GenerateTestAddresses(seed[0][0], seed[0][1], t)

}
