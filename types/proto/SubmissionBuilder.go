package proto

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

// SubmissionBuilder helps build an accumulated submission to the BVC.
type SubmissionBuilder struct {
	sub Submission
}

func (sb *SubmissionBuilder) Instruction(ins AccInstruction) *SubmissionBuilder {
	sb.sub.Instruction = ins
	return sb
}

func (sb *SubmissionBuilder) Data(data []byte) *SubmissionBuilder {
	sb.sub.Data = data
	return sb
}

func (sb *SubmissionBuilder) Timestamp(timestamp int64) *SubmissionBuilder {
	sb.sub.Timestamp = timestamp
	return sb
}

func (sb *SubmissionBuilder) Signature(sig types.Bytes) *SubmissionBuilder {
	sb.sub.Signature = sig
	return sb
}

func (sb *SubmissionBuilder) PubKey(pubKey types.Bytes) *SubmissionBuilder {
	sb.sub.Key = pubKey[:]
	return sb
}

func (sb *SubmissionBuilder) Type(chainType types.Bytes) *SubmissionBuilder {
	sb.sub.Type = chainType
	return sb
}

func (sb *SubmissionBuilder) ChainUrl(url string) *SubmissionBuilder {
	adi, chain, _ := types.ParseIdentityChainPath(url)
	sb.sub.Chainid = types.GetChainIdFromChainPath(chain).Bytes()
	sb.sub.AdiChainPath = chain
	if len(sb.sub.Identitychain) == 0 {
		sb.sub.Identitychain = types.GetIdentityChainFromIdentity(adi).Bytes()
	}
	return sb
}

func (sb *SubmissionBuilder) AdiUrl(url string) *SubmissionBuilder {
	adi, _, _ := types.ParseIdentityChainPath(url)
	sb.sub.Identitychain = types.GetIdentityChainFromIdentity(adi).Bytes()
	if len(sb.sub.AdiChainPath) == 0 {
		sb.sub.AdiChainPath = adi
	}

	return sb
}

func (sb *SubmissionBuilder) BuildUnsigned() (*Submission, error) {

	if sb.sub.Instruction == 0 {
		return nil, fmt.Errorf("instruction not set")
	}

	if len(sb.sub.Data) == 0 {
		return nil, fmt.Errorf("no payload data set")
	}

	if len(sb.sub.AdiChainPath) == 0 {
		return nil, fmt.Errorf("invalid adi url")
	}

	if sb.sub.Timestamp == 0 {
		return nil, fmt.Errorf("timestamp not set")
	}

	return &sb.sub, nil
}

func (sb *SubmissionBuilder) Build() (*Submission, error) {

	if len(sb.sub.Signature) != 64 {
		return nil, fmt.Errorf("invalid signature length")
	}

	if len(sb.sub.Key) != 32 {
		return nil, fmt.Errorf("invalid public key data length")
	}

	var err error
	_, err = sb.BuildUnsigned()
	if err != nil {
		return nil, err
	}

	return &sb.sub, nil
}

func Builder() *SubmissionBuilder {
	return &SubmissionBuilder{}
}

// AssembleBVCSubmissionHeader This will populate the identity chain, chainId, and instruction fields for a BVC submission
func AssembleBVCSubmissionHeader(identityname string, chainpath string, ins AccInstruction) *Submission {
	sub := Submission{}

	sub.Identitychain = types.GetIdentityChainFromIdentity(identityname).Bytes()
	if chainpath == "" {
		chainpath = identityname
	}
	sub.Chainid = types.GetChainIdFromChainPath(chainpath).Bytes()
	sub.Instruction = ins
	return &sub
}

// MakeBVCSubmission This will make the BVC submision protobuf transaction
func MakeBVCSubmission(ins string, adi types.UrlAdi, chainUrl types.UrlChain, payload []byte, timestamp int64, signature []byte, pubkey ed25519.PubKey) *Submission {
	v := InstructionTypeMap[ins]
	if v == 0 {
		return nil
	}
	sub := AssembleBVCSubmissionHeader(string(adi), string(chainUrl), v)
	sub.Data = payload
	sub.Timestamp = timestamp
	sub.Signature = signature
	sub.Key = pubkey
	return sub
}

var InstructionTypeMap = map[string]AccInstruction{
	"adi":          AccInstruction_Identity_Creation,
	"tokenAccount": AccInstruction_Token_URL_Creation,
	"tokenTx":      AccInstruction_Token_Transaction,
	"dataChain":    AccInstruction_Data_Chain_Creation,
	"dataEntry":    AccInstruction_Data_Entry,
	"scratchChain": AccInstruction_Scratch_Chain_Creation,
	"scratchEntry": AccInstruction_Scratch_Entry,
	"token":        AccInstruction_Token_Issue,
	"keyUpdate":    AccInstruction_Key_Update,
	"queryTx":      AccInstruction_Deep_Query,
	"query":        AccInstruction_State_Query,
}

// Subtx this is a generic structure for a bvc submission transaction that can be marshalled to and from json,
//this is helpful for json rpc
type Subtx struct {
	IdentityChainpath string `json:"chainUrl"`
	Payload           []byte `json:"payload"`
	Timestamp         int64  `json:"timestamp"`
	Signature         []byte `json:"sig"`
	Key               []byte `json:"key"`
}

func (p *Subtx) Set(chainpath string, sub *Submission) {
	p.IdentityChainpath = chainpath
	p.Payload = sub.Data
	p.Timestamp = sub.Timestamp
	p.Signature = sub.Signature
	p.Key = sub.Key
}

func (p *Subtx) MarshalJSON() ([]byte, error) {
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
		return nil, fmt.Errorf("invalid json : %s", ret)
	}
	return []byte(ret), nil
}
