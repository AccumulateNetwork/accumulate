package api

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func AssembleBVCSubmissionHeader(identityname string, chainpath string, ins proto.AccInstruction) *proto.Submission {
	sub := proto.Submission{}

	sub.Identitychain = types.GetIdentityChainFromAdi(identityname).Bytes()
	if chainpath == "" {
		chainpath = identityname
	}
	sub.Chainid = types.GetIdentityChainFromAdi(chainpath).Bytes()
	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
	sub.Instruction = ins
	return &sub
}

func MakeBVCSubmission(ins string, identityname string, chainpath string, payload []byte, timestamp int64, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	v := InstructionTypeMap[ins]
	if v == 0 {
		return nil
	}
	sub := AssembleBVCSubmissionHeader(identityname, chainpath, v)
	sub.Data = payload
	sub.Timestamp = timestamp
	sub.Signature = signature
	sub.Key = pubkey
	return sub
}

//fullchainpath == identityname/chainpath
func MarshalBinarySig(fullchainpath string, payload []byte, timestamp int64) []byte {
	var msg []byte

	//The chain path is either the identity name or the full chain path [identityname]/[chainpath]
	chainid := sha256.Sum256([]byte(fullchainpath))
	msg = append(msg, chainid[:]...)

	payloadhash := sha256.Sum256(payload)
	msg = append(msg, payloadhash[:]...)

	var tsbytes [8]byte
	binary.LittleEndian.PutUint64(tsbytes[:], uint64(timestamp))
	msg = append(msg, tsbytes[:]...)

	return msg
}

var InstructionTypeMap = map[string]proto.AccInstruction{
	"identity-create":      proto.AccInstruction_Identity_Creation,
	"idc":                  proto.AccInstruction_Identity_Creation,
	"token-url-create":     proto.AccInstruction_Token_URL_Creation,
	"url":                  proto.AccInstruction_Token_URL_Creation,
	"token-tx":             proto.AccInstruction_Token_Transaction,
	"tx":                   proto.AccInstruction_Token_Transaction,
	"data-chain-create":    proto.AccInstruction_Data_Chain_Creation,
	"dcc":                  proto.AccInstruction_Data_Chain_Creation,
	"data-entry":           proto.AccInstruction_Data_Entry,
	"de":                   proto.AccInstruction_Data_Entry,
	"scratch-chain-create": proto.AccInstruction_Scratch_Chain_Creation,
	"scc":                  proto.AccInstruction_Scratch_Chain_Creation,
	"scratch-entry":        proto.AccInstruction_Scratch_Entry,
	"se":                   proto.AccInstruction_Scratch_Entry,
	"token-issue":          proto.AccInstruction_Token_Issue,
	"ti":                   proto.AccInstruction_Token_Issue,
	"key-update":           proto.AccInstruction_Key_Update,
	"ku":                   proto.AccInstruction_Key_Update,
	"deep-query":           proto.AccInstruction_Deep_Query,
	"dq":                   proto.AccInstruction_Deep_Query,
	"query":                proto.AccInstruction_Light_Query,
	"q":                    proto.AccInstruction_Light_Query,
}


type Subparams struct {
	IdentityChainpath string `json:"identity-chainpath"`
	Payload           []byte `json:"payload"`
	Timestamp         int64  `json:"timestamp"`
	Signature         []byte `json:"sig"`
	Key               []byte `json:"key"`
}

func (p *Subparams) Set(chainpath string, sub *proto.Submission) {
	p.IdentityChainpath = chainpath
	p.Payload = sub.Data
	p.Timestamp = sub.Timestamp
	p.Signature = sub.Signature
	p.Key = sub.Key
}

func (p *Subparams) MarshalJSON() ([]byte, error) {
	var ret string
	ret = fmt.Sprintf("{\"params\": [{\"identity-chainpath\":\"%s\"}",
		p.IdentityChainpath)
	if p.Payload != nil {
		if json.Valid(p.Payload) {
			ret = fmt.Sprintf("%s,{\"payload\":%s}", ret, p.Payload)
		} else {
			ret = fmt.Sprintf("%s,{\"payload\":\"%s\"}", ret, p.Payload)
		}
	}
	if p.Signature == nil || p.Key == nil {
		ret += "]}"
	} else {
		ret = fmt.Sprintf("%s, {\"timestamp\":%d}, {\"sig\":\"%x\"}, {\"key\":\"%x\"}]}",
			ret, p.Timestamp, p.Signature, p.Key)
	}
	if !json.Valid([]byte(ret)) {
		return nil, fmt.Errorf("Invalid json : %s", ret)
	}
	return []byte(ret), nil
}

