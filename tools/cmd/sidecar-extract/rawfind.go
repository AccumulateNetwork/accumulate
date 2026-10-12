// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// rawFind searches log files byte by byte for the listed keys, with no regard
// for entry structure, and reports each occurrence and whether an intact entry
// starts where the key's header would be.
func rawFind(keysPath string, files []string) error {
	keys := map[uint64][]byte{}
	f, err := os.Open(keysPath)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, err := hex.DecodeString(strings.Fields(sc.Text())[0])
		if err == nil && len(k) == 32 {
			keys[binary.BigEndian.Uint64(k[:8])] = k
		}
	}
	f.Close()
	fmt.Printf("%d keys\n", len(keys))
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var n, intact int
		for i := 0; i+32 <= len(b); i++ {
			k, ok := keys[binary.BigEndian.Uint64(b[i:])]
			if !ok || string(b[i:i+32]) != string(k) {
				continue
			}
			n++
			// A move entry's key is the move prefix, the key, the version
			for _, start := range []int{i - headerSize, i - headerSize - len(badgerMove)} {
				if start >= 0 {
					if _, _, ok := decodeEntry(b[start:]); ok {
						intact++
					}
				}
			}
			if n <= 5 {
				fmt.Printf("  %s offset %d key %x\n", path, i, k[:6])
			}
		}
		fmt.Printf("%s: %d occurrences, %d in intact entries\n", path, n, intact)
	}
	return nil
}
