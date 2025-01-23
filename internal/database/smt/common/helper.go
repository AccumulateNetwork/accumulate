// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// For key value stores where buckets are not supported, we add a byte to the
// key to represent a bucket. For now, all buckets are hard coded, but we could
// change that in the future.
//
// Buckets are not really enough to index everything we wish to index.  So
// we have labels as well.  Labels are shifted 8 bits left, so they can be
// combined with the buckets to create a unique key.
//
// This allows us to put the raw directory block at DBlockBucket+L_raw, and meta data
// about the directory block at DBlockBucket+MetaLabel
package common

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Uint64FixedBytes
// Return []byte, big endian, of len 8 for the uint64
// (Note casting an Int64 to uInt64 will give the same 8 bytes)
func Uint64FixedBytes(i uint64) (data []byte) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], i)
	return buf[:]
}

// BytesFixedUint64
// Return a Uint64, for an 8 byte value,big endian
// (Note casting to an Int64 will work fine)
func BytesFixedUint64(data []byte) (i uint64, nextData []byte) {
	i = binary.BigEndian.Uint64(data)
	return i, data[8:]
}

// Uint64Bytes
// Marshal a uint64 (big endian)
func Uint64Bytes(i uint64) (data []byte) {
	var buf [16]byte
	count := binary.PutUvarint(buf[:], i)
	return buf[:count]
}

// BytesUint64
// Unmarshal a uint64 (big endian)
func BytesUint64(data []byte) (uint64, []byte) {
	value, count := binary.Uvarint(data)
	return value, data[count:]
}

// Int64Bytes
// Marshal a int64 (big endian)
// We only need this function on top of Uint64Bytes to avoid a type conversion when dealing with int64 values
func Int64Bytes(i int64) []byte {
	var buf [16]byte
	// Note that shifting i right 56 bits DOES fill the in64 with the sign bit, but the byte conversion kills that.
	count := binary.PutVarint(buf[:], i)
	return buf[:count]
}

// BytesInt64
// Unmarshal a int64 (big endian)
// We only need this function on top of BytesUint64 to avoid a type conversion when dealing with int64 values
func BytesInt64(data []byte) (int64, []byte) {
	value, count := binary.Varint(data)
	return value, data[count:]
}

// FormatTimeLapse
// Simple formatting for duration time.  Prints all results within a fixed field of text.
func FormatTimeLapse(d time.Duration) string {
	return FormatTimeLapseSeconds(int64(d.Seconds()))
}

// DurationFormat
// Simple formatting if what I have is seconds. Prints all results within a fixed field of text.
func FormatTimeLapseSeconds(total int64) string {
	days := total / 24 / 60 / 60
	total -= days * 24 * 60 * 60
	hours := total / 60 / 60
	total -= hours * 60 * 60
	minutes := total / 60
	seconds := total - minutes*60
	if days > 0 {
		return fmt.Sprintf("%3d/%02d:%02d:%02d d/h:m:s", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("    %02d:%02d:%02d h:m:s  ", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("       %02d:%02d m:s    ", minutes, seconds)
	} else {
		return fmt.Sprintf("          %2d s      ", seconds)
	}
}

// SliceBytes
// Append a Uvarint length infront of a slice, effectively converting a slice to a counted string
func SliceBytes(slice []byte) []byte {
	var varInt [16]byte                                              // Buffer to hold a Uvarint
	countOfBytes := binary.PutUvarint(varInt[:], uint64(len(slice))) // calculate the Uvarint of the len of the slice
	counted := append(varInt[:countOfBytes], slice...)               // Now put the Uvarint right in front of the slice
	return counted                                                   // Return the resulting counted string
}

// BytesSlice
// Convert a counted byte array (which is a count followed by the byte values) to a slice.  We return what is
// left of the data once the counted byte array is removed
func BytesSlice(data []byte) (slice []byte, data2 []byte) {
	countOfBytes, count := binary.Uvarint(data) // Get the number of bytes in the slice, and count of bytes used for the count
	data = data[count:]
	slice = append(slice, data[:countOfBytes]...)
	data = data[countOfBytes:]
	return slice, data
}

func hexToBytes(hexStr string) []byte {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return raw
}