func CreateKeyPair() ed25519.PrivKey {
	return ed25519.GenPrivKey()
}

func ParseIdentityChainPath(s string) (identity string, chainpath string,err error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", "", err
	}
	identity = strings.ToLower(u.Hostname())
	chainpath = identity
	if len(u.Path) != 0 {
		chainpath += u.Path
	}
	return identity, chainpath, nil
}

func toJSON(m interface{}) (string, error) {
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(js), ",", ", "), nil
}

func URLParser(s string) (ret *proto.Submission, err error) {

	if !utf8.ValidString(s) {
		return ret, fmt.Errorf("URL is has invalid UTF8 encoding")
	}

	if !strings.HasPrefix(s, "acc://") {
		s = "acc://" + s
	}

	var sub *proto.Submission

	u, err := url.Parse(s)
	if err != nil {
		return ret, err
	}

	fmt.Println(u.Scheme)

	fmt.Println(u.Host)
	//so the primary is up to the "." if it is there.
	hostname := strings.ToLower(u.Hostname())
	//DDIIaccounts := strings.Split(hostname,".")

	m, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return ret, err
	}

	chainpath := hostname
	if len(u.Path) != 0 {
		chainpath += u.Path
	}

	insidx := strings.Index(u.RawQuery, "&")
	if len(u.RawQuery) > 0 && insidx < 0 {
		insidx = len(u.RawQuery)
	}

	var data []byte
	var timestamp int64
	var signature []byte
	var key []byte
	if insidx > 0 {
		k := u.RawQuery[:insidx]
		if k == "query" || k == "q" {
			if v := m["payload"]; v == nil {
				m.Del(k)
				js, err := toJSON(m)
				if err != nil {
					return nil, fmt.Errorf("Unable to create url query %s, %v", s, err)
				}
				data = []byte(js)

			}
		}
		//make the correct submission based upon raw query...  Light query needs to be handled differently.
		if v := m["payload"]; v != nil {
			if len(v) > 0 {
				data, err = hex.DecodeString(m["payload"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse payload in url %s, %v", s, err)
				}
			}
		}
		if v := m["timestamp"]; v != nil {
			if len(v) > 0 {
				timestamp, err = strconv.ParseInt(v[0], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("Unable to parse timestamp in url %s, %v", s, err)
				}
			}
		}

		if v := m["sig"]; v != nil {
			if len(v) > 0 {
				signature, err = hex.DecodeString(m["sig"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse signature in url %s, %v", s, err)
				}
			}
		}
		if v := m["key"]; v != nil {
			if len(v) > 0 {
				key, err = hex.DecodeString(m["key"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse signature in url %s, %v", s, err)
				}
			}
		}

		sub = MakeBVCSubmission(k, hostname, chainpath, data, timestamp, signature, key)
	}

	if sub == nil {
		sub = AssembleBVCSubmissionHeader(hostname, chainpath, proto.AccInstruction_Unknown)
	}

	//json rpc params:

	return sub, nil
}


//This will create a submission message that for a token transaction.  Assume only 1 input and many outputs.
func CreateTokenTransaction(inputidentityname *string,
	intputchainname *string, inputamt *big.Int, outputs *map[string]*big.Int, metadata *string,
	signer ed25519.PrivKey) (*proto.Submission, error) {

	type AccTransaction struct {
		Input    map[string]*big.Int  `json:"inputs"`
		Output   *map[string]*big.Int `json:"outputs"`
		Metadata json.RawMessage      `json:"metadata,omitempty"`
	}

	var tx AccTransaction
	tx.Input = make(map[string]*big.Int)
	tx.Input[*intputchainname] = inputamt
	tx.Output = outputs
	if metadata != nil {
		err := tx.Metadata.UnmarshalJSON([]byte(fmt.Sprintf("{%s}", *metadata)))
		if err != nil {
			return nil, fmt.Errorf("Error marshalling metadata %v", err)
		}
	}

	txdata, err := json.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("Error formatting transaction, %v", err)
	}

	sig, err := signer.Sign(txdata)
	if err != nil {
		return nil, fmt.Errorf("Cannot sign data %v", err)
	}
	if signer.PubKey().VerifySignature(txdata, sig) == false {
		return nil, fmt.Errorf("Bad Signature")
	}

	sub := MakeBVCSubmission("tx", *inputidentityname, *intputchainname, txdata, time.Now().Unix(), sig, signer.PubKey().(ed25519.PubKey))

	return sub, nil
}