package dbvc

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/primitives"
	"github.com/tendermint/tendermint/crypto"
	"time"

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
	BodyMR types.Hash	    //This is the Merkle root of the body data which accompanies this block. It is calculated with SHA256.
	PrevKeyMR types.Hash	//Key Merkle root of previous block. It is the value which is used as a key into databases holding the Directory Block. It is calculated with SHA256.
	PrevFullHash types.Hash	//This is a SHA256 checksum of the previous Directory Block. It is calculated by hashing the serialized block from the beginning of the header through the end of the body. It is included to allow simplified client verification without building a Merkle tree and to double-check the previous block if SHA2 is weakened in the future.
	Timestamp int64     //This the time when the block is opened. Blocks start on 10 minute marks based on UTC (ie 12:00, 12:10, 12:20). The data in this field is POSIX time, counting the number of minutes since epoch in 1970.
	DBHeight uint32      //The Directory Block height is the sequence it appears in the blockchain. Starts at zero.
	BlockCount uint32	 // This is the number of Entry Blocks that were updated in this block. It is a count of the ChainID:Key pairs. Inclusive of the special blocks. Big endian.
}

func NewDBlockHeader() *DBlockHeader {
	d := new(DBlockHeader)

	d.Version = 1
	d.NetworkID = 0xACC00001
	//d.BodyMR = primitives.NewZeroHash()
	//d.PrevKeyMR = primitives.NewZeroHash()
	//d.PrevFullHash = primitives.NewZeroHash()

	return d
}


func (h *DBlockHeader) Init() {
	//if h.BodyMR == nil {
	//	h.BodyMR = primitives.NewZeroHash()
	//}
	//if h.PrevKeyMR == nil {
	//	h.PrevKeyMR = primitives.NewZeroHash()
	//}
	//if h.PrevFullHash == nil {
	//	h.PrevFullHash = primitives.NewZeroHash()
	//}
}

func (b *DBlockHeader) GetHeaderHash() (h *types.Hash,err error) {
	binaryEBHeader, err := b.MarshalBinary()
	if err != nil {
		return nil, err
	}

	copy(h.Bytes(), crypto.Sha256(crypto.Sha256(binaryEBHeader)))

	return h, nil
}

func (a *DBlockHeader) IsSameAs(b *DBlockHeader) bool {
	if a == nil || b == nil {
		if a == nil && b == nil {
			return true
		}
		return false
	}

	if a.Version != b.Version {
		return false
	}
	if a.NetworkID != b.NetworkID {
		return false
	}

	if bytes.Equal(a.BodyMR[:], b.BodyMR[:]) == false {
		return false
	}

	if bytes.Equal(a.PrevKeyMR[:], b.PrevKeyMR[:]) == false {
		return false
	}
	if bytes.Equal(a.PrevFullHash[:], b.PrevFullHash[:]) == false {
		return false
	}

	if a.Timestamp != b.Timestamp {
		return false
	}
	if a.DBHeight != b.DBHeight {
		return false
	}
	if a.BlockCount != b.BlockCount {
		return false
	}

	return true
}

func (b *DBlockHeader) GetVersion() uint64 {
	return b.Version
}

func (h *DBlockHeader) SetVersion(version uint64) {
	h.Version = version
}

func (h *DBlockHeader) GetNetworkID() uint32 {
	return h.NetworkID
}

func (h *DBlockHeader) SetNetworkID(networkID uint32) {
	h.NetworkID = networkID
}

func (h *DBlockHeader) GetBodyMR() (rval *types.Hash) {

	return &h.BodyMR
}

func (h *DBlockHeader) SetBodyMR(bodyMR *types.Hash) {
	h.BodyMR = *bodyMR
}

func (h *DBlockHeader) GetPrevKeyMR() (rval *types.Hash) {

	return &h.PrevKeyMR
}

func (h *DBlockHeader) SetPrevKeyMR(prevKeyMR *types.Hash) {
	h.PrevKeyMR = *prevKeyMR
}

func (h *DBlockHeader) GetPrevFullHash() (rval *types.Hash) {
	return &h.PrevFullHash
}

func (h *DBlockHeader) SetPrevFullHash(PrevFullHash *types.Hash) {
	h.PrevFullHash = *PrevFullHash
}

func (h *DBlockHeader) GetTimestamp() time.Time {

	return time.Unix(h.Timestamp,0)
}

func (h *DBlockHeader) SetTimestamp(timestamp time.Time) {

	h.Timestamp = timestamp.Unix()
}

