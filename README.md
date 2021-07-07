# SMT

Stateful Merkle Trees provide the support for  building and maintaining
information in Merkle Trees.  On the face of this, SMT isn't very different
from how Merkle Trees have been used in blockchains since Bitcoin.  However,
SMT allows a blockchain to maintain Merkle Trees spanning many blocks, and
even Merkle Trees of Merkle Trees.

An SMT can collect hashes of validated data from multiple sources and
create a Merkle Tree that orders all these hashes by arrival time to the
SMT.  The order of all sources is maintained.

An additional feature is the ability to order entries into blocks, but
allow the Merkle Tree to span blocks. This means that all the data collected
is in fact organized into a single Merkle Tree, but sets of entries
are understood to represent blocks.

SMT goes further, and allows Merkle Trees to be maintained over time, but
anchored into parent Merkle Trees. In other words, SMT allows validators to
submit hashes of validated entries into SMT and maintain those entries in
their own Merkle Trees that, on block boundaries, are anchored into other
Merkle Trees, building a Hierarchical set of Merkle Trees,  or a Merkle Tree
of Merkle trees.

A single Merkle Tree for example could take as elements the letters of the
Alphabet.  In this notation, a and b represent hashes, and ab represents
some set of hashing of a and b.  Because we combine hashes on power of 2
boundaries, we can understand how any set of characters is combined.

for example, abcdefgh is h( h( h(a+b), h(c+d) ), h( h(e,f), h(g,h) ) )

```
a  b  c  d  e  f  g  h  i  j  k  l  m  n  o  p  q  r  s  t  u  v  w  x  y  z
  ab    cd    ef    gh    ij    kl    mn    op    qr    st    uv    wx   [yz]
      abcd        efgh        ijkl        mnop        qrst        uvwx
              abcdefgh                ijklmnop               [qrstuvwx]
                             [abcdefghijklmnop] 
```

The roots of this tree are **abcdefghijklmnop**, **qrstuvwx**, and **yz**.
Given a hash function h(x), and a concatenation
operator +, The DAG root of the above partial Merkle Tree is

> h(abcdefghijklmnop + h([qrstuvwx] + [yz]))

In order to create blocks, suppose the alphabet were entered into a
blockchain that managed 5 elements per block (and we put any leftovers in
their own block).  Then the above blockchain of characters becomes a Merkle
tree split up as follows, with each block labeled d0-d5:

```
a b c d e    f g h i j     k l m n o          p q r s t     u v w x y    z
 ab  cd |   ef  gh  ij      kl  mn |         op  qr  st      uv  wx |   yz
   abcd |     efgh   |    ijkl     |       mnop    qrst        uvwx |    |
        | abcdefgh   |             |     ijklmnop     |    qrstuvwx |    |
        |            |             | abcdefghijklmnop |             |    |
        |            |             |                  |             |    |
       d0           d1            d2                 d3            d4   d5
```

Note that we end up with six blocks, with the following elements:

* a b c d e
* f g h i j
* k l m n o
* p q r s t
* u v w x y
* z

Also note that each block has a DAG root. Consider the DAG roots below,
labeled d0 to d5 as follows:

