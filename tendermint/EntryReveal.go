package tendermint

// EntryHeaderSize is the exact length of an Entry header.
const EntryHeaderSize = 1 + // version
	32 + // chain id
	2 // total len

// EntryMaxDataSize is the maximum data length of an Entry.
const EntryMaxDataSize = 10240

// EntryMaxTotalSize is the maximum total encoded length of an Entry.
const EntryMaxTotalSize = EntryMaxDataSize + EntryHeaderSize

//
//import (
//	"crypto/sha256"
//	"fmt"
//	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
//
//	"crypto/ed25519"
//)
//
//type EntryCommit struct {
//	Version	uint64
//	ChainId [32]byte //either a ddii or hash
//	ExtIdSize int16 //stored big endian
//	Numec int8
//	Signature valacctypes.Signature //signature contains signature and pubkey
//	//Header
//	//varInt_F	Version	starts at 0. Higher numbers are currently rejected. Can safely be coded using 1 byte for the first 127 versions.
//	//32 bytes	ChainID	This is the Chain which the author wants this Entry to go into.
//	//2 bytes	ExtIDs Size	Describes how many bytes required for the set of External IDs for this Entry. Must be less than or equal to the Payload size. Big endian.
//	//Payload		This is the data between the end of the Header and the end of the Content.
//	//External IDs		This section is only interpreted and enforced if the External ID Size is greater than zero.
//	//2 bytes	ExtID element 0 length	This is the number of the following bytes to be interpreted as an External ID element. Cannot be 0 length.
//	//variable	ExtID 0	This is the data for the first External ID.
//	//2 bytes	ExtID X	Size of the X External ID
//	//variable	ExtID X data	This is the Xth element. The last byte of the last element must fall on the last byte specified ExtIDs Size in the header.
//	//Content
//	//variable	Entry Data	This is the unstructured part of the Entry. It is all user specified data.
//}
//const EntryCommitLedgerSize int = 1+6+32+1
//const EntryCommitSize int = EntryCommitLedgerSize+32+64
//
//func (ec *EntryCommit) MarshalLedger() []byte {
//	ret := make([]byte,EntryCommitLedgerSize)
//
//	startoffset := 0
//	endoffset := 1
//	l := copy(ret[startoffset:1], valacctypes.EncodeVarIntGoBytes(ec.Version))
//	startoffset = endoffset
//	endoffset += l
//
//	l = copy(ret[startoffset:endoffset], ec.Timestamp.Bytes()[2:8])
//	startoffset = endoffset
//	endoffset += l
//
//	l = copy(ret[startoffset:endoffset],ec.Entryhash.Bytes())
//	startoffset = endoffset
//	endoffset += l
//
//	ret[startoffset] =byte(ec.Numec)
//	startoffset = endoffset
//
//	return ret
//}
//
//func (ec *EntryCommit) Marshal() []byte {
//	ret := make([]byte,EntryCommitSize)
//
//	ll := copy(ret, ec.MarshalLedger())
//	copy(ret[ll:],ec.Signature.Bytes())
//
//	return ret[:]
//}
//
//func (ec *EntryCommit) UnmarshalLedger(data []byte) ([]byte,error) {
//
//	var rem []byte
//
//	if len(data) < EntryCommitLedgerSize {
//		return nil,fmt.Errorf("Invalid entry commit size for unmarshalling")
//	}
//
//	ec.Version, rem = valacctypes.DecodeVarInt(data)
//	if len(rem) < EntryCommitLedgerSize-1 {
//		return nil,fmt.Errorf("Invalid entry commit size after version")
//	}
//
//	var ts [8]byte
//
//	copy(ts[2:8],rem[0:6])
//	ec.Timestamp.Extract(ts[:])
//
//	rem = ec.Entryhash.Extract(rem[6:6+32])
//
//	ec.Numec = int8(rem[0])
//
//	if len(rem) == 1 {
//		return nil, nil
//	}
//
//	return rem[1:],nil
//}
//
//func (ec *EntryCommit) Unmarshal(data []byte) error {
//
//	rem, err := ec.UnmarshalLedger(data)
//	if err == nil {
//		ec.Signature.Extract(rem)
//	}
//
//	return err
//}
//
//func (ec *EntryCommit) Valid() bool {
//	return ed25519.Verify(ec.Signature.PublicKey, ec.MarshalLedger(),ec.Signature.Signature[:])
//}
//
//func (ec *EntryCommit) TXID() valacctypes.Hash {
//	return sha256.Sum256(ec.MarshalLedger())
//}
//
//func (ec *EntryCommit) EntryHash() valacctypes.Hash {
//	return sha256.Sum256(ec.Marshal())
//}
/*
// Cost returns the EntryCost of e, using e.MarshalBinaryLen().
//
// If e.ChainID == nil, the NewChainCost is added.
func (e Entry) Cost() (uint8, error) {
	return EntryCost(e.MarshalBinaryLen(), e.ChainID == nil)
}

// MarshalBinaryLen returns the total encoded length of e.
func (e Entry) MarshalBinaryLen() int {
	extIDTotalSize := len(e.ExtIDs) * 2 // Two byte len(ExtID) per ExtID
	for _, extID := range e.ExtIDs {
		extIDTotalSize += len(extID)
	}
	return EntryHeaderSize + extIDTotalSize + len(e.Content)
}

// MarshalBinary returns the raw Entry data for e. This will return an error if
// !e.IsPopulated(). The data format is as follows.
//
//      [Version byte (0x00)] +
//      [ChainID (Bytes32)] +
//      [Total ExtID encoded length (uint16 BE)] +
//      [ExtID 0 length (uint16)] + [ExtID 0 (Bytes)] +
//      ... +
//      [ExtID X length (uint16)] + [ExtID X (Bytes)] +
//      [Content (Bytes)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e Entry) MarshalBinary() ([]byte, error) {
	if len(e.marshalBinaryCache) > 0 {
		return e.marshalBinaryCache, nil
	}

	if e.ChainID == nil {
		return nil, fmt.Errorf("missing ChainID")
	}

	totalSize := e.MarshalBinaryLen()
	if totalSize > EntryMaxTotalSize {
		return nil, fmt.Errorf("length exceeds %v", EntryMaxTotalSize)
	}

	// Header, version byte 0x00
	data := make([]byte, totalSize)
	i := 1
	i += copy(data[i:], e.ChainID[:])
	binary.BigEndian.PutUint16(data[i:i+2],
		uint16(totalSize-len(e.Content)-EntryHeaderSize))
	i += 2

	// Payload
	for _, extID := range e.ExtIDs {
		n := len(extID)
		binary.BigEndian.PutUint16(data[i:i+2], uint16(n))
		i += 2
		i += copy(data[i:], extID)
	}
	copy(data[i:], e.Content)

	return data, nil
}

// EntryHeaderSize is the exact length of an Entry header.
const EntryHeaderSize = 1 + // version
	32 + // chain id
	2 // total len

// EntryMaxDataSize is the maximum data length of an Entry.
const EntryMaxDataSize = 10240

// EntryMaxTotalSize is the maximum total encoded length of an Entry.
const EntryMaxTotalSize = EntryMaxDataSize + EntryHeaderSize

// UnmarshalBinary unmarshals raw entry data into e.
//
// If e.ChainID is not nil, it must equal the ChainID described in the data.
//
// If e.Hash is not nil, it must equal ComputeEntryHash(data).
//
// Like json.Unmarshal, if e.ExtIDs or e.Content are preallocated, they are
// reset to length zero and then appended to.
//
// The data must encode a valid Entry. Entries are encoded as follows:
//
//      [Version byte (0x00)] +
//      [ChainID (Bytes32)] +
//      [Total ExtID encoded length (uint16 BE)] +
//      [ExtID 0 length (uint16)] + [ExtID 0 (Bytes)] +
//      ... +
//      [ExtID X length (uint16)] + [ExtID X (Bytes)] +
//      [Content (Bytes)]
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *Entry) UnmarshalBinary(data []byte) error {

	if len(data) < EntryHeaderSize || len(data) > EntryMaxTotalSize {
		return fmt.Errorf("invalid length")
	}

	if data[0] != 0x00 {
		return fmt.Errorf("invalid version byte")
	}

	i := 1 // Skip version byte.

	var chainID Bytes32
	i += copy(chainID[:], data[i:i+len(e.ChainID)])
	if e.ChainID != nil {
		if *e.ChainID != chainID {
			return fmt.Errorf("invalid ChainID")
		}
	} else {
		e.ChainID = &chainID
	}

	extIDTotalSize := int(binary.BigEndian.Uint16(data[i : i+2]))
	if extIDTotalSize == 1 || EntryHeaderSize+extIDTotalSize > len(data) {
		return fmt.Errorf("invalid ExtIDs length")
	}
	i += 2

	e.ExtIDs = e.ExtIDs[0:0]

	for i < EntryHeaderSize+extIDTotalSize {
		extIDSize := int(binary.BigEndian.Uint16(data[i : i+2]))
		if i+2+extIDSize > EntryHeaderSize+extIDTotalSize {
			return fmt.Errorf("error parsing ExtIDs")
		}
		i += 2

		e.ExtIDs = append(e.ExtIDs, Bytes(data[i:i+extIDSize]))
		i += extIDSize
	}

	if e.Content == nil {
		e.Content = data[i:]
	} else {
		e.Content = append(e.Content[0:0], data[i:]...)
	}

	// Verify Hash, if set, otherwise populate it.
	hash := ComputeEntryHash(data)
	if e.Hash != nil {
		if *e.Hash != hash {
			return fmt.Errorf("invalid hash")
		}
	} else {
		e.Hash = &hash
	}

	// Cache data for efficient marshaling.
	e.marshalBinaryCache = data

	return nil
}
*/