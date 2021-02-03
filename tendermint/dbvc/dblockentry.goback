package dbvc

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/primitives"
	"github.com/tendermint/tendermint/crypto"

	//"time"

	//"github.com/FactomProject/factomd/common/interfaces"
	"os"
)

//
//Header
//varInt_F	Version	Describes the protocol version that this block is made under. Only valid value is 0. Can safely be coded using 1 byte for the first 127 versions.
//4 bytes	NetworkID	This is a magic number identifying the main Factom network. The value for MainNet Directory Blocks is 0xFA92E5A2. TestNet is 0xFA92E5A3.
//32 bytes	BodyMR	This is the Merkle root of the body data which accompanies this block. It is calculated with SHA256.
//32 bytes	PrevKeyMR	Key Merkle root of previous block. It is the value which is used as a key into databases holding the Directory Block. It is calculated with SHA256.
//32 bytes	PrevFullHash	This is a SHA256 checksum of the previous Directory Block. It is calculated by hashing the serialized block from the beginning of the header through the end of the body. It is included to allow simplified client verification without building a Merkle tree and to double-check the previous block if SHA2 is weakened in the future.
//4 bytes	Timestamp	This the time when the block is opened. Blocks start on 10 minute marks based on UTC (ie 12:00, 12:10, 12:20). The data in this field is POSIX time, counting the number of minutes since epoch in 1970.
//4 bytes	DB Height	The Directory Block height is the sequence it appears in the blockchain. Starts at zero.
//4 bytes	Block Count	This is the number of Entry Blocks that were updated in this block. It is a count of the ChainID:Key pairs. Inclusive of the special blocks. Big endian.
//Body
//N/A 32 bytes	Admin Block ChainID	Indication the next item is the serial hash of the Admin Block.
//N/A 32 bytes	Admin Block LookupHash	This is the LookupHash of the Admin Block generated during this time period.
//N/A 32 bytes	Entry Credit Block ChainID	Indication the next item belongs to the Entry Credit Block.
//N/A 32 bytes	Entry Credit Block HeaderHash	This is the serial hash of the Entry Credit Block Header generated during this time period.
//32 bytes	Factoid Block ChainID	Indication the next item belongs to the Factoid Block.
//32 bytes	Factoid Block KeyMR	This is the KeyMR of the Factoid Block generated during this time period.
//32 bytes	ChainID 0	This is the ChainID of one Entry Block which was updated during this block time. These ChainID:KeyMR pairs are sorted numerically based on the ChainID.
//32 bytes	KeyMR 0	This is the Key Merkle Root of the Entry Block with ChainID 0 which was created during this Directory Block.
//32 bytes	ChainID N	Nth Entry Block ChainID.
//32 bytes	KeyMR N	Nth Entry Block KeyMR.


type DBlockEntry struct {
	MasterChainAddr uint64 //since our dblocks only contain Master Chains, we can use the address instead of full chainid
	KeyMR          types.Hash //sha256[ entries  for the chain. ]
}


func (c *DBlockEntry) GetMasterChainID() (uint64) {
	//defer func() { rval = primitives.CheckNil(rval, "DBEntry.GetChainID") }()

	return c.MasterChainAddr
}

func (c *DBlockEntry) SetMasterChainID(chainID uint64) {
	c.MasterChainAddr = chainID
}

func (c *DBlockEntry) GetKeyMR() (rval types.Hash) {
	//defer func() { rval = primitives.CheckNil(rval, "DBEntry.GetKeyMR") }()

	return c.KeyMR
}

func (c *DBlockEntry) SetKeyMR(keyMR types.Hash) {
	c.KeyMR = keyMR
}

func (e *DBlockEntry) MarshalBinary() (rval []byte, err error) {
	defer func(pe *error) {
		if *pe != nil {
			fmt.Fprintf(os.Stderr, "DBEntry.MarshalBinary err:%v", *pe)
		}
	}(&err)
	//e.Init()
	buf := primitives.NewBuffer(nil)

	err = buf.PushUInt64(e.MasterChainAddr)
	if err != nil {
		return nil, err
	}

	_, err = buf.Write(e.KeyMR.Bytes())
	if err != nil {
		return nil, err
	}

	return buf.DeepCopyBytes(), nil
}

func (e *DBlockEntry) UnmarshalBinaryData(data []byte) ([]byte, error) {
	newData := data
	var err error

	buf := primitives.NewBuffer(data)

	e.MasterChainAddr, err = buf.PopUInt64()

	if err != nil {
		return nil, err
	}
	_, err = buf.Read(e.KeyMR.Bytes())
	if err != nil {
		return nil, err
	}

	return newData, nil
}

func (e *DBlockEntry) UnmarshalBinary(data []byte) (err error) {
	_, err = e.UnmarshalBinaryData(data)
	return err
}

func (e *DBlockEntry) ShaHash() (rval *types.Hash) {
	//defer func() { rval = primitives.CheckNil(rval, "DBEntry.ShaHash") }()

	byteArray, _ := e.MarshalBinary()
	copy(rval.Bytes(),crypto.Sha256(byteArray))
	return rval
}

func (e *DBlockEntry) JSONByte() ([]byte, error) {
	return primitives.EncodeJSON(e)
}

func (e *DBlockEntry) JSONString() (string, error) {
	return primitives.EncodeJSONString(e)
}

func (e *DBlockEntry) String() string {
	var out primitives.Buffer

	out.WriteString(fmt.Sprintf("chainid:       %x\n", e.GetMasterChainID()))
	out.WriteString(fmt.Sprintf("     keymr:    %s\n", bytes.NewBuffer(e.GetKeyMR().Bytes()[:6]).String()))
	return (string)(out.DeepCopyBytes())
}

