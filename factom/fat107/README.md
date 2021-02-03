# Factom Data Store

A robust protocol for storing data of nearly arbitrary size on the Factom
Blockchain.

## Features
- Secure - ChainID uniquely identifies the exact data within an optional
  application defined Namespace.
- Extensible - Applications may define any arbitrary metadata to attach to the
  data store.
- DOS proof - The data blocks may appear in any order, on any chain, and may be
  interspersed with unrelated or garbage entries.
- Efficient - Clients only need to download the first modestly sized entry in
  the chain to discover the total data size, and exact number of entries that
they will need to download. All required Data Block Entries can be discovered
and then downloaded concurrently.
- Arbitrary size - There is no theoretical limit to the size of data that may
  be stored. However most clients will likely enforce sane practical limits
before downloading a file.
- Censorship resistent - Commit all entries before revealing any to ensure that
  a data store cannot be censored.
- Optional compression - Use gzip, zlib, or none.

## Specification

#### Abstract

The Factom Blockchain has a maximum Entry size of 10240 bytes. Applications
that wish to store contiguous data that exceed this limit must break up the
data across many Entries. This standard defines a generic Data Store Chain
protocol for storing data of arbitrary size, while allowing efficient lookup of
the data by Chain ID, which can be derived using the data's hash and an
optional application defined Namespace data.

#### Summary

Raw Data of arbitrary size is broken up into Data Blocks and stored in Data
Block Entries. The Data Block Entry Hashes are recorded in their proper order
in the Data Block Index. The Data Block Index (DBI) is stored in a linked list
of DBI Entries. The First Entry of a Data Store Chain records the first DBI
Entry Hash of the linked list, along with size, compression, and any
application defined Metadata. The Chain ID of the Data Store Chain is derived
from the sha256d data hash, along with any optional application defined
namespace data. Thus the Chain ID uniquely identifies a piece of data within an
application defined Namedspace.


### First Chain Entry

The First Chain Entry establishes the sha256d hash of the data, its size,
compression details, the first Data Block Index Entry Hash, and any optional
application defined Namespace data or Metadata.


#### NameIDs/ExtIDs

The sha256d hash and the Namespace data are used to compute the Chain ID. This
allows for applications to lookup or deduplicate data within their own
Namespace by Chain ID.

##### Namespace

A Data Store may be defined within an optional application defined Namespace.
The Namespace is an arbitrary set of ExtIDs that are appended to the NameIDs of
a Data Store Chain. Thus the Namespace must be known by a client in order to
compute the Chain ID for a Data Store. For this reason it is not recommended to
use the Namespace to describe the data. Use the Application Metadata which is
stored in the Content of the First Entry instead for metadata that describes
the data.

| i | Type | Description |
|- |- |- |
| 0 | string | "data-store",  A human readable marker that defines this data protocol. |
| 1 | Bytes32 | The sha256d hash of the data. |
| ... | (any) | Optional application defined Namespace IDs... |

#### Content

The Content is a Metadata JSON Object which includes the information required
to reconstruct the data along with any application defined metadata. This
includes the first DBI Entry, data size, and any optional compression details.

Note: Normally DBI Entries and Data Blocks will exist on the same Data Store
Chain that references them, but this is not a requirement. The First Entry may
reference any valid DBI Entry on any Chain. This allows Data Stores to be
created that reference pre-existing DBI Entries from a different Namespace.

##### Metadata Object
| Name | Type| Description |
|-|-|-|
| "data-store" | string | Protocol version, currently "1.0" |
| "size" | uint64 | Total data size |
| "dbi-start" | Bytes32 | The hash of the first DBI Entry as a hex string |
| "compression" | Compression Object | Optional compression details, omit if no compression is used |
| "metadata"  | (any)     | Optional application defined Metadata |

##### Compression Object

Data may optionally be compressed before it is stored on chain. Currently this
standard defines the use of the following compression formats: zlib, gzip.
Compression details are stored in a JSON Compression Object with the following
fields.

| Name | Type| Description |
|-|-|-|
| "format" | string | "zlib" or "gzip" |
| "size" | uint64 | Total compressed data size |

### Data Block Index Entry

