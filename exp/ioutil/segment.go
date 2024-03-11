// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// # Segmented files
//
// [SegmentedReader] and [SegmentedWriter] define a file format that allows a
// single file to be partitioned into multiple independent sections. Each
// section has a header that specifies the size and type of the section and the
// offset of the next section. Thus the sections form a kind of linked list
// within the file.
//
// Each section consists of a 64 byte header followed by data. The data can have
// any length, but the space on disk (in the file) will be padded to the nearest
// multiple of 64 to ensure the headers are aligned on 64-byte boundaries. When
// a section is opened for writing, if it is not the first section, its offset
// is recorded in the previous section's header. When a section writer is
// closed, the length of data written is recorded in the section's header and
// the file's offset is advanced to the next 64-byte boundary.
//
// Section headers are structured as follows:
//
//   - Type - 2 bytes
//   - (reserved) - 6 bytes
//   - Size - 8 bytes
//   - Next section offset - 8 bytes
//   - (reserved) - 40 bytes
package ioutil
