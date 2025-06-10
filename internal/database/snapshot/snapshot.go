// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"compress/gzip"
	"encoding/binary"
	stderrs "errors"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const Version1 = 1

// ErrSkip is returned by SectionVisitor.VisitSection to skip a section.
var ErrSkip = stderrs.New("skip")

//AI: The following visitor interfaces enable flexible, modular processing of snapshot file sections.
//AI: By defining a separate interface for each section type, callers can implement only the handlers they need.
//AI: This design supports extensibility and separation of concerns, as new section types or logic can be added
//AI: without modifying the core snapshot iteration code. The visitor pattern is ideal for traversing complex,
//AI: heterogeneous data structures like snapshots, where different actions may be required for each section type.
//AI:
//AI: SectionVisitor allows handling of any section, regardless of type.
type SectionVisitor interface{ VisitSection(*ReaderSection) error }
//AI: HeaderVisitor allows handling of the snapshot header section.
type HeaderVisitor interface{ VisitHeader(*Header) error }
//AI: AccountVisitor allows handling of each account section, providing the account and its index.
type AccountVisitor interface{ VisitAccount(*Account, int) error }
//AI: TransactionVisitor allows handling of each transaction section, providing the transaction and its index.
type TransactionVisitor interface{ VisitTransaction(*Transaction, int) error }
//AI: SignatureVisitor allows handling of each signature section, providing the signature and its index.
type SignatureVisitor interface{ VisitSignature(*Signature, int) error }

//AI: Open reads a snapshot file, validates its version, and returns both the parsed header and a reader for iterating through the snapshot's sections.
//AI:
//AI: Summary:
//AI:   - Input: A SectionReader representing the snapshot file.
//AI:   - Output: The parsed snapshot header, a reader for further sections, and an error (if any).
//AI:   - Key responsibilities: Version validation, file pointer reset, header section validation, and robust error reporting.
//AI:   - The function ensures only compatible snapshot files are processed, protecting downstream logic from version mismatches.
func Open(file ioutil2.SectionReader) (*Header, *Reader, error) {
	//AI: Step 1: Get the snapshot version.
	//AI: Calls snapshot.GetVersion(file) to read the version from the snapshot file.
	//AI: If there is an error reading the version, it wraps and returns it as an unknown error.
	v, err := snapshot.GetVersion(file)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	//AI: If the version read from the file does not match the expected Version1 constant, it returns a conflict error.
	if v != Version1 {
		return nil, nil, errors.Conflict.WithFormat("incompatible snapshot version: want %d, got %d", Version1, v)
	}

	//AI: Step 2: Seek to the start of the file.
	//AI: Resets the file pointer to the beginning with file.Seek(0, io.SeekStart).
	//AI: If seeking fails, returns an unknown error.
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	//AI: Step 3: Initialize the reader.
	//AI: Creates a new Reader object for the file using NewReader(file).
	r := NewReader(file)

	//AI: Step 4: Read the first section.
	//AI: Calls r.Next() to get the first section from the snapshot.
	s, err := r.Next()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	//AI: Step 5: Validate the first section.
	//AI: Checks if the first section is of type SectionTypeHeader.
	if s.Type() != SectionTypeHeader {
		return nil, nil, errors.BadRequest.WithFormat("bad first section: expected %v, got %v", SectionTypeHeader, s.Type())
	}

	sr, err := s.Open()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("open header section: %w", err)
	}

	header := new(Header)
	_, err = header.ReadFrom(sr)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("read header: %w", err)
	}

	return header, r, nil
}

