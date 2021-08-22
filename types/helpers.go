package types

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// AssembleBVCSubmissionHeader This will populate the identity chain, chainId, and instruction fields for a BVC submission
func AssembleBVCSubmissionHeader(identityname string, chainpath string, ins proto.AccInstruction) *proto.Submission {
	sub := proto.Submission{}

	sub.Identitychain = GetIdentityChainFromIdentity(identityname).Bytes()
	if chainpath == "" {
		chainpath = identityname
	}
	sub.Chainid = GetChainIdFromChainPath(chainpath).Bytes()
	sub.Instruction = ins
	return &sub
}

// MakeBVCSubmission This will make the BVC submision protobuf transaction
func MakeBVCSubmission(ins string, adi UrlAdi, chainUrl UrlChain, payload []byte, timestamp int64, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
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

// MarshalBinaryLedgerAdiChainPath fullchainpath == identityname/chainpath
//This function will generate a ledger needed for ed25519 signing or sha256 hashed to produce TXID
func MarshalBinaryLedgerAdiChainPath(fullchainpath string, payload []byte, timestamp int64) []byte {
	var msg []byte

	//the timestamp will act
	var tsbytes [8]byte
	binary.LittleEndian.PutUint64(tsbytes[:], uint64(timestamp))
	msg = append(msg, tsbytes[:]...)

	//The chain path is either the identity name or the full chain path [identityname]/[chainpath]
	chainid := sha256.Sum256([]byte(fullchainpath))
	msg = append(msg, chainid[:]...)

	msg = append(msg, payload...)

	return msg
}

// MarshalBinaryLedgerChainId create a ledger that can be used for signing or generating a txid
func MarshalBinaryLedgerChainId(chainid []byte, payload []byte, timestamp int64) []byte {
	var msg []byte

	var tsbytes [8]byte
	binary.LittleEndian.PutUint64(tsbytes[:], uint64(timestamp))
	msg = append(msg, tsbytes[:]...)

	//The chain path is either the identity name or the full chain path [identityname]/[chainpath]
	msg = append(msg, chainid[:]...)

	msg = append(msg, payload...)

	return msg
}

var InstructionTypeMap = map[string]proto.AccInstruction{
	"adi-create":           proto.AccInstruction_Identity_Creation,
	"token-account-create": proto.AccInstruction_Token_URL_Creation,
	"token-tx-create":      proto.AccInstruction_Token_Transaction,
	"data-chain-create":    proto.AccInstruction_Data_Chain_Creation,
	"data-entry-create":    proto.AccInstruction_Data_Entry,
	"scratch-chain-create": proto.AccInstruction_Scratch_Chain_Creation,
	"scratch-entry-create": proto.AccInstruction_Scratch_Entry,
	"token-create":         proto.AccInstruction_Token_Issue,
	"key-update":           proto.AccInstruction_Key_Update,
	"query-tx-create":      proto.AccInstruction_Deep_Query,
	"query":                proto.AccInstruction_Light_Query,
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

func (p *Subtx) Set(chainpath string, sub *proto.Submission) {
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

// CreateKeyPair generic helper function to create an ed25519 key pair
func CreateKeyPair() ed25519.PrivKey {
	return ed25519.GenPrivKey()
}

// ParseIdentityChainPath helpful parser to extract the identity name and chainpath
//for example RedWagon/MyAccAddress becomes identity=redwagon and chainpath=redwagon/MyAccAddress
func ParseIdentityChainPath(s string) (identity string, chainpath string, err error) {

	if !utf8.ValidString(s) {
		return "", "", fmt.Errorf("URL is has invalid UTF8 encoding")
	}

	if !strings.HasPrefix(s, "acc://") {
		s = "acc://" + s
	}

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

// toJSON marshal object to json
func toJSON(m interface{}) (string, error) {
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(js), ",", ", "), nil
}

// URLParser helper function to take an acme url and generate a submission transaction.
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
					return nil, fmt.Errorf("unable to create url query %s, %v", s, err)
				}
				data = []byte(js)

			}
		}
		//make the correct submission based upon raw query...  Light query needs to be handled differently.
		if v := m["payload"]; v != nil {
			if len(v) > 0 {
				data, err = hex.DecodeString(m["payload"][0])
				if err != nil {
					return nil, fmt.Errorf("unable to parse payload in url %s, %v", s, err)
				}
			}
		}
		if v := m["timestamp"]; v != nil {
			if len(v) > 0 {
				timestamp, err = strconv.ParseInt(v[0], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("unable to parse timestamp in url %s, %v", s, err)
				}
			}
		}

		if v := m["sig"]; v != nil {
			if len(v) > 0 {
				signature, err = hex.DecodeString(m["sig"][0])
				if err != nil {
					return nil, fmt.Errorf("unable to parse signature in url %s, %v", s, err)
				}
			}
		}
		if v := m["key"]; v != nil {
			if len(v) > 0 {
				key, err = hex.DecodeString(m["key"][0])
				if err != nil {
					return nil, fmt.Errorf("unable to parse signature in url %s, %v", s, err)
				}
			}
		}

		sub = MakeBVCSubmission(k, UrlAdi(hostname), UrlChain(chainpath), data, timestamp, signature, key)
	}

	if sub == nil {
		sub = AssembleBVCSubmissionHeader(hostname, chainpath, proto.AccInstruction_Unknown)
	}

	//json rpc params:

	return sub, nil
}

