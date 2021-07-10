package types

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"

	//fold in the types to accumulate
	valacctypes "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"

	"math/rand"
	//"github.com/AccumulateNetwork/SMT/smt"
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


// Entry represents a Factom Entry.
//
// Entry can be used to Get data when the Hash is known, or submit a new Entry
// to a given ChainID.
type Entry struct {
	// An Entry in EBlock.Entries after a successful call to EBlock.Get has
	// its ChainID, Hash, and Timestamp.
    Version uint64
	ChainID   *[32]byte  `json:"chainid,omitempty"`
	Hash      *[32]byte  `json:"entryhash,omitempty"`
	//EntryHash  *[32]byte  `json:"entryhash,omitempty"`

	// Entry.Get populates the Content and ExtIDs.
	//array of external id's
	ExtIDs  [][]byte `json:"extids"`
	Content []byte   `json:"content"`

	marshalBinaryCache []byte
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

// ClearMarshalBinaryCache discards the cached MarshalBinary data.
//
// Subsequent calls to MarshalBinary will re-construct the data from the fields
// of the Entry.
func (e *Entry) ClearMarshalBinaryCache() {
	e.marshalBinaryCache = nil
}

// IsPopulated returns true if e has already been successfully populated by a
// call to Get.
func (e Entry) IsPopulated() bool {
	return e.ChainID != nil &&
		e.ExtIDs != nil &&
		e.Content != nil
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

// ComputeEntryHash returns the Entry hash of data. Entry's are hashed via:
// sha256(sha512(data) + data).
func ComputeEntryHash(data []byte) [32]byte {
	sum := sha512.Sum512(data)
	saltedSum := make([]byte, len(sum)+len(data))
	i := copy(saltedSum, sum[:])
	copy(saltedSum[i:], data)
	return sha256.Sum256(saltedSum)
}

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

	var chainID [32]byte
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

		e.ExtIDs = append(e.ExtIDs, []byte(data[i:i+extIDSize]))
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


type EntryCommitReveal struct {
	Entry Entry
	Segwit EntryCommit //discarded after validation
}

func (ecr EntryCommitReveal) Marshal() ([]byte, error) {

	data := make([]byte,ecr.Entry.MarshalBinaryLen() + ecr.Segwit.MarshalCommitLen())

	entrydata, err := ecr.Entry.MarshalBinary()
	if err != nil {
		return nil, err
	}

	l := copy(data,entrydata)

	commitdata := ecr.Segwit.MarshalCommit()

	if commitdata == nil {
		return nil, fmt.Errorf("Invalid data for marshal of commit")
	}

	copy(data[l:], commitdata)

	return data, nil
}

func (ecr EntryCommitReveal) Unmarshal(data []byte) error {

	err := ecr.Entry.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	err = ecr.Segwit.UnmarshalCommit(data)
	if err != nil {
		return err
	}

	return nil
}



const EntryCommitLedgerSize int = 1+6+32+1
const ChainCommitLedgerSize int = EntryCommitLedgerSize + 32 + 32
const EntryCommitSize int = EntryCommitLedgerSize+32+64
const ChainCommitSize int = ChainCommitLedgerSize+32+64



//
//func GenerateCommit(e *Entry) EntryCommit {
//	return EntryCommit{}
//}

func (ec *EntryCommit) isChainCommit() bool {
	return ec.ChainIDHash != nil && ec.ChainWeld != nil
}

func getInt48BE(data []byte) int64 {
	const size = 6
	var x int64
	for i := 0; i < size; i++ {
		x |= int64(data[i]) << (8 * (size - 1 - i))
	}
	return x
}

func putInt48BE(data []byte, x int64) {
	const size = 6
	for i := 0; i < size; i++ {
		data[i] = byte(x >> (8 * (size - 1 - i)))
	}
}

func (ec *EntryCommit) MarshalCommitLen() int {
	commitsize := EntryCommitLedgerSize
	if ec.isChainCommit() {
		commitsize = ChainCommitLedgerSize
	}
	return commitsize
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
	ledger := ec.MarshalLedger()
	sig := ec.Signature.Bytes()
	ret := make([]byte,len(ledger)+len(sig))

	ll := copy(ret, ledger)
	copy(ret[ll:],sig)

	return ret[:]
}

func (ec *EntryCommit) UnmarshalLedger(data []byte) ([]byte,error) {
	var rem []byte
	commitsize := len(data)
	asChainCommit := false
	if commitsize != EntryCommitSize {
		if commitsize != ChainCommitSize {
			return nil, fmt.Errorf("Invalid entry commit size for unmarshalling.")
		}
		asChainCommit = true
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
//
//func (ec *EntryCommit) EntryHash() valacctypes.Hash {
//	return sha256.Sum256(ec.MarshalCommit())
//}
// sha256(sha256(data))
func sha256d(data []byte) [sha256.Size]byte {
	hash := sha256.Sum256(data)
	return sha256.Sum256(hash[:])
}

///Generate a commit binary.  entrydata is either the adi/chainid or it is the raw data.
func GenerateCommit(entrydata []byte, entryhash *valacctypes.Hash,
	newChain bool) ([]byte, valacctypes.Hash) {

	commitSize := EntryCommitSize
	if newChain {
		commitSize = ChainCommitSize
	}

	commit := make([]byte, commitSize)

	i := 1 // Skip version byte

	// ms is a timestamp salt in milliseconds.
	ms := valacctypes.TimeStamp(time.Now().Unix()*1e3 + rand.Int63n(1000))
	i += copy(commit[i:], ms.Bytes()[2:8])

	if newChain {
		chainID := entrydata[1 : 1+len(valacctypes.Hash{})]
		// ChainID Hash
		chainIDHash := sha256d(chainID)
		i += copy(commit[i:], chainIDHash[:])

		// Commit Weld sha256d(entryhash | chainid)
		weld := sha256d(append(entryhash[:], chainID[:]...))
		i += copy(commit[i:], weld[:])
	}

	// Entry Hash
	i += copy(commit[i:], entryhash[:])

	cost, _ := EntryCost(len(entrydata), newChain)
	commit[i] = byte(cost)
	i++

	txID := sha256.Sum256(commit[:i])
	//
	//// Public Key
	//signedDataSize := i
	//i += copy(commit[i:], es.PublicKey())
	//
	//// Signature
	//sig := ed25519.Sign(es.PrivateKey(), commit[:signedDataSize])
	//copy(commit[i:], sig)

	return commit, txID
}


func SignCommit( es valacctypes.PrivateKey, commit []byte) error {

	if len(commit) != EntryCommitSize {
		if len(commit) != ChainCommitSize {
			return fmt.Errorf("Entry is neither a Chain Commit nor Entry Commit. Cannot sign.")
		}
	}

	// Public Key
	signedDataSize := len(commit) - 32 - 64
	i := signedDataSize
	i += copy(commit[i:], es.GetPublicKey()) //es.PublicKey())

	// Signature
	sig := es.Sign(commit[:signedDataSize])
	copy(commit[i:], sig)

	return nil
}

// NewChainCost is the fixed added cost of creating a new chain.
const NewChainCost = 10

// EntryCost returns the required Entry Credit cost for an entry with encoded
// length equal to size. An error is returned if size exceeds 10275.
//
// Set newChain to true to add the NewChainCost.
func EntryCost(size int, newChain bool) (uint8, error) {
	if size < EntryHeaderSize {
		return 0, fmt.Errorf("invalid size")
	}
	size -= EntryHeaderSize
	if size > 10240 {
		return 0, fmt.Errorf("Entry cannot be larger than 10KB")
	}
	cost := uint8(size / 1024)
	if size%1024 > 0 {
		cost++
	}
	if cost < 1 {
		cost = 1
	}
	if newChain {
		cost += NewChainCost
	}
	return cost, nil
}
