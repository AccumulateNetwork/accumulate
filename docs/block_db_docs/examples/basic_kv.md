# Basic Key-Value Operations

This example demonstrates how to perform basic key-value operations with BlockchainDB.

## Creating a New Database

```go
package main

import (
	"fmt"
	"os"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
)

func main() {
	// Create a directory for the database
	dbDir := "./mydb"
	os.MkdirAll(dbDir, os.ModePerm)

	// Create a new KV store
	// Parameters:
	// - history: enable history tracking (true/false)
	// - directory: path to the database directory
	// - offsetsCnt: number of offset entries (1024 is a good starting point)
	// - keyLimit: key limit before history push
	// - maxCachedBlocks: maximum number of blocks to cache
	kv, err := blockchainDB.NewKV(
		true,  // Enable history
		dbDir, // Database directory
		1024,  // Offset count
		10000, // Key limit
		100,   // Max cached blocks
	)
	if err != nil {
		fmt.Printf("Error creating database: %v\n", err)
		return
	}
	defer kv.Close()

	fmt.Println("Database created successfully")

	// Now we can use the database for key-value operations
	performOperations(kv)
}

func performOperations(kv *blockchainDB.KV) {
	// Create some keys
	key1 := createKey("key1")
	key2 := createKey("key2")

	// Store values
	err := kv.Put(key1, []byte("Hello, BlockchainDB!"))
	if err != nil {
		fmt.Printf("Error storing value: %v\n", err)
		return
	}

	err = kv.Put(key2, []byte("This is another value"))
	if err != nil {
		fmt.Printf("Error storing value: %v\n", err)
		return
	}

	fmt.Println("Values stored successfully")

	// Retrieve values
	value1, err := kv.Get(key1)
	if err != nil {
		fmt.Printf("Error retrieving value: %v\n", err)
		return
	}
	fmt.Printf("Value for key1: %s\n", value1)

	value2, err := kv.Get(key2)
	if err != nil {
		fmt.Printf("Error retrieving value: %v\n", err)
		return
	}
	fmt.Printf("Value for key2: %s\n", value2)

	// Update a value
	err = kv.Put(key1, []byte("Updated value"))
	if err != nil {
		fmt.Printf("Error updating value: %v\n", err)
		return
	}

	// Retrieve the updated value
	updatedValue, err := kv.Get(key1)
	if err != nil {
		fmt.Printf("Error retrieving updated value: %v\n", err)
		return
	}
	fmt.Printf("Updated value for key1: %s\n", updatedValue)
}

// Helper function to create a 32-byte key from a string
func createKey(input string) [32]byte {
	var key [32]byte
	copy(key[:], input)
	return key
}
```

## Opening an Existing Database

```go
package main

import (
	"fmt"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
)

func main() {
	// Open an existing KV store
	dbDir := "./mydb"
	kv, err := blockchainDB.OpenKV(dbDir)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	defer kv.Close()

	fmt.Println("Database opened successfully")

	// Retrieve a value
	key := createKey("key1")
	value, err := kv.Get(key)
	if err != nil {
		fmt.Printf("Error retrieving value: %v\n", err)
		return
	}
	fmt.Printf("Value for key1: %s\n", value)
}

// Helper function to create a 32-byte key from a string
func createKey(input string) [32]byte {
	var key [32]byte
	copy(key[:], input)
	return key
}
```

## Working with Binary Data

BlockchainDB works well with binary data, which is common in blockchain applications:

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
)

func main() {
	// Create a directory for the database
	dbDir := "./bindb"
	os.MkdirAll(dbDir, os.ModePerm)

	// Create a new KV store
	kv, err := blockchainDB.NewKV(
		false, // Disable history for this example
		dbDir, // Database directory
		1024,  // Offset count
		10000, // Key limit
		100,   // Max cached blocks
	)
	if err != nil {
		fmt.Printf("Error creating database: %v\n", err)
		return
	}
	defer kv.Close()

	// Create a key using SHA-256 hash
	data := []byte("some transaction data")
	hash := sha256.Sum256(data)
	
	// Store binary data
	binaryData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	err = kv.Put(hash, binaryData)
	if err != nil {
		fmt.Printf("Error storing binary data: %v\n", err)
		return
	}

	fmt.Printf("Stored binary data with key: %s\n", hex.EncodeToString(hash[:]))

	// Retrieve binary data
	retrievedData, err := kv.Get(hash)
	if err != nil {
		fmt.Printf("Error retrieving binary data: %v\n", err)
		return
	}

	fmt.Printf("Retrieved binary data: %v\n", retrievedData)
	fmt.Printf("Hex representation: %s\n", hex.EncodeToString(retrievedData))
}
```

## Error Handling

Proper error handling is important when working with BlockchainDB:

```go
package main

import (
	"fmt"
	"os"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
)

func main() {
	// Create a directory for the database
	dbDir := "./errdb"
	os.MkdirAll(dbDir, os.ModePerm)

	// Create a new KV store
	kv, err := blockchainDB.NewKV(
		false, // Disable history
		dbDir, // Database directory
		1024,  // Offset count
		10000, // Key limit
		100,   // Max cached blocks
	)
	if err != nil {
		fmt.Printf("Error creating database: %v\n", err)
		return
	}
	defer kv.Close()

	// Create a key
	var key [32]byte
	copy(key[:], "nonexistent-key")

	// Try to get a non-existent key
	value, err := kv.Get(key)
	if err != nil {
		fmt.Printf("Expected error when retrieving non-existent key: %v\n", err)
		// Handle the error appropriately
	} else {
		fmt.Printf("Retrieved value: %s\n", value)
	}

	// Store a value for the key
	err = kv.Put(key, []byte("Now the key exists"))
	if err != nil {
		fmt.Printf("Error storing value: %v\n", err)
		return
	}

	// Now the key should exist
	value, err = kv.Get(key)
	if err != nil {
		fmt.Printf("Unexpected error: %v\n", err)
	} else {
		fmt.Printf("Retrieved value: %s\n", value)
	}
}
```
