package tendermint

import (
	"encoding/binary"
	"fmt"

	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"

	"github.com/AccumulateNetwork/accumulated/factom/varintf"
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
	BVCHeight uint32          /// (4 bytes) Height of master chain block
	Timestamp uint64
	MDRoot valacctypes.Hash

}

func (entry *BVCEntry) MarshalBinary()([]byte, error) {
	ret := make([]byte,1+1+len(entry.DDII)+4+8+32)

	offset := 0
	endoffset := 1
	varintf.Put(ret[:endoffset],uint64(entry.Version))
	offset++
	endoffset++

	ret[offset] = byte(len(entry.DDII))

	endoffset += int(ret[offset])
	offset++

	copy(ret[offset:endoffset],entry.DDII);
	offset = endoffset-1
	endoffset += 4

	binary.BigEndian.PutUint32(ret[offset:endoffset],entry.BVCHeight)
	offset += 4
	endoffset += 8

	binary.BigEndian.PutUint64(ret[offset:endoffset],entry.Timestamp)
	offset += 8
	endoffset += 32

	copy(ret[offset:endoffset],entry.MDRoot[:])

	return ret[:],nil
}


func (entry *BVCEntry) UnmarshalBinary(data []byte) ([][]byte, error) {


	version, offset := varintf.Decode(data)
	if offset != 1 {
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
	entry.DDII = data[offset:endoffset+1]

	ret := make([][]byte,4)

	ret[DDII_type] = entry.DDII

	offset = endoffset
	endoffset = offset + 4
	ret[BVCHeight_type] = data[offset:endoffset+1]
	entry.BVCHeight = binary.LittleEndian.Uint32(ret[BVCHeight_type])


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