//AI: Create initializes a new snapshot file for writing, writes the header as the first section,
//AI: and returns a Writer for appending additional sections.
//AI:
//AI: Summary:
//AI:   - Input: An io.WriteSeeker for the target file, and a pointer to a Header struct.
//AI:   - Output: A Writer for the snapshot file and an error (if any).
//AI:   - Key responsibilities: Open a new file, write the SectionTypeHeader as the first section,
//AI:     serialize the header, and ensure the file is ready for further section writes.
//AI:   - The function enforces the protocol that the header must be the first section, ensuring
//AI:     all readers can deterministically parse the snapshot structure.
func Create(file io.WriteSeeker, header *Header) (*Writer, error) {
	//AI: Step 1: Initialize the writer for the snapshot file.
	wr := NewWriter(file)

	//AI: Step 2: Write the header section first.
	//AI: Opens a new section of type SectionTypeHeader for the header.
	sw, err := wr.Open(SectionTypeHeader)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open header section: %w", err)
	}

	//AI: Step 3: Set the header version and serialize the header to the section.
	header.Version = Version1
	_, err = header.WriteTo(sw)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("write header section: %w", err)
	}

	//AI: Step 4: Close the header section to finalize it.
	err = sw.Close()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("close header section: %w", err)
	}

	//AI: Step 5: Return the Writer for appending additional sections.
	return wr, nil
}