// GetChainIdFromChainPath this expects an identity chain path to produce the chainid.  RedWagon/Acc/Chain/Path
func GetChainIdFromChainPath(identitychainpath string) *Bytes32 {
	_, chainpathformatted, err := ParseIdentityChainPath(identitychainpath)
	if err != nil {
		return nil
	}

	h := Bytes32(sha256.Sum256([]byte(chainpathformatted)))
	return &h
}

// GetIdentityChainFromIdentity Helper function to generate an identity chain from adi. can return nil, if the adi is malformed
func GetIdentityChainFromIdentity(adi string) *Bytes32 {
	namelower, _, err := ParseIdentityChainPath(adi)
	if err != nil {
		return nil
	}

	h := Bytes32(sha256.Sum256([]byte(namelower)))
	return &h
}

//GetAddressFromIdentityChain get the 8-bit address from the identity chain.  this is used for bvc routing
func GetAddressFromIdentityChain(identitychain []byte) uint64 {
	addr := binary.LittleEndian.Uint64(identitychain)
	return addr
}

//GetAddressFromIdentity given a string, return the address used for bvc routing
func GetAddressFromIdentity(name string) uint64 {
	b := GetIdentityChainFromIdentity(name)
	return GetAddressFromIdentityChain(b[:])
}

//This function will build a chain from an DDII / ADI.  If the string is 64 characters in length, then it is assumed
//to be a hex encoded ChainID instead.
//func BuildChainIdFromAdi(chainadi *string) ([]byte, error) {
//
//	chainidlen := len(*chainadi)
//	var chainid managed.Hash
//
//	if chainidlen < 32 {
//		chainid = sha256.Sum256([]byte(*chainadi))
//	} else if chainidlen == 64 {
//		_, err := hex.Decode(chainid[:], []byte(*chainadi))
//		if err != nil {
//			fmt.Errorf("[Error] cannot decode chainid %s", *chainadi)
//			return nil, err
//		}
//	} else {
//		return nil, fmt.Errorf("[Error] invalid chainid for validator on shard %s", *chainadi)
//	}
//
//	return chainid.Bytes(), nil
//}

type Bytes []byte

// MarshalJSON serializes ByteArray to hex
func (s *Bytes) MarshalJSON() ([]byte, error) {
	bytes, err := json.Marshal(fmt.Sprintf("%x", string(*s)))
	return bytes, err
}

// UnmarshalJSON serializes ByteArray to hex
func (s *Bytes) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	d, err := hex.DecodeString(str)
	if err != nil {
		return nil
	}
	*s = make([]byte, len(d))
	copy(*s, d)
	return err
}

