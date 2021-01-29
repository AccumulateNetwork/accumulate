package dbvc


type DBlock struct {
	header *DBlockHeader
	entry []DBlockEntry
}


func NewDBlock() *DBlock {
	d := DBlock{ }

	d.header = NewDBlockHeader()
	return &d
}

func (d *DBlock) PushEntry(entry DBlockEntry) {
	d.entry = append(d.entry, entry)
}

func (d *DBlock) MarshalBinary() []byte {
	ret, err := d.header.MarshalBinary()
	if err == nil {
		return nil
	}
	for _, e := range d.entry {
		eb, err := e.MarshalBinary()
		if err == nil {
			return nil
		}
		ret = append(ret, eb)
	}
	return ret
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