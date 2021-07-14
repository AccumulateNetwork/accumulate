package tendermint

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
)

////bvc entry header:
//const BVCEntryMaxSize = 1+32+4+8+32


const(
	DDII_type int = 0
	BVCHeight_type int = 1
	Timestamp_type int = 2
	MDRoot_type int = 3
)

type BVCEntry struct {
	Version byte
	DDII []byte
	BVCHeight int64          /// (4 bytes) Height of master chain block
	Timestamp uint64
	MDRoot managed.Hash

}

func (entry *BVCEntry) MarshalBinary()([]byte, error) {
	ret := make([]byte,1+1+len(entry.DDII)+4+8+32)

	offset := 0
	endoffset := 1

	b := bytes.Buffer{}
	b.Write(ret[:endoffset])

	entry.Version = 1

	valacctypes.EncodeVarInt(&b,uint64(entry.Version))

	offset++
	endoffset++

	ret[offset] = byte(len(entry.DDII))

	if ret[offset] == 0 {
		return nil, fmt.Errorf("BVCEntry marshal error: entry.DDII has zero length")
	}
	endoffset += int(ret[offset])
	offset++

	copy(ret[offset:endoffset],entry.DDII);
	offset = endoffset-1
	endoffset += 4

	binary.BigEndian.PutUint64(ret[offset:endoffset],uint64(entry.BVCHeight))
	offset += 4
	endoffset += 8

	binary.BigEndian.PutUint64(ret[offset:endoffset],entry.Timestamp)
	offset += 8
	endoffset += 32


	copy(ret[offset:endoffset],entry.MDRoot[:])

	return ret[:],nil
}


func (entry *BVCEntry) UnmarshalBinary(data []byte) ([][]byte, error) {


	//version, offset := varintf.Decode(data)
	version, _ := valacctypes.DecodeVarInt(data)
	offset := 0
	if version != 1 {
		return nil, fmt.Errorf("Invalid version")
	}
	entry.Version = byte(version)
	ddiilen := data[offset]
	if ddiilen > 32 && ddiilen > 0 {
		return nil, fmt.Errorf("Invalid DDII Length.  Must be > 0 && <= 32")
	}

	offset++
	endoffset := offset + int(ddiilen)
	if endoffset+4+16+32+1 > len(data) {
		return nil, fmt.Errorf("Insuffient data for parsing BVC Entry")
	}
	entry.DDII = make([]byte,ddiilen)

	copy(entry.DDII, data[offset:endoffset+1])

	ret := make([][]byte,4)

	ret[DDII_type] = entry.DDII

	offset = endoffset
	endoffset = offset + 4
	ret[BVCHeight_type] = data[offset:endoffset+1]
	entry.BVCHeight = int64(binary.LittleEndian.Uint64(ret[BVCHeight_type]))


	offset = endoffset
	endoffset = offset + 4
	ret[Timestamp_type] = data[offset:endoffset+1]
	entry.Timestamp = binary.LittleEndian.Uint64(ret[Timestamp_type])

	offset = endoffset
	endoffset = offset + 32

	ret[MDRoot_type] = data[offset:endoffset+1]
	copy(entry.MDRoot[:],ret[MDRoot_type])
	return ret,nil
}


