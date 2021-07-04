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
  
## SMT API
* `func  NewMerkleManager(DBManager *database.Manager, markPower int64, 
  initialSalt []byte)
  *MerkleManager` Returns a MerkleManager using the given initial salt
  
* `func (m MerkleManager) Copy(salt []byte)` Effectively points the 
  MerkleManager to point to a MerkleTree associated with the given salt
* `func (m MerkleManager) SetBlockIndex()`    
  
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

     