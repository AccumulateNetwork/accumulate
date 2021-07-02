package tendermint

import (
	"crypto/sha256"
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

type EntryCommitReveal struct {
	entry Entry
	segwit EntryCommit //discarded after validation
}

// IsPopulated returns true if e has already been successfully populated by a
// call to Get.
func (e Entry) IsPopulated() bool {
	return len(e.ChainID) != 0 &&
		e.ExtIDs != nil &&
		e.Content != nil
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

	if len(commit) != ChainCommitSize || len(commit) != EntryCommitSize {
		return fmt.Errorf("Entry is neither a Chain Commit nor Entry Commit. Cannot sign.")
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