func (h *DBlockHeader) GetDBHeight() uint32 {
	return h.DBHeight
}

func (h *DBlockHeader) SetDBHeight(dbheight uint32) {
	h.DBHeight = dbheight
}

func (h *DBlockHeader) GetBlockCount() uint32 {
	return h.BlockCount
}

func (h *DBlockHeader) SetBlockCount(blockcount uint32) {
	h.BlockCount = blockcount
}

func (e *DBlockHeader) JSONByte() ([]byte, error) {
	return primitives.EncodeJSON(e)
}

func (e *DBlockHeader) JSONString() (string, error) {
	return primitives.EncodeJSONString(e)
}

func (e *DBlockHeader) String() string {
	e.Init()

	var out primitives.Buffer
	out.WriteString(fmt.Sprintf("  version:         %v\n", e.Version))
	out.WriteString(fmt.Sprintf("  networkid:       %x\n", e.NetworkID))
	out.WriteString(fmt.Sprintf("  bodymr:          %s\n", bytes.NewBuffer(e.BodyMR.Bytes()[:6]).String()))
	out.WriteString(fmt.Sprintf("  prevkeymr:       %s\n", bytes.NewBuffer(e.PrevKeyMR.Bytes()[:6]).String()))
	out.WriteString(fmt.Sprintf("  prevfullhash:    %s\n", bytes.NewBuffer(e.PrevFullHash.Bytes()[:6]).String()))
	out.WriteString(fmt.Sprintf("  timestamp:       %d\n", e.Timestamp))
	out.WriteString(fmt.Sprintf("  timestamp str:   %s\n", e.GetTimestamp().String()))
	out.WriteString(fmt.Sprintf("  dbheight:        %d\n", e.DBHeight))
	out.WriteString(fmt.Sprintf("  blockcount:      %d\n", e.BlockCount))

	return (string)(out.DeepCopyBytes())
}

func (b *DBlockHeader) MarshalBinary() (rval []byte, err error) {
	defer func(pe *error) {
		if *pe != nil {
			fmt.Fprintf(os.Stderr, "DBlockHeader.MarshalBinary err:%v", *pe)
		}
	}(&err)
	//b.Init()
	buf := primitives.NewBuffer(nil)

	err = buf.PushVarInt(uint64(b.Version))
	if err != nil {
		return nil, err
	}
	err = buf.PushUInt32(b.NetworkID)
	if err != nil {
		return nil, err
	}
	_, err = buf.Write(b.BodyMR.Bytes())
	if err != nil {
		return nil, err
	}
	_, err = buf.Write(b.PrevKeyMR.Bytes())
	if err != nil {
		return nil, err
	}
	_, err = buf.Write(b.PrevFullHash.Bytes())
	if err != nil {
		return nil, err
	}
	err = buf.PushInt64(b.Timestamp)
	if err != nil {
		return nil, err
	}
	err = buf.PushUInt32(b.DBHeight)
	if err != nil {
		return nil, err
	}
	err = buf.PushUInt32(b.BlockCount)
	if err != nil {
		return nil, err
	}

	if b.BlockCount > 100000 {
		panic("Send: Blockcount too great in directory block")
	}

	return buf.DeepCopyBytes(), err
}

func (b *DBlockHeader) UnmarshalBinaryData(data []byte) ([]byte, error) {
	buf := primitives.NewBuffer(data)
	var err error

	b.Version, err = buf.PopVarInt()
	if err != nil {
		return nil, err
	}
	b.NetworkID, err = buf.PopUInt32()
	if err != nil {
		return nil, err
	}

	_, err = buf.Read(b.BodyMR.Bytes())
	if err != nil {
		return nil, err
	}

	_, err = buf.Read(b.PrevKeyMR.Bytes())
	if err != nil {
		return nil, err
	}

	_, err = buf.Read(b.PrevFullHash.Bytes())
	if err != nil {
		return nil, err
	}

	b.Timestamp, err = buf.PopInt64()
	if err != nil {
		return nil, err
	}
	b.DBHeight, err = buf.PopUInt32()
	if err != nil {
		return nil, err
	}
	b.BlockCount, err = buf.PopUInt32()
	if err != nil {
		return nil, err
	}

	if b.BlockCount > 100000 {
		panic("Receive: Blockcount too great in directory block" + fmt.Sprintf(":::: %d", b.BlockCount))
	}

	return buf.DeepCopyBytes(), nil
}

func (b *DBlockHeader) UnmarshalBinary(data []byte) (err error) {
	_, err = b.UnmarshalBinaryData(data)
	return err
}

