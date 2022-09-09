package snapshot

import (
	"compress/gzip"
	"encoding/binary"
	stderrs "errors"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

const Version1 = 1

// ErrSkip is returned by SectionVisitor.VisitSection to skip a section.
var ErrSkip = stderrs.New("skip")

type SectionVisitor interface{ VisitSection(*ReaderSection) error }
type AccountVisitor interface{ VisitAccount(*Account, int) error }
type TransactionVisitor interface{ VisitTransaction(*Transaction, int) error }
type SignatureVisitor interface{ VisitSignature(*Signature, int) error }

// Open reads a snapshot file, returning the header values and a reader.
func Open(file ioutil2.SectionReader) (*Header, *Reader, error) {
	r := NewReader(file)
	s, err := r.Next()
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	if s.Type() != SectionTypeHeader {
		return nil, nil, errors.Format(errors.StatusBadRequest, "bad first section: expected %v, got %v", SectionTypeHeader, s.Type())
	}

	sr, err := s.Open()
	if err != nil {
		return nil, nil, errors.Format(errors.StatusUnknownError, "open header section: %w", err)
	}

	header := new(Header)
	_, err = header.ReadFrom(sr)
	if err != nil {
		return nil, nil, errors.Format(errors.StatusUnknownError, "read header: %w", err)
	}

	return header, r, nil
}

func Create(file io.WriteSeeker, header *Header) (*Writer, error) {
	wr := NewWriter(file)

	// Write the header
	sw, err := wr.Open(SectionTypeHeader)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "open header section: %w", err)
	}

	header.Version = Version1
	_, err = header.WriteTo(sw)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "write header section: %w", err)
	}
	err = sw.Close()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "close header section: %w", err)
	}

	return wr, nil
}

func Visit(file ioutil2.SectionReader, visitor interface{}) error {
	vSection, _ := visitor.(SectionVisitor)
	vAccount, _ := visitor.(AccountVisitor)
	vTransaction, _ := visitor.(TransactionVisitor)
	vSignature, _ := visitor.(SignatureVisitor)

	header, rd, err := Open(file)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "open: %w", err)
	}
	if header.Version != Version1 {
		return errors.Format(errors.StatusBadRequest, "expected version %d, got %d", Version1, header.Version)
	}

	for {
		s, err := rd.Next()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
			return errors.Format(errors.StatusUnknownError, "read next section header: %w", err)
		}

		if vSection != nil {
			err = vSection.VisitSection(s)
			if err != nil {
				if errors.Is(err, ErrSkip) {
					continue
				}
				return errors.Format(errors.StatusUnknownError, "visit section: %w", err)
			}
		}

		switch s.typ {
		case SectionTypeAccounts:
			if vAccount == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "open section: %w", err)
			}

			var i int
			err = pmt.ReadSnapshot(sr, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				account := new(Account)
				err := account.UnmarshalBinaryFrom(reader)
				if err != nil {
					return errors.Format(errors.StatusEncodingError, "unmarshal account: %w", err)
				}

				account.Hash = hash
				err = vAccount.VisitAccount(account, i)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "visit account: %w", err)
				}
				i++
				return nil
			})
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "read BPT snapshot: %w", err)
			}

			err = vAccount.VisitAccount(nil, i)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "visit account: %w", err)
			}

		case SectionTypeTransactions, SectionTypeGzTransactions:
			if vTransaction == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "open section: %w", err)
			}

			var r io.Reader = sr
			var gz *gzip.Reader
			if s.typ == SectionTypeGzTransactions {
				gz, err = gzip.NewReader(sr)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "open gzip reader: %w", err)
				}
				r = gz
			}

			s := new(txnSection)
			err = s.UnmarshalBinaryFrom(r)
			if err != nil {
				return errors.Format(errors.StatusEncodingError, "unmarshal transaction section: %w", err)
			}

			for i, txn := range s.Transactions {
				err = vTransaction.VisitTransaction(txn, i)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "visit transaction: %w", err)
				}
			}

			err = vTransaction.VisitTransaction(nil, len(s.Transactions))
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "visit transaction: %w", err)
			}

			if gz != nil {
				err = gz.Close()
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "close gzip reader: %w", err)
				}
			}

		case SectionTypeSignatures:
			if vSignature == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "open section: %w", err)
			}

			s := new(sigSection)
			err = s.UnmarshalBinaryFrom(sr)
			if err != nil {
				return errors.Format(errors.StatusEncodingError, "unmarshal signature section: %w", err)
			}

			for i, sig := range s.Signatures {
				err = vSignature.VisitSignature(sig, i)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "visit signature: %w", err)
				}
			}

			err = vSignature.VisitSignature(nil, len(s.Signatures))
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "visit signature: %w", err)
			}

		default:
			// Skip unknown sections
		}
	}
}

func (h *Header) ReadFrom(rd io.Reader) (int64, error) {
	var v [8]byte
	n, err := io.ReadFull(rd, v[:])
	if err != nil {
		return int64(n), errors.Format(errors.StatusEncodingError, "read length: %w", err)
	}

	l := binary.BigEndian.Uint64(v[:])
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "read data: %w", err)
	}

	err = h.UnmarshalBinary(b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "unmarshal: %w", err)
	}

	return int64(n + m), nil
}

func (h *Header) WriteTo(wr io.Writer) (int64, error) {
	b, err := h.MarshalBinary()
	if err != nil {
		return 0, errors.Format(errors.StatusEncodingError, "marshal: %w", err)
	}

	var v [8]byte
	binary.BigEndian.PutUint64(v[:], uint64(len(b)))
	n, err := wr.Write(v[:])
	if err != nil {
		return int64(n), errors.Format(errors.StatusEncodingError, "write length: %w", err)
	}

	m, err := wr.Write(b)
	if err != nil {
		return int64(n + m), errors.Format(errors.StatusEncodingError, "write data: %w", err)
	}

	return int64(n + m), nil
}
