# Bloom Filter

The Bloom Filter component in BlockchainDB provides a space-efficient probabilistic data structure used to test whether an element is a member of a set.

## Overview

Bloom filters are used in BlockchainDB to quickly determine if a key might exist in a dataset without having to perform expensive disk operations. They provide a fast way to check if a key is definitely not in the database, which can significantly improve performance for lookups of non-existent keys.

## Structure

```go
type BloomFilter struct {
    Filter []byte
    K      int
}
```

## Key Features

- Space-efficient probabilistic data structure
- Fast membership testing with no false negatives (but possible false positives)
- Configurable false positive rate
- Useful for quickly filtering out non-existent keys

## Basic Operations

### Creating a New Bloom Filter

```go
// Create a new Bloom Filter
filter := blockchainDB.NewBloomFilter(1024, 3) // size and number of hash functions
```

### Adding an Element

```go
// Add an element to the filter
filter.Add([]byte("key data"))
```

### Testing for Membership

```go
// Test if an element might be in the set
exists := filter.Test([]byte("key data"))
```

## Implementation Details

- Uses multiple hash functions to set bits in the filter
- The number of hash functions (K) affects the false positive rate
- The size of the filter affects both the false positive rate and memory usage
- Returns false positives sometimes, but never false negatives

## Performance Considerations

- Bloom filters are extremely space-efficient compared to storing the actual keys
- Lookup operations are constant time, regardless of the number of elements
- The false positive rate can be tuned by adjusting the filter size and number of hash functions
- Particularly useful in scenarios where disk I/O is expensive and a quick negative result is valuable