A Data Block Index Entry contains all or part of the Data Block Index (DBI)
which defines all the Data Block Entry Hashes in the proper order to
reconstruct the data, or the compressed data.

If the DBI does not fit into a single Entry, the DBI is split into the shortest
possible linked list of DBI Entries. The linked list is formed by the first
ExtID referencing the Hash of the next DBI Entry, if it exists.

#### ExtIDs
| i | Type | Description |
|-|-|-|
| 0 | Bytes32 | The Hash of the next DBI Entry, if it exists. |

#### Content

The Data Block Index is a binary data structure which is simply the ordered
concatenation of all raw 32 byte Data Block Entry Hashes. The Content of a DBI
Entry is as many complete Data Block Entry Hashes that may fit in the remaining
space of the Entry. Only the final DBI Entry in a linked list may be not full.

An Entry contains 10240 bytes. Thus a maximum of `10240/32 = 320` Entry Hashes
may fit in a single DBI entry. However, if more than 320 hashes are required,
an ExtID with the next DBI Entry Hash must be included, which is `2+32 = 34`
bytes. This means that only `10240-34 = 10206` bytes remain, which only leaves
space for `10206/32 = 318` DBI Entry Hashes.

Note: While it would be possible to parse a DBI Entry Linked List with DBI
Entries that did not use all of their available space, this opens up the
possibility for the creation of Data Stores that cause a client to download
more Entries than strictly necessary. Such Data Stores should be considered
invalid.

### Data Block Entry

A Data Block Entry contains a piece of the Data referenced by the DBI. The Data
is split into as few Data Block Entries as possible. Thus only the last Data
Block in a DBI may be not full.

Note: While it would be possible to parse a set of Data Blocks that did not use
all of their available space, this opens up the possibility for the creation of
Data Stores that cause a client to download more Entries than strictly
necessary. Such Data Stores should be considered invalid.

#### ExtIDs

A Data Block Entry may not have any ExtIDs.

#### Content

A block of the raw data. The Content must include as much raw data as possible.
Thus only the last Data Block in a DBI may be not full.

### Writing a data store

1. Compute the Chain ID
- Compute the hash of the data. `sha256d(data)`
- Construct the ExtIDs of the First Entry with any application defined
  Namespace data.
- Compute the ChainID
  `sha256(sha256(ExtID[0])|sha256(ExtID[1])|...|sha256(ExtID[n]))`

2. Build the Data Block Entries
- Optionally compress the data using zlib or gzip.
- Construct `len(compressData)/10240` Entries (`+1` if
  `len(compressedData)%10240 > 0`).
- Sequentially fill the Content of the Entries as much of the data as possible,
  populate the ChainID, and compute the Entry Hashes.

3. Build the Data Block Index Entries
- Construct Data Block Index Entries as follows until no more Data Block Hashes
  remain.
        a. If the number of remaining Data Block Entry Hashes are less than or
equal to 320, put all remaining hashes in this DBI Entry.
        b. Else, put 318 hashes in the content, and create another DBI Entry.
- Compute the DBI Entry Hashes from last to first, placing the Entry Hash of
  the "next" (logical order, not iteration order) DBI Entry in the first ExtID
of each "preceding" DBI Entry.

4. Construct the Data Store Chain First Entry
- Set the NameIDs
- Construct the Content Metadata JSON Object with data size, any compression
  details, and any application defined metadata.

5. Publish the data store
- Commit the first entry, and then commit all DBI Entries and Data Block
  entries.
- Wait for ACK for all Commits.
- Reveal all Entries.

### Reading a data store

1. Compute the Chain ID, if not already known, using the data hash and optional
   application Namespace data.
2. Download and validate the First Entry.
- Validate ExtID structure.
- Validate JSON content structure.
- Confirm Data Store version.
- Confirm that the declared file size, and compressed file size if compressed,
  are sane values.
- Validate any application Metadata.
3. Download the DBI.
- Download all DBI Entries by traversing the linked list. Ensure that only the
  last DBI Entry has fewer than 318 Data Block Entry Hashes.
4. Download the Data Blocks and reconstruct the, possibly compressed, data.
- Data Blocks may be downloaded concurrently since they are all known from the
  DBI.
- Order the data according to the DBI.
- Decompress the data, if compressed.
- Verify the sha256d data hash.
