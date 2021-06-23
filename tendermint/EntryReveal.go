package tendermint

import (
	"crypto/sha256"
	"fmt"
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"

	"crypto/ed25519"
)

type EntryCommit struct {
	Version	uint64
	ChainId [32]byte //either a ddii or hash
	ExtIdSize int16 //stored big endian
	Numec int8
	Signature valacctypes.Signature //signature contains signature and pubkey
	//Header
	//varInt_F	Version	starts at 0. Higher numbers are currently rejected. Can safely be coded using 1 byte for the first 127 versions.
	//32 bytes	ChainID	This is the Chain which the author wants this Entry to go into.
	//2 bytes	ExtIDs Size	Describes how many bytes required for the set of External IDs for this Entry. Must be less than or equal to the Payload size. Big endian.
	//Payload		This is the data between the end of the Header and the end of the Content.
	//External IDs		This section is only interpreted and enforced if the External ID Size is greater than zero.
	//2 bytes	ExtID element 0 length	This is the number of the following bytes to be interpreted as an External ID element. Cannot be 0 length.
	//variable	ExtID 0	This is the data for the first External ID.
	//2 bytes	ExtID X	Size of the X External ID
	//variable	ExtID X data	This is the Xth element. The last byte of the last element must fall on the last byte specified ExtIDs Size in the header.
	//Content
	//variable	Entry Data	This is the unstructured part of the Entry. It is all user specified data.
}
const EntryCommitLedgerSize int = 1+6+32+1
const EntryCommitSize int = EntryCommitLedgerSize+32+64

func (ec *EntryCommit) MarshalLedger() []byte {
	ret := make([]byte,EntryCommitLedgerSize)

	startoffset := 0
	endoffset := 1
	l := copy(ret[startoffset:1], valacctypes.EncodeVarIntGoBytes(ec.Version))
	startoffset = endoffset
	endoffset += l

	l = copy(ret[startoffset:endoffset], ec.Timestamp.Bytes()[2:8])
	startoffset = endoffset
	endoffset += l

	l = copy(ret[startoffset:endoffset],ec.Entryhash.Bytes())
	startoffset = endoffset
	endoffset += l

	ret[startoffset] =byte(ec.Numec)
	startoffset = endoffset

	return ret
}

func (ec *EntryCommit) Marshal() []byte {
	ret := make([]byte,EntryCommitSize)

	ll := copy(ret, ec.MarshalLedger())
	copy(ret[ll:],ec.Signature.Bytes())

	return ret[:]
}

func (ec *EntryCommit) UnmarshalLedger(data []byte) ([]byte,error) {

	var rem []byte

	if len(data) < EntryCommitLedgerSize {
		return nil,fmt.Errorf("Invalid entry commit size for unmarshalling")
	}

	ec.Version, rem = valacctypes.DecodeVarInt(data)
	if len(rem) < EntryCommitLedgerSize-1 {
		return nil,fmt.Errorf("Invalid entry commit size after version")
	}

	var ts [8]byte

	copy(ts[2:8],rem[0:6])
	ec.Timestamp.Extract(ts[:])

	rem = ec.Entryhash.Extract(rem[6:6+32])

	ec.Numec = int8(rem[0])

	if len(rem) == 1 {
		return nil, nil
	}

	return rem[1:],nil
}

func (ec *EntryCommit) Unmarshal(data []byte) error {

	rem, err := ec.UnmarshalLedger(data)
	if err == nil {
		ec.Signature.Extract(rem)
	}

	return err
}

func (ec *EntryCommit) Valid() bool {
	return ed25519.Verify(ec.Signature.PublicKey, ec.MarshalLedger(),ec.Signature.Signature[:])
}

func (ec *EntryCommit) TXID() valacctypes.Hash {
	return sha256.Sum256(ec.MarshalLedger())
}

func (ec *EntryCommit) EntryHash() valacctypes.Hash {
	return sha256.Sum256(ec.Marshal())
}
