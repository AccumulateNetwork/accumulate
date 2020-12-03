# SMT

Stateful Merkle Trees provide the support for building and maintaining information in multiple ordered and distributed blockchains.

The SMT implementation is a layered one:

* SMT  
  * Construction: collect hashes and add to Merkle Tree
  * Build: Time Stamping, block building
  * Persistence: Record entries, Merkle States to the database
  * Validation: Validate entries to be added to the Merkle Tree
  
## SMT
* `func NewSMT() SMT`
  
   create a new SMT object to manage a Merkle tree   
### Construction
 
* `func (m \*SMT) AddHash([]byte)` add a hash to the merkle tree
* `func (m \*SMT) GetState() (index int64, state [][]byte)` Returns State 
         of Merkle tree.  The index of the last hash added, the Merkle Tree state, and the 
         ordered list of entry hashes as they were added to the Merkle Tree
* `func (m \*SMT) SetState(index int64, state [][]byte)` Sets the state 
         of SMT to allow building on an existing Merkle Tree
* `func (m \*SMT) GetDagRoot() []byte` Returns the Dag Root for the 
         current state of the Merkle Tree
   
### Build 

* `func (m \*SMT) EndBlock() (dagRoot []byte, index int64, state [][]byte)` 
             
   ends the current block by adding the timestamp and header to the Merkle tree to SMT.  
   The `dagRoot` sans the transaction

### Validation
```Go
type Hash [32]byte
```
```Go
type Fee struct {
    TimeStamp      int64        // 8
    DDII           Hash         // 32
    ChainID        [33]byte     // 33
    Credits        int8         // 1
    SignatureIdx   int8         // 1
    Signature      [64]byte     // 64  
                                // 1 end byte ( 140 bytes for FEE)
    Transaction    []byte       // Transaction
}
```

```Go
type Validation interface {
    Type() string
    SetState(state *StateEntry)
    GetState() state *StateEntry
	ValidTx(entry *Entry) bool
	ValidTxList(entries []*Hash) bool
}
```
* `func (m \*SMT) SetValidator(validator *Falidation)`  Set the validator function on the SMT

     