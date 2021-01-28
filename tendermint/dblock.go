package tendermint

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"unsafe"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
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
//32 bytes	Admin Block ChainID	Indication the next item is the serial hash of the Admin Block.
//32 bytes	Admin Block LookupHash	This is the LookupHash of the Admin Block generated during this time period.
//32 bytes	Entry Credit Block ChainID	Indication the next item belongs to the Entry Credit Block.
//32 bytes	Entry Credit Block HeaderHash	This is the serial hash of the Entry Credit Block Header generated during this time period.
//32 bytes	Factoid Block ChainID	Indication the next item belongs to the Factoid Block.
//32 bytes	Factoid Block KeyMR	This is the KeyMR of the Factoid Block generated during this time period.
//32 bytes	ChainID 0	This is the ChainID of one Entry Block which was updated during this block time. These ChainID:KeyMR pairs are sorted numerically based on the ChainID.
//32 bytes	KeyMR 0	This is the Key Merkle Root of the Entry Block with ChainID 0 which was created during this Directory Block.
//32 bytes	ChainID N	Nth Entry Block ChainID.
//32 bytes	KeyMR N	Nth Entry Block KeyMR.


type DBlockHeader struct {
	Version uint64	    //varint Describes the protocol version that this block is made under. Only valid value is 0. Can safely be coded using 1 byte for the first 127 versions.
	NetworkID uint32	//This is a magic number identifying the main Factom network. The value for MainNet Directory Blocks is 0xFA92E5A2. TestNet is 0xFA92E5A3.
	BodyMR [32]byte	    //This is the Merkle root of the body data which accompanies this block. It is calculated with SHA256.
	PrevKeyMR [32]byte	//Key Merkle root of previous block. It is the value which is used as a key into databases holding the Directory Block. It is calculated with SHA256.
	PrevFullHash [32]byte	//This is a SHA256 checksum of the previous Directory Block. It is calculated by hashing the serialized block from the beginning of the header through the end of the body. It is included to allow simplified client verification without building a Merkle tree and to double-check the previous block if SHA2 is weakened in the future.
	Timestamp uint32     //This the time when the block is opened. Blocks start on 10 minute marks based on UTC (ie 12:00, 12:10, 12:20). The data in this field is POSIX time, counting the number of minutes since epoch in 1970.
	DBHeight uint32      //The Directory Block height is the sequence it appears in the blockchain. Starts at zero.
	BlockCount uint32	 // This is the number of Entry Blocks that were updated in this block. It is a count of the ChainID:Key pairs. Inclusive of the special blocks. Big endian.
}



func NewDBockHeader() *DBockHeader {
	return &DBockHeader{Version: 1, NetworkID: 0xACC00001}
}

func (dbheader *DBlockHeader) Marshal() []byte {
    var rbuf bytes.Buffer
    types.EncodeVarInt(&rbuf,dbheader.Version)


    binary.Write(&rbuf,binary.BigEndian,&dbheader.NetworkID)
    rbuf.Write(types.Uint32Bytes(dbheader.NetworkID))
    rbuf.Write(dbheader.BodyMR[:])
    rbuf.Write(dbheader.PrevKeyMR[:])
    rbuf.Write(dbheader.PrevFullHash[:])

    binary.Write(&rbuf,binary.BigEndian)
	rbuf.Write(types.Uint32Bytes(dbheader.Timestamp))
	rbuf.Write(types.Uint32Bytes(dbheader.DBHeight))

    //Write to buffer BigEndian...
	rbuf.WriteByte(byte(0xFF & (dbheader.BlockCount >> 0x24)))
	rbuf.WriteByte(byte(0xFF & (dbheader.BlockCount >> 0x16)))
	rbuf.WriteByte(byte(0xFF & (dbheader.BlockCount >> 0x8)))
	rbuf.WriteByte(byte(0xFF & dbheader.BlockCount))

    return rbuf.Bytes()
}

func (dbheader *DBlockHeader) Unmarshal(ibuf []byte) {
    var rbuf bytes.Buffer

    rbuf.Write(ibuf)

	if len(ibuf)
}

type DBlockEntry struct {
	MasterChainAddr uint64 //since our dblocks only contain Master Chains, we can use the address instead of full chainid
	KeyMR          [32]byte //sha256[ entries  for the chain. ]
}

type DBlockBody struct {
	//32 bytes	Admin Block ChainID	Indication the next item is the serial hash of the Admin Block.
	//32 bytes	Admin Block LookupHash	This is the LookupHash of the Admin Block generated during this time period.
	//for i = 1 to BlockCount

	//32 bytes	ChainID[i]	This is the ChainID of one Entry Block which was updated during this block time. These ChainID:KeyMR pairs are sorted numerically based on the ChainID.
	//32 bytes	KeyMR[i]]	This is the Key Merkle Root of the Entry Block with ChainID 0 which was created during this Directory Block.
}

type DBlock struct {

	header DBlockHeader
	blockCount int32 //number of entry to follow
	entry []DBlockEntry

}
// Entry Header
//                   BVCMR_012345 (KeyMR that goes in DBlockBody)
//                 /                   \
//              BVCMR_0123            BVCMR45
//          /                \          \
//       BVCMR_01        BVCMR_23       BVCMR45
//      /      \         /      \         /      \
//  BVCMR_0  BVCMR_1  BVCMR_2 BVCMR_3   BVCMR4  BVCMR5
//32 bytes
//ChainID
//All the Entries in this Entry Block have this ChainID
//32 bytes
//BodyMR
//This is the Merkle root of the body data which accompanies this block.  It is calculated with SHA256.
//32 bytes
//PrevKeyMR
//Key Merkle root of previous block.  This is the value of this ChainID's previous Entry Block Merkle root which was placed in the Directory Block.  It is the value which is used as a key into databases holding the Entry Block. It is calculated with SHA256.
//32 bytes
//PrevFullHash
//This is a SHA256 checksum of the previous Entry Block of this ChainID. It is calculated by hashing the serialized block from the beginning of the header through the end of the body. It is included to doublecheck the previous block if SHA2 is weakened in the future.  First block has a PrevFullHash of 0.
//4 bytes
//EB Sequence
//This is the sequence which this block is in for this ChainID.  This number increments by 1 for every new EB with this chain ID.  First block is height 0. Big endian.
//4 bytes
//DB Height
//This the Directory Block height which this Entry Block is located in. Big endian.
//4 bytes
//Entry Count
//This is the number of Entry Hashes and time delimiters that the body of this block contains.  Big endian.

//