package database_test

import (
	"testing"

	"golang.org/x/exp/rand"
)

type mapVal map[[32]byte]struct{}
type mapRef map[interface{}]struct{}

const batchSize = 1000

func BenchmarkMap(b *testing.B) {
	// If the key is a value-type, it will not create GC pressure
	b.Run("Value-type key", func(b *testing.B) {
		// Reset by making a new map
		b.Run("Make", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				insertVals(make(mapVal, batchSize))
			}
		})

		// Reset by deleting all the entries in the map
		b.Run("Delete", func(b *testing.B) {
			m := make(mapVal, batchSize)
			for i := 0; i < b.N; i++ {
				insertVals(m)

				// The compiler optimizes this (1.11+)
				for k := range m {
					delete(m, k)
				}
			}
		})
	})

	// If the key is a reference-type, the keys must be GC'd
	b.Run("Reference-type key", func(b *testing.B) {
		// Reset by making a new map
		b.Run("Make", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				insertRefs(mapRef{})
			}
		})

		// Reset by deleting all the entries in the map
		b.Run("Delete", func(b *testing.B) {
			m := mapRef{}
			for i := 0; i < b.N; i++ {
				insertRefs(m)

				// The compiler optimizes this (1.11+)
				for k := range m {
					delete(m, k)
				}
			}
		})
	})

	// How expensive is creating 100 random numbers?
	b.Run("Only rand", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := 0; j < batchSize; j++ {
				randKey()
			}
		}
	})
}

func insertVals(m mapVal) {
	for i := 0; i < batchSize; i++ {
		m[randKey()] = struct{}{}
	}
}

func insertRefs(m mapRef) {
	for i := 0; i < batchSize; i++ {
		m[randKey()] = struct{}{}
	}
}

//go:inline
func randKey() [32]byte {
	var b [32]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(err)
	}
	return b
}