* d0 = h(abcd+e)
* d1 = h(abcdefgh + ij)
* d2 = h(abcdefgh + h(ijkl + h(mn+o)))
* d3 = h(abcdefghijklmnop +qrst)
* d4 = h(abcdefghijklmnop + h(qrstuvwx + y))
* d5 = h(abcdefghijklmnop + h(qrstuvwx + yz)

Now we can construct a Merkle Tree that takes as elements the roots of a set
of merkle trees.  Further note that regardless of the rules for constructing
blocks, any blockchain can fit within a Merkle Tree rather than fitting
Merkle Trees into blocks, using the method described above.

So suppose that we have not just d0-d5, but e0-e5, f0-f5, and g0-g5 where d,
e,f,g all represent DAG Roots of merkle trees.  We can build a root Merkle
Tree that would take these elements:

```
d0 e0 f0 g0  d1 e1 f1 g1  d2 e2 f2 g2      d3 e3 f3 g3  d4 e4 f4 g4  d5 e5 f5 g5
 d0e0  f0g0   d1e1  f1g1   d2e2  f2g2       d3e3  f3g3   d4e4  f4g4   d5e5  f5g5
   d0e0f0g0     d1e1f1g1     d2e2f2g2         d3e3f3g3     d4e4f4g4     d5e5f5g5
        d0e0f0g0d1e1f1g1              d2e2f2g2d3e3f3g3          d4e4f4g4d5e5f5g5
                      d0e0f0g0d1e1f1g1d2e2f2g2d3e3f3g3
```

Due to the limitations of a document like this, flushing out how deep the
merkle trees can be nested will not be illustrated.  If the
root MerKle Tree can hold the DAGs of other Merkle Trees, those Merkle Trees
can also hold DAGS of other Merkle Trees.

## Database

The database used underneath the MerkleManager is a key value store that 
uses the concepts of salts, buckets, and labels to organize the key value 
pairs generated in the process of building multiple Merkle Trees.

Salts provide a way of separating one Merkle Tree's data and hashes from 
another. Some overall tracking of the contents of the database is done 
without a salt, i.e. at the root level.  Most if not all salts will be the 
ChainID (hash of the chain URL).

* **Salt** *Root, no Salt*, no label, no key: Index Count
   * **Index2Salt** Index / Salt  -- Given a Salt Index returns a Salt
   * **Salt2Index** Salt / Index -- Given a Salt returns a Salt Index
* **ElementIndex** *Salted by ChainID* element hash / element Index
* **States** *Salted by ChainID* element index / Merkle State -- Given an 
  element index in a Merkle Tree, return the Merkle State at that point. Not 
  all element indexes are defined
* **NextElement** *Salted by ChainID* element index / next element -- 
  Returns the next element to be added to the Merkle Tree at a given index
* **Element** *Salted by ChainID* no label, no key: Count of elements in the 
  merkle tree
  
Buckets used by the MerkleManager:

* **BucketIndex** *Salted by ChainID* element index / BlockIndex struct -- A 
  BlockIndex struct has three indexes: The BVC block index, the Main chain 
  index, and the Pending Index.  Either of the Main chain index or the 
  Pending Index can be -1 (indicating no entries in this block, but not both.  
  * **\<blockIndex\>** -- Using the block index as a key, returns the 
    BlockIndex struct at that index in the Block Index Chain (BlxIdxChain).
  * **\<none\>** -- Providing no label or key, returns the highest 
    BlockIndex in the Block Index Chain (BlxIdxChain)
  
## SMT/managed/MerkleManager
The MerkleManager is used to build and persist Merkle Trees.  Features 
include the dynamic construction of Merkle Trees, saving such Merkle Trees 
to a specified key value store, handling of multiple Merkle Trees, maintenance 
of shadow Merkle Trees, and more.

* `func  NewMerkleManager(DBManager *database.Manager, markPower int64, 
  Salt []byte) *MerkleManager` Returns a MerkleManager using the 
  given initial salt
* `func (m MerkleManager) Copy(salt []byte)` Effectively points the 
  MerkleManager to point to a MerkleTree associated with the given salt
  `func (m MerkleManager) CurrentSalt() (salt []byte)` Returns the current 
  salt in used by the MerkleManagher
* `func (m MerkleManager) GetLementCount() (elementCount int64)` Returns the 
  count of elements in the Stateful Merkle Tree currently under the focus of 
  the MerkleManager
* `func (m MerkleManager) SetBlockIndex(blockIndex int)` Mark this point in the 
  Merkle 
  Tree as the last element in a block
* `func (m MerkleManager) GetState(element int64)` Get the state of the 
  MerkleTree at the given 
  element index.  If no state was stored at that index, returns nil.  Note
  that the frequency at which states are stored is determined by the mark power.
* `func (m MerkleManager) GetNext(element int64)` Return the element added 
  at this index.  We store
  the state just prior to moving into the power of 2, and the element that
  when added brings the merkle tree to a power of 2 (frequency determined by
  the mark power).  This is key to generating receipts efficently from a 
  stateful merkle tree
* `func (m MerkleManager) AddHash(hash Hash)` Adds a hash to the current
      MerkleTree under management
* `func (m MerkleManager)  AddHashString(hash string)`  Does a conversion of 
  a hex string to a binary hash before calling AddHash.  Panics if provided 
  a string that is not a valid hex string.
  
## SMT/managed/Receipt
Receipts allow the generation and validation of the elements in a Merkle 
Tree against other, later elements in the Merkle Tree.  The use case this 
supports is the dynamic construction of Merkle Trees that are anchored over 
time either to other merkle trees or to blockchains.
* `GetReceipt(manager *MerkleManager, element Hash, anchor Hash) *Receipt` 
  Given the hash of an element in the Merkle Tree, and the hash of an 
  element that has been anchored externally, compute the receipt of the 
  element hash.  Note that the element index must be less than or equal to 
  the anchor index. If either hash is not a member of the merkle tree, or 
  the hash of the element has an index higher than the index of the anchor, 
  a nil reciept is returned.
* `(r Receipt) Validate` applies the hashes as specified by the receipt and 
  validates that the element hash is proven by the anchor hash.  

## SMT/managed/MerkleState
Supports the dynamic construction of Merkle Trees.  Note that the 
MerkleState only handles the "storm front" of merkle tree construction, such 
that the state is updated as elements are added.  The MerkleState object 
does not persist the elements of a merkle tree. This is done externally, and 
in this library this is done by the MerkleManager.
* `(m MerkleState) Copy()` Returns a new copy of the MerkleState
* `(m MerkleState) CopyAndPoint()` Returns a pointer to a new copy of the 
  MerkleState
* `(m MerkleState) Equal(m2 MerkleState) (isEqual bool)` Returns true if the 
  MerkleState is equal to the m2 MerkleState
* `(m MerkleState) Marshal() (MSBytes[]byte)` Returns a serialized version 
  of the MerkleState
* `(m *MerkleState) UnMarshal(MSBytes []byte)` Decodes the MerkleState from 
  the given bytes and updates the MerkleState to reflect that state.  
* `(m *MerkleState) AddToMerkleTree(hash_ [32]byte` Add a given Hash to the 
  Merkle Tree and update the Merkle State
  


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
    Signature      []byte       // 64 minimum 
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
* `func (m \*SMT) SetValidator(validator *Validation)`  Set the validator function on the SMT

     