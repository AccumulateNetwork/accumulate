// Copyright 2023 The Accumulate Authors
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

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const Version1 = 1

// ErrSkip is returned by SectionVisitor.VisitSection to skip a section.
var ErrSkip = stderrs.New("skip")

type SectionVisitor interface{ VisitSection(*ReaderSection) error }
type HeaderVisitor interface{ VisitHeader(*Header) error }
type AccountVisitor interface{ VisitAccount(*Account, int) error }
type TransactionVisitor interface{ VisitTransaction(*Transaction, int) error }
type SignatureVisitor interface{ VisitSignature(*Signature, int) error }

// Open reads a snapshot file, returning the header values and a reader.
func Open(file ioutil2.SectionReader) (*Header, *Reader, error) {
	r := NewReader(file)
	s, err := r.Next()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
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

func Create(file io.WriteSeeker, header *Header) (*Writer, error) {
	wr := NewWriter(file)

	// Write the header
	sw, err := wr.Open(SectionTypeHeader)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open header section: %w", err)
	}

	header.Version = Version1
	_, err = header.WriteTo(sw)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("write header section: %w", err)
	}
	err = sw.Close()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("close header section: %w", err)
	}

	return wr, nil
}

func Visit(file ioutil2.SectionReader, visitor interface{}) error {
	vSection, _ := visitor.(SectionVisitor)
	vHeader, _ := visitor.(HeaderVisitor)
	vAccount, _ := visitor.(AccountVisitor)
	vTransaction, _ := visitor.(TransactionVisitor)
	vSignature, _ := visitor.(SignatureVisitor)

	header, rd, err := Open(file)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	if header.Version != Version1 {
		return errors.BadRequest.WithFormat("expected version %d, got %d", Version1, header.Version)
	}

	if vHeader != nil {
		err = vHeader.VisitHeader(header)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	for {
		s, err := rd.Next()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
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

		switch s.typ {
		case SectionTypeAccounts:
			if vAccount == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			var i int
			err = pmt.ReadSnapshot(sr, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
				account := new(Account)
				err := account.UnmarshalBinaryFrom(reader)
				if err != nil {
					return errors.EncodingError.WithFormat("unmarshal account: %w", err)
				}

				// Assume a mark power of 8
				account.ConvertOldChains(8)

				// Fill in the URL field if possible
				if account.Url == nil {
					if account.Main == nil {
						return errors.BadRequest.WithFormat("cannot determine URL of account")
					}
					account.Url = account.Main.GetUrl()
				}

				account.Hash = hash
				err = vAccount.VisitAccount(account, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit account: %w", err)
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
			if vTransaction == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			var r io.Reader = sr
			var gz *gzip.Reader
			if s.typ == SectionTypeGzTransactions {
				gz, err = gzip.NewReader(sr)
				if err != nil {
					return errors.UnknownError.WithFormat("open gzip reader: %w", err)
				}
				r = gz
			}

			s := new(txnSection)
			err = s.UnmarshalBinaryFrom(r)
			if err != nil {
				return errors.EncodingError.WithFormat("unmarshal transaction section: %w", err)
			}

			for i, txn := range s.Transactions {
				err = vTransaction.VisitTransaction(txn, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit transaction: %w", err)
				}
			}

			err = vTransaction.VisitTransaction(nil, len(s.Transactions))
			if err != nil {
				return errors.UnknownError.WithFormat("visit transaction: %w", err)
			}

			if gz != nil {
				err = gz.Close()
				if err != nil {
					return errors.UnknownError.WithFormat("close gzip reader: %w", err)
				}
			}

		case SectionTypeSignatures:
			if vSignature == nil {
				continue
			}

			sr, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open section: %w", err)
			}

			s := new(sigSection)
			err = s.UnmarshalBinaryFrom(sr)
			if err != nil {
				return errors.EncodingError.WithFormat("unmarshal signature section: %w", err)
			}

			for i, sig := range s.Signatures {
				err = vSignature.VisitSignature(sig, i)
				if err != nil {
					return errors.UnknownError.WithFormat("visit signature: %w", err)
				}
			}

			err = vSignature.VisitSignature(nil, len(s.Signatures))
			if err != nil {
				return errors.UnknownError.WithFormat("visit signature: %w", err)
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
