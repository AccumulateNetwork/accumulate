// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/block"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/sidecar"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (s *SidecarService) Requires() []ioc.Requirement { return nil }
func (s *SidecarService) Provides() []ioc.Provided    { return nil }

func (s *SidecarService) start(inst *Instance) error {
	if s.Path == "" {
		return errors.BadRequest.With("no sidecar path specified")
	}
	if len(s.Listen) == 0 {
		return errors.BadRequest.With("no listen addresses specified")
	}

	db, err := s.open(inst.path(s.Path))
	if err != nil {
		return err
	}
	inst.cleanup("sidecar", func(context.Context) error { return db.Close() })

	l := &HttpListener{Listen: s.Listen}
	l.applyHttpDefaults()
	_, err = l.startHTTP(inst, sidecar.Handler(db))
	return err
}

// open opens the sidecar database read-only with the configured backend. The
// sidecar is never created by serving it, so a missing database is an error.
func (s *SidecarService) open(path string) (interface {
	keyvalue.Beginner
	io.Closer
}, error) {
	typ := StorageTypeLevelDB
	if s.StorageType != nil {
		typ = *s.StorageType
	}
	switch typ {
	case StorageTypeLevelDB:
		return leveldb.Open(path, leveldb.ReadOnly())
	case StorageTypeExpBlockDB:
		return block.Open(path, block.ReadOnly())
	default:
		return nil, errors.BadRequest.WithFormat("unsupported sidecar storage type %v", typ)
	}
}