func (s *Bytes) Bytes() []byte {
	return []byte(*s)
}

func (s *Bytes) MarshalBinary() ([]byte, error) {
	var buf [8]byte
	l := s.Size(&buf)
	i := l - len(*s)
	data := make([]byte, l)
	copy(data, buf[:i])
	copy(data[i:], *s)
	return data, nil
}

func (s *Bytes) UnmarshalBinary(data []byte) error {
	slen, l := binary.Varint(data)
	if l == 0 {
		return fmt.Errorf("cannot unmarshal string")
	}
	if len(data) < int(slen)+l {
		return fmt.Errorf("insufficient data to unmarshal string")
	}
	*s = data[l : int(slen)+l]
	return nil
}

func (s *Bytes) Size(varintbuf *[8]byte) int {
	buf := varintbuf
	if buf == nil {
		buf = &[8]byte{}
	}
	l := int64(len(*s))
	i := binary.PutVarint(buf[:], l)
	return i + int(l)
}

// Bytes32 is a fixed array of 32 bytes
type Bytes32 [32]byte

// MarshalJSON serializes ByteArray to hex
func (s *Bytes32) MarshalJSON() ([]byte, error) {
	bytes, err := json.Marshal(s.ToString())
	return bytes, err
}

// UnmarshalJSON serializes ByteArray to hex
func (s *Bytes32) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	return s.FromString(str)
}

// Bytes returns the bite slice of the 32 byte array
func (s Bytes32) Bytes() []byte {
	return s[:]
}

// FromString takes a hex encoded string and sets the byte array.
// The input parameter, str, must be 64 hex characters in length
func (s *Bytes32) FromString(str string) error {
	if len(str) != 64 {
		return fmt.Errorf("insufficient data")
	}

	d, err := hex.DecodeString(str)
	if err != nil {
		return err
	}
	copy(s[:], d)
	return nil
}

// ToString will convert the 32 byte array into a hex string that is 64 hex characters in length
func (s *Bytes32) ToString() String {
	return String(hex.EncodeToString(s[:]))
}

// Bytes32 is a fixed array of 32 bytes
type Bytes64 [64]byte

// MarshalJSON serializes ByteArray to hex
func (s *Bytes64) MarshalJSON() ([]byte, error) {
	bytes, err := json.Marshal(s.ToString())
	return bytes, err
}

// UnmarshalJSON serializes ByteArray to hex
func (s *Bytes64) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	return s.FromString(str)
}

// Bytes returns the bite slice of the 32 byte array
func (s Bytes64) Bytes() []byte {
	return s[:]
}

// FromString takes a hex encoded string and sets the byte array.
// The input parameter, str, must be 64 hex characters in length
func (s *Bytes64) FromString(str string) error {
	if len(str) != 128 {
		return fmt.Errorf("insufficient data")
	}

	d, err := hex.DecodeString(str)
	if err != nil {
		return err
	}
	copy(s[:], d)
	return nil
}

// ToString will convert the 32 byte array into a hex string that is 64 hex characters in length
func (s *Bytes64) ToString() String {
	return String(hex.EncodeToString(s[:]))
}

type String string

func (s *String) MarshalBinary() ([]byte, error) {
	b := Bytes(*s)
	return b.MarshalBinary()
}

func (s *String) UnmarshalBinary(data []byte) error {
	var b Bytes
	err := b.UnmarshalBinary(data)
	if err == nil {
		*s = String(b)
	}
	return err
}

func (s *String) Size(varintbuf *[8]byte) int {
	b := Bytes(*s)
	return b.Size(varintbuf)
}

type Byte byte

func (s *Byte) MarshalBinary() ([]byte, error) {
	ret := make([]byte, 1)
	ret[0] = byte(*s)
	return ret, nil
}

func (s *Byte) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("insufficient data length for unmarshal of a byte")
	}
	*s = Byte(data[0])
	return nil
}

type UrlAdi string

type UrlChain string

type Amount struct {
	big.Int
}

func (a *Amount) AsBigInt() *big.Int {
	return &a.Int
}