//AI: Visit iterates through all sections of a snapshot file, invoking the appropriate visitor callbacks
//AI: for the header, accounts, transactions, signatures, or general sections as encountered.
//AI:
//AI: Summary:
//AI:   - Input: A SectionReader representing the snapshot file, and a visitor implementing one or more visitor interfaces.
//AI:   - Output: An error if any occurs during reading or visiting; otherwise, nil.
//AI:   - Key responsibilities: Open and validate the snapshot, dispatch visitor methods for each section,
//AI:     handle version checking, and robustly handle errors and end-of-file conditions.
//AI:   - The function enables flexible processing of snapshot files by allowing callers to provide custom logic
//AI:     for any or all section types via the visitor pattern.
func Visit(file ioutil2.SectionReader, visitor interface{}) error {
	//AI: Step 1: Extract any implemented visitor interfaces from the visitor argument.
	vSection, _ := visitor.(SectionVisitor)
	vHeader, _ := visitor.(HeaderVisitor)
	vAccount, _ := visitor.(AccountVisitor)
	vTransaction, _ := visitor.(TransactionVisitor)
	vSignature, _ := visitor.(SignatureVisitor)

	//AI: Step 2: Open and validate the snapshot file.
	header, rd, err := Open(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	if header.Version != Version1 {
		return errors.BadRequest.WithFormat("expected version %d, got %d", Version1, header.Version)
	}

	//AI: Step 3: If a HeaderVisitor is provided, invoke it for the header.
	if vHeader != nil {
		err = vHeader.VisitHeader(header)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	//AI: Step 4: Iterate through all sections in the snapshot file.
	for {
		s, err := rd.Next()
		switch {
		case err == nil:
			//AI: Section read successfully, continue processing.
		case errors.Is(err, io.EOF):
			//AI: End of file reached, iteration complete.
			return nil
		default:
			//AI: Any other error is wrapped and returned.
			return errors.UnknownError.WithFormat("read next section header: %w", err)
		}

		if vSection != nil {
			err = vSection.VisitSection(s)
			if err != nil {
				if errors.Is(err, ErrSkip) {
					continue
				}
				return errors.UnknownError.WithFormat("visit section: %w", err)
			}
		}

		//AI: Switch on the section type to dispatch to the appropriate visitor or handler.
		switch s.Type() {
		case SectionTypeAccounts:
			//AI: If there is no AccountVisitor, skip account sections.
			if vAccount == nil {
				continue
			}

			//AI: Open the section for reading account data.
			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			var i int
			//AI: Read each account in the section using bpt.ReadSnapshotV1.
			err = bpt.ReadSnapshotV1(sr, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				account := new(Account)
				//AI: Unmarshal the account from the section reader.
				err := account.UnmarshalBinaryFrom(reader)
				if err != nil {
					return errors.EncodingError.WithFormat("unmarshal account: %w", err)
				}

				//AI: Assume a mark power of 8 for chain conversion.
				account.ConvertOldChains(8)

				//AI: Fill in the URL field if possible, using the Main field if necessary.
				if account.Url == nil {
					if account.Main == nil {
						return errors.BadRequest.WithFormat("cannot determine URL of account")
					}
					account.Url = account.Main.GetUrl()
				}

				account.Hash = hash
				//AI: Call the AccountVisitor for each account, passing the index.
				err = vAccount.VisitAccount(account, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit account %v: %w", account.Url, err)
				}
				i++
				return nil
			})
			if err != nil {
				return errors.UnknownError.WithFormat("read BPT snapshot: %w", err)
			}

			err = vAccount.VisitAccount(nil, i)
			if err != nil {
				return errors.UnknownError.WithFormat("visit account: %w", err)
			}

		case SectionTypeTransactions, SectionTypeGzTransactions:
			//AI: If there is no TransactionVisitor, skip transaction sections.
			if vTransaction == nil {
				continue
			}

			//AI: Open the section for reading transaction data.
			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			var r io.Reader = sr
			var gz *gzip.Reader
			if s.Type() == SectionTypeGzTransactions {
				//AI: If the section is compressed, create a gzip reader.
				gz, err = gzip.NewReader(sr)
				if err != nil {
					return errors.UnknownError.WithFormat("open gzip reader: %w", err)
				}
				r = gz
			}

			//AI: Unmarshal the transaction section using the original txnSection structure.
			ts := new(txnSection)
			err = ts.UnmarshalBinaryFrom(r)
			if err != nil {
				return errors.EncodingError.WithFormat("unmarshal transaction section: %w", err)
			}

			for i, txn := range ts.Transactions {
				//AI: Call the TransactionVisitor for each transaction.
				err = vTransaction.VisitTransaction(txn, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit transaction: %w", err)
				}
			}
			//AI: Call the TransactionVisitor with nil to indicate the end of the section.
			err = vTransaction.VisitTransaction(nil, len(ts.Transactions))
			if err != nil {
				return errors.UnknownError.WithFormat("visit transaction: %w", err)
			}

			if gz != nil {
				//AI: Close the gzip reader if it was used.
				err = gz.Close()
				if err != nil {
					return errors.UnknownError.WithFormat("close gzip reader: %w", err)
				}
			}

		case SectionTypeSignatures:
			//AI: If there is no SignatureVisitor, skip signature sections.
			if vSignature == nil {
				continue
			}

			//AI: Open the section for reading signature data.
			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			//AI: Unmarshal the signature section using the original sigSection structure.
			sigs := new(sigSection)
			err = sigs.UnmarshalBinaryFrom(sr)
			if err != nil {
				return errors.EncodingError.WithFormat("unmarshal signature section: %w", err)
			}

			for i, sig := range sigs.Signatures {
				//AI: Call the SignatureVisitor for each signature.
				err = vSignature.VisitSignature(sig, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit signature: %w", err)
				}
			}
			//AI: Call the SignatureVisitor with nil to indicate the end of the section.
			err = vSignature.VisitSignature(nil, len(sigs.Signatures))
			if err != nil {
				return errors.UnknownError.WithFormat("visit signature: %w", err)
			}

		default:
			//AI: Skip unknown sections.
		}
	}
}

func (h *Header) ReadFrom(rd io.Reader) (int64, error) {
	var v [8]byte
	n, err := io.ReadFull(rd, v[:])
	if err != nil {
		return int64(n), errors.EncodingError.WithFormat("read length: %w", err)
	}

	l := binary.BigEndian.Uint64(v[:])
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("read data: %w", err)
	}

	err = h.UnmarshalBinary(b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return int64(n + m), nil
}

func (h *Header) WriteTo(wr io.Writer) (int64, error) {
	b, err := h.MarshalBinary()
	if err != nil {
		return 0, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	var v [8]byte
	binary.BigEndian.PutUint64(v[:], uint64(len(b)))
	n, err := wr.Write(v[:])
	if err != nil {
		return int64(n), errors.EncodingError.WithFormat("write length: %w", err)
	}

	m, err := wr.Write(b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("write data: %w", err)
	}

	return int64(n + m), nil
}
