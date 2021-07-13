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
package storage

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/user"
	"time"
)

// GetHomeDir
// Used to find the Home Directory from which the configuration directory for the ValAcc application to
// use for its database.  This is not a terribly refined way of configuring the ValAcc and may be
// refined in the future.
func GetHomeDir() string {
	anchorPlatformHome := os.Getenv("ANCHOR_PLATFORM")
	if anchorPlatformHome != "" {
		return anchorPlatformHome
	}

	// Get the OS specific home directory via the Go standard lib.
	var homeDir string
	usr, err := user.Current()
	if err == nil {
		homeDir = usr.HomeDir
	}

	// Fall back to standard HOME environment variable that works
	// for most POSIX OSes if the directory from the Go standard
	// lib failed.
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}
	return homeDir
}

// s2a
// Slice to 32 byte Array
func s2a(s []byte) (a [32]byte) {
	copy(a[:], s)
	return a
}

// BoolBytes
// Marshal a Bool
func BoolBytes(b bool) []byte {
	if b {
		return append([]byte{}, 1)
	}
	return append([]byte{}, 0)
}

// BytesBool
// Unmarshal a Uint8
func BytesBool(data []byte) (f bool, newData []byte) {
	if data[0] != 0 {
		f = true
	}
	return f, data[1:]
}

// Uint16Bytes
// Marshal a int32 (big endian)
func Uint16Bytes(i uint16) []byte {
	return append([]byte{}, byte(i>>8), byte(i))
}

// BytesUint16
// Unmarshal a uint32 (big endian)
func BytesUint16(data []byte) (uint16, []byte) {
	return uint16(data[0])<<8 + uint16(data[1]), data[2:]
}

// Uint32Bytes
// Marshal a int32 (big endian)
func Uint32Bytes(i uint32) []byte {
	return append([]byte{}, byte(i>>24), byte(i>>16), byte(i>>8), byte(i))
}

// BytesUint32
// Unmarshal a uint32 (big endian)
func BytesUint32(data []byte) (uint32, []byte) {
	return uint32(data[0])<<24 + uint32(data[1])<<16 + uint32(data[2])<<8 + uint32(data[3]), data[4:]
}

// Uint64Bytes
// Marshal a uint64 (big endian)
func Uint64Bytes(i uint64) (data []byte) {
	var buf [16]byte
	count := binary.PutUvarint(buf[:], i)
	return buf[count:]
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
	value, cnt := binary.Varint(data)
	return value, data[cnt:]
}

// DurationFormat
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
