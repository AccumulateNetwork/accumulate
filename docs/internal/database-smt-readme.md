# SMT

Stateful Merkle Trees provide the support for  building and maintaining
information in Merkle Trees.  On the face of this, SMT isn't very different
from how Merkle Trees have been used in blockchains since Bitcoin.  However,
SMT allows a blockchain to maintain Merkle Trees spanning many blocks, and
even Merkle Trees of Merkle Trees.

An SMT can collect hashes of validated data from multiple sources and
create a Merkle Tree that orders all these hashes by arrival time to the
SMT.  The arrival order of entries from all sources is maintained.

SMTs can be arranged in any way an application requires.  The SMT manager
can be redirected to update multiple SMTs.  Note that one of the logical
arrangements for SMT is to create an SMT of SMTs.

SMTs collect entries in lists maintained on power of 2 boundaries (i.e.
every 4, 8, ... 1024, .... boundaries. Blocks are documented by creating
indexes of blocks and elements in a shadow Merkle tree.  This is all that is
needed to be able to generate the state of the Merkle Tree at the end of any
block.

A single Merkle Tree, for example, could take as elements the letters of the
Alphabet.  In this notation, a and b represent hashes, and ab represents
hash resulting from hashing a and b together.  Because we combine
hashes on power of 2 boundaries, we can understand how to combine any set of n
characters where n is a power of 2.

for example, assume **H**(x) indicates the hash of x, and **+** is the
concatenation operator, then:
```
ab       = H( a+b )
abcd     = H( H( a+b ) + H( c+d ) )
abcdefgh = H( H( H( a+b ) + H( c+d ) ) + H( H( e+f ) + H( g+h ) ) )
```

Using this simplified notation, we can write out a Merkle Tree with 26
elements denoted by the letters a-z this way.  Note that at 26 entries,
three sub merkle trees are not yet combined, and are denoted with [ ]s.

```
a  b  c  d  e  f  g  h  i  j  k  l  m  n  o  p  q  r  s  t  u  v  w  x  y  z
  ab    cd    ef    gh    ij    kl    mn    op    qr    st    uv    wx   [yz]
      abcd        efgh        ijkl        mnop        qrst        uvwx
              abcdefgh                ijklmnop               [qrstuvwx]
                             [abcdefghijklmnop]
```

The roots of this tree are **abcdefghijklmnop**, **qrstuvwx**, and **yz**.
Given a hash function H(x), and a concatenation
operator +, The DAG root of the above partial Merkle Tree is
```
  H( abcdefghijklmnop + H( [qrstuvwx] + [yz] ) )
```
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

## Binary Patricia Trees (BPT)
Patricia Trees are designed to be able to create small cryptographic proofs
about the particular state of values at a particular point of time in some
process.  On a blockchain, a Patricia Tree can be used to prove the balance
of an account (an address) in a blockchain at a particular block height.

Accumulate organizes the blockchain into a series of chains under particular
identities.  Being able to prove the state of a particular chain at a
particular block height becomes critical.  The index of the block where a
chain was last modified is also important. The BPT provides these
proofs for the state of each chain.

To summarize:

* Identities can act as domains to allow addressing the blockchain as a set of
  URLs
* Chains are organized under Identities
* The Membership of Chains is proven using Stateful Merkle Trees. All
  entries in a chain are members of one, growing, Merkle Tree.  At any point
  in time, the membership of the Merkle Tree is provable by the Merkle State.
* Binary Patricia Trees are used to prove the state of every chain in a Block
  Validator Node at a particular block height

A Merkle Tree takes a set of records, then hashes them in order to produce a
Merkle Root.  The BPT does the same thing, adding a key per record to
organize where the records go into the BPT. So while a Merkle Tree organizes
the records by the order in which they are added, a Patricia tree organizes
records according to the key of each record. A principle feature of a
Patricia is that the adding order does not matter; the record set is what
matters.

At the heart of a blockchain is maintaining a proof of the state without
requiring all elements of that state to be at hand.  Accumulate organizes
all these proofs as a very large set of chains.  Even token transactions are
organized as chains where the membership and order of transactions from a
particular account exist in its own chain.

Knowing and proving what the complete state of a chain at a particular block
height becomes critical.  That is known if one can prove the last state
of the Merkle Tree that holds those transactions.

We place these states in our Patricia Tree.  The implementation here is the
Binary Patricia Tree.  The Keys used are hashes derived from the URL for
each chain.  The Values are the hash of the Merkle Tree States.

### Implementation

The quality we need in the Patricia Tree implementation is the ability to
quickly update its state, and that its state provides proofs of the state of
the entire protocol.  This implementation ensures that, no matter what order is
used to add records to a Patricia Tree, the same set of records produces
exactly the same Patricia Tree (Assuming no two records modify the same
state). Note that this is true of any Patricia Tree Implementation.

Further, subsets of the Patricia Tree are independently provably correct,
as long as there is a path to the root of the Patricia Tree to the
subsection of the Patricia tree.

The BPT is a Binary Patricia Tree, and perhaps more correctly called
a Binary Patricia-Merkle Tree.  The summary of all Key/Value pairs is a hash
that acts like the Merkle Root of a Merkle Tree, while additions or
modifications of the Merkle Tree only require localized re-computations.

Many implementations of Patricia Trees are described in the literature.
With BPT as are used by Accumulate, the keys are randomly distributed from
a binary point of  view because the keys are the hashes of URLs.  Mining
these hashes to build some particular pattern of leading bits is not very
manageable or possible as the keys derive from URLs.  URLs can certainly be
mined to have interesting leading bits, but little incentive exists to do so.

Given that the keys created from hashes can be relied on to be numerically
random, a BPT will be very well-balanced if this feature is exploited.
Note that we use the leading bits to organize entries in the BPT.  However,
nothing prevents using every 3rd bit, the trailing bits, or any other
random walk of bits in the key.  Should any attack be mounted to create
chainIDs that significantly unbalance the BPT, we can refactor the Patricia
Tree using any of these methods, and do so over time (reorganizing only
parts of the BPT at a time).

We have three entry types in the BPT:
* Node -- Node entries are used to organize the tree.  They have a left path
  and a right path, and exist at a height in the BPT.  The Height is used to
  consider a particular bit in the BPT (at that point).  The Left path is
  taken if the key has a zero bit at that point.  The Right path is taken
  if the key has a 1 at that point.
* Value -- The key value pair in the BPT.  Value entries have no children, and
  paths through the BPT from parent to child Node entries must end with either
  at a nil or a Value entry.
* Load -- Node represents the fact that the next Byte Block is not loaded,
  and needs to be loaded if the search is running through this part of the
  BPT.

Consider a set of keys that might be added to the BPT. The sequence of URLs
formed from acc://RedWagon/1, acc://RedWagon/2,acc://RedWagon/3, ... would
result in the following Keys, the first byte, and the binary of the first byte:
```
                                                                First    First
                             Keys                                Byte     Byte
  (Hashs of the URLs acc://RedWagon/1, acc://RedWagon2, ...)    in Hex   in Binary
694833d340c7e952163b3dd8a25bfeea8b1163971d4816093a0eb77889006e5b  69    [01101001]
a100ecde2c835a02f722a395311d3f070cf874e9945f42645ba8bcfe8f883f1c  a1    [10100001]
2de44ed61567e49d338c1a1dc42cd801e57080bfb925784d124b8027b7bc2a39  2d    [00101101]
fb1f13654fb5178b489c1dc3f5ac24e488b68ee7beedeaabbb3e90f5ff38c96e  fb    [11111011]
efdcd18725e4ab8bdc5521d40bc6626c2374e453c86a1dcbca3a19c31696623e  ef    [11101111]
```

Walking through adding these entries to a BPT will illustrate the approach.
It is left to the reader to walk through adding these nodes in different
orders to demonstrate how order does not change the end state of the BPT.

In the chart above, we see the hashes of a set of URLs built off of the
RedWagon identity/identifier/domain.  The following steps show adding the
states of these chains to an empty BPT.

We start with the Root entry, an empty Node
```
|L  /R
Root
```
With the ChainID starting with 0x69, the first bit is zero:

```
69
 \L  /R  <-- looking at bit 0
  Root
```
Adding 0xA1:
```
69    a1
 \L  /R  <-- looking at bit 0
  Root
```
Adding 0x2d will push 0x69 up, adding a Node on the Left
```
2d  69
 \L /R       <-- Looking at bit 1
   N     a1
    \L  /R   <-- Looking at bit 0
     Root
```
Adding 0xfb will push 0xA1 up, adding a Node on the right
```
2d  69   a1   fb
 \L /R    \L /R  <-- Looking at bit 1
   N        N
    \L    /R     <-- Looking at bit 0
      Root
```
Finally adding 0xef will push 0xfb up, adding a node on the right
```
            ef    fb
             \L  /R     <-- Looking at bit 2
2d  69   a1    N
 \L /R    \L  /R        <-- Looking at bit 1
   N        N
    \L    /R            <-- Looking at bit 0
      Root
```

Note that even adding just 5 nodes resulted in a reasonably balanced tree.
Tests have demonstrated that we get pretty reasonable results with larger
test sets, even if the trees are rarely absolutely optimal.

## Saving the BPT to disk
Each byte of the ChainID represents up to 511 nodes/values in the MerkleTree
(1+2+4+...+256). The BPT packages each byte into one persisted BPT block.  When
the BPT is updated, modified BPT blocks are persisted to the database.

BPT blocks are indexed in the database under the "BPT" bucket using the
preceding bytes as keys.

### Database

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
* **BPT** *no Salt* -- ChainID / Byte Block -- The nodes in the Binary
  Patricia Tree (BPT) are saved as blocks of BPT nodes.  Each BPT Byte Block
  is addressed by the key up to the block.
   * **Root** *no Salt, no Key* / BPT -- returns the state of the BPT and
     the root node.


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
  salt in use by the MerkleManager
* `func (m MerkleManager) GetElementCount() (elementCount int64)` Returns the
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
  the mark power).  This is key to generating receipts efficiently from a
  stateful merkle tree
* `func (m MerkleManager) AddHash(hash Hash)` Adds a hash to the current
      MerkleTree under management
* `func (m MerkleManager)  AddHashString(hash string)`  Does a conversion of
  a hex string to a binary hash before calling AddHash.  Panics if provided
  a string that is not a valid hex string.

## SMT/merkle.Receipt
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
  a nil receipt is returned.
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
Accumulate is based on authentications. We support a hierarchy of keys that
allows for cold storage and multi-signature security for the control and
management of an identity, and the chains and tokens managed by an identity.
