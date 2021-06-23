package tendermint

import (
	"crypto/sha256"
	"fmt"
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"time"

	"crypto/ed25519"
)

type EntryCommit struct {
	Version	uint64
	Timestamp valacctypes.TimeStamp
	Entryhash valacctypes.Hash
	Numec int8
	Signature valacctypes.Signature //signature contains signature and pubkey

	ChainIDHash *valacctypes.Hash
	ChainWeld *valacctypes.Hash
	//varInt_F	Version	starts at 0. Higher numbers are currently rejected. Can safely be coded using 1 byte for the first 127 versions.
	//6 bytes	milliTimestamp	This is a timestamp that is user defined. It is a unique value per payment. This is the number of milliseconds since 1970 epoch.
	//32 bytes	Entry Hash	This is the SHA512+256 descriptor of the Entry to be paid for.
	//1 byte	Number of Entry Credits	This is the number of Entry Credits which will be deducted from the balance of the public key. Any values above 10 are invalid.
	//32 bytes	Pubkey	This is the Entry Credit public key which will have the balance reduced. It is the ed25519 A value.
	//64 bytes	Signature	This is a signature of this Entry Commit by the pubkey. Parts ordered R then S. Signature covers from Version through 'Number of Entry Credits'
}

type ChainCommit struct {
	EntryCommit
	ChainIdHash valacctypes.Hash
	CommitWeld valacctypes.Hash
//	32 bytes	ChainID Hash	This is a double hash (SHA256d) of the ChainID which the Entry is in.
//	32 bytes	Commit Weld	SHA256(SHA256(Entry Hash | ChainID)) This is the double hash (SHA256d) of the Entry Hash concatenated with the ChainID.
}

Entr


// Entry represents a Factom Entry.
//
// Entry can be used to Get data when the Hash is known, or submit a new Entry
// to a given ChainID.
type Entry struct {
	// An Entry in EBlock.Entries after a successful call to EBlock.Get has
	// its ChainID, Hash, and Timestamp.
    Version uint64
	ChainID   [32]byte  `json:"chainid,omitempty"`
	//EntryHash  *[32]byte  `json:"entryhash,omitempty"`

	// Entry.Get populates the Content and ExtIDs.
	//array of external id's
	ExtIDs  *[]byte `json:"extids"`
	Content []byte   `json:"content"`

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

// IsPopulated returns true if e has already been successfully populated by a
// call to Get.
func (e Entry) IsPopulated() bool {
	return e.ChainID != nil &&
		e.ExtIDs != nil &&
		e.Content != nil
}

const EntryCommitLedgerSize int = 1+6+32+1
const ChainCommitLedgerSize int = EntryCommitLedgerSize + 32 + 32
const EntryCommitSize int = EntryCommitLedgerSize+32+64
const ChainCommitSize int = ChainCommitLedgerSize+32+64

func GenerateCommit(e *Entry) EntryCommit {
	return EntryCommit{}
}

func (ec *EntryCommit) isChainCommit() bool {
	return ec.ChainIDHash != nil && ec.ChainWeld != nil
}

func (ec *EntryCommit) MarshalLedger() []byte {
	commitsize := EntryCommitLedgerSize
	asChainCommit := ec.isChainCommit()
	if asChainCommit {
		commitsize = ChainCommitLedgerSize
	}

	ret := make([]byte,commitsize)
	startoffset := 0
	endoffset := 1

	l := copy(ret[startoffset:endoffset], valacctypes.EncodeVarIntGoBytes(ec.Version))
	startoffset = endoffset
	endoffset += l

	l = copy(ret[startoffset:endoffset], ec.Timestamp.Bytes()[2:8])
	startoffset = endoffset
	endoffset += l

	if asChainCommit {
		l = copy(ret[startoffset:endoffset], ec.ChainIDHash.Bytes())
		startoffset = endoffset
		endoffset += l

		l = copy(ret[startoffset:endoffset], ec.ChainWeld.Bytes())
		startoffset = endoffset
		endoffset += l
	}

	l = copy(ret[startoffset:endoffset],ec.Entryhash.Bytes())
	startoffset = endoffset
	endoffset += l

	ret[startoffset] =byte(ec.Numec)
	startoffset = endoffset

	return ret
}

func (ec *EntryCommit) MarshalCommit() []byte {
	ret := make([]byte,EntryCommitSize)

	ll := copy(ret, ec.MarshalLedger())
	copy(ret[ll:],ec.Signature.Bytes())

	return ret[:]
}

func (ec *EntryCommit) UnmarshalLedger(data []byte) ([]byte,error) {
	var rem []byte
	commitsize := EntryCommitLedgerSize
	asChainCommit := ec.isChainCommit()
	if asChainCommit {
		commitsize = ChainCommitLedgerSize
	}

	if len(data) < commitsize {
		return nil,fmt.Errorf("Invalid entry commit size for unmarshalling")
	}

	ec.Version, rem = valacctypes.DecodeVarInt(data)
	if len(rem) < commitsize-1 {
		return nil,fmt.Errorf("Invalid entry commit size after version")
	}

	var ts [8]byte
	copy(ts[2:8],rem[0:6])
	ec.Timestamp.Extract(ts[:])

    rem = rem[6:]

	if asChainCommit {
		ec.ChainIDHash = new(valacctypes.Hash)
		rem = ec.ChainIDHash.Extract(rem)

		ec.ChainWeld = new(valacctypes.Hash)
		rem = ec.ChainWeld.Extract(rem)
	}

	rem = ec.Entryhash.Extract(rem)

	ec.Numec = int8(rem[0])

	if len(rem) == 1 {
		return nil, nil
	}

	return rem[1:],nil
}

func (ec *EntryCommit) UnmarshalCommit(data []byte) error {
	rem, err := ec.UnmarshalLedger(data)
	if err == nil && len(rem) >= 32+64 {
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
	return sha256.Sum256(ec.MarshalCommit())
}
