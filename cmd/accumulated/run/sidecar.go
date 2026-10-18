// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
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

	db, err := leveldb.Open(inst.path(s.Path))
	if err != nil {
		return err
	}
	inst.cleanup("sidecar", func(context.Context) error { return db.Close() })

	var era keyvalue.Beginner
	if len(s.Era) > 0 {
		dirs := make([]string, len(s.Era))
		for i, d := range s.Era {
			dirs[i] = inst.path(d)
		}
		e, err := sidecar.OpenEra(dirs, db)
		if err != nil {
			return errors.UnknownError.WithFormat("open the era: %w", err)
		}
		inst.cleanup("sidecar era", func(context.Context) error { return e.Close() })
		era = e
	}

	l := &HttpListener{Listen: s.Listen}
	l.applyHttpDefaults()
	_, err = l.startHTTP(inst, sidecar.HandlerWithEra(db, era))
	return err
}
