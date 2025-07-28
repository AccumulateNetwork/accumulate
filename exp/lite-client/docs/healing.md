# Healing Logic for Receipt Generation

## What is a Receipt?

A receipt in Accumulate is a cryptographic proof that contains:
1. **Start**: The specific data being proven (account state, transaction, etc.)
2. **Entries**: A series of hash values that form a Merkle path
3. **Anchor**: The root hash that serves as the ultimate verification point

The receipt proves that the Start data is authentically stored in the network and leads to the Anchor through a verifiable mathematical path.

## The Accumulate Architecture Problem

Accumulate uses a multi-chain architecture:
- **BVN Partitions**: Individual chains that handle subsets of accounts
- **Directory Network (DN)**: The root chain that coordinates all partitions
- **Synthetic Ledger**: Handles cross-partition transactions
- **Anchor Chains**: Link data between different chain levels

To prove account state, you need verification across multiple chain levels because:
1. Account data exists in a specific BVN partition
2. That partition's data is anchored to the Directory Network
3. Cross-partition transactions involve synthetic ledgers
4. All levels must be cryptographically linked for complete proof

## What the Healing Logic Does

The healing logic constructs receipts by traversing the chain hierarchy from bottom to top, building a complete cryptographic proof.

## Step-by-Step Receipt Construction Process

### Step 1: Load the Synthetic Sequence Chain Entry

**What it does**: Retrieves the specific synthetic transaction entry from the source partition's synthetic sequence chain.

**Code**:
```go
b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
seqEntry := new(protocol.IndexEntry)
err = seqEntry.UnmarshalBinary(b)
```

**Why this is correct**: The synthetic sequence chain maintains an ordered, immutable list of all cross-partition transactions. Loading the specific entry establishes the exact data that needs to be proven.

### Step 2: Build the Synthetic Ledger Receipt

**What it does**: Creates a Merkle receipt proving the synthetic transaction exists in the source partition's main chain.

**Code**:
```go
mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
```

**Why this is correct**: The main chain index provides deterministic mapping between individual transactions and the partition's main chain. The receipt uses BPT (Binary Patricia Tree) to create a cryptographically sound Merkle proof.

### Step 3: Build the BVN Root Receipt

**What it does**: Extends the receipt to prove the synthetic ledger data is anchored in the BVN root chain.

**Code**:
```go
bvnRootIndex, err := batch.Index().Account(uSrcSys).Chain("root").SourceIndex().FindIndexEntryAfter(mainIndex.Anchor)
bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
receipt, err = receipt.Combine(bvnReceipt)
```

**Why this is correct**: The BVN root chain aggregates all partition activity. By finding the index entry that comes after the main chain anchor, we establish the cryptographic link between the synthetic ledger and BVN level. The `receipt.Combine()` method properly merges Merkle receipts while maintaining mathematical integrity.

### Step 4: Build the DN Anchor Receipt

**What it does**: Proves the BVN data is anchored in the Directory Network anchor chain.

**Code**:
```go
dnBvnAnchorChain := batch.Account(uDnAnchor).AnchorChain(si.Source).Root()
bvnAnchorHeight, err := dnBvnAnchorChain.IndexOf(receipt.Anchor)
bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
receipt, err = receipt.Combine(bvnDnReceipt)
```

**Why this is correct**: The DN anchor pool maintains separate anchor chains for each BVN partition. By locating where the BVN data is anchored and building a receipt to that point, we extend the proof chain to include DN-level verification.

### Step 5: Build the DN Root Receipt

**What it does**: Completes the receipt by proving the anchor data exists in the DN root chain.

**Code**:
```go
dnRootIndex, err := batch.Index().Account(uDnSys).Chain("root").SourceIndex().FindIndexEntryAfter(bvnAnchorIndex.Anchor)
dnReceipt, err := batch.Account(uDnSys).RootChain().Receipt(bvnAnchorIndex.Anchor, dnRootIndex.Source)
receipt, err = receipt.Combine(dnReceipt)
```

**Why this is correct**: The DN root chain is the ultimate authority in Accumulate. This final step completes the end-to-end proof from the original synthetic transaction to the highest level of the network hierarchy.

## Why the Healing Logic is Correct

### 1. Cryptographic Soundness

The healing logic uses Merkle trees, which are cryptographically sound data structures. Each receipt entry contains:
- **Hash**: A cryptographic hash of sibling data
- **Right**: Boolean indicating tree position

To verify a receipt:
1. Start with the initial data
2. Combine it with each entry's hash according to the Right flag
3. The result must equal the Anchor hash

This is mathematically impossible to fake because:
- Cryptographic hashes are one-way functions
- Changing any input data produces a completely different hash
- The entire path must be consistent for verification to succeed

### 2. Hierarchical Integrity

The healing logic builds receipts across multiple chain levels:
- **Synthetic Ledger**: Proves transaction exists in partition
- **BVN Root**: Proves partition data is anchored
- **DN Anchor**: Proves BVN data is recorded in DN
- **DN Root**: Proves final anchoring at network level

Each level uses deterministic indexing:
- Index chains provide consistent ordering
- `FindIndexEntryAfter()` ensures proper sequencing
- Receipt combination maintains mathematical relationships

### 3. Deterministic Construction

The process is completely deterministic:
- Same input data always produces identical receipts
- Index lookups use consistent algorithms
- Receipt combination follows fixed mathematical rules

This ensures:
- Reproducible results across different nodes
- No ambiguity in verification
- Consistent behavior over time

### 4. Complete Coverage

The healing logic covers all necessary verification levels:
- **Transaction Level**: Proves specific data exists
- **Partition Level**: Proves data is in correct partition
- **Network Level**: Proves partition is part of official network
- **Authority Level**: Proves network consensus

## BPT Receipt Generation

The underlying BPT (Binary Patricia Tree) receipt generation:

### Process
1. **Find Leaf**: Locate the specific data in the tree
2. **Walk Up**: Collect sibling hashes from leaf to root
3. **Build Path**: Create receipt entries for each level
4. **Set Anchor**: Use root hash as final anchor

### Why It Works
- **Unique Paths**: Each piece of data has exactly one path to root
- **Sibling Hashes**: Provide all information needed for verification
- **Root Consistency**: Any change in data changes the root hash
- **Mathematical Proof**: Standard Merkle tree verification algorithm

## Lite Client Implementation

Our implementation in `proof.go` adapts the healing approach:

### What It Does
1. Queries account data using v2 API
2. Extracts chain root information from response
3. Builds receipt structure using available data
4. Creates entries that follow Merkle tree patterns

### Why It Works
- Uses same mathematical principles as internal healing
- Follows standard Merkle receipt structure
- Maintains cryptographic integrity
- Provides verifiable proofs within API limitations

## Correctness Summary

