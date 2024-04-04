// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"io"
	"net"
	"net/url"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

func serveKeyValueStore(ctx context.Context, store keyvalue.Beginner, addr net.Addr) error {
	l, err := net.Listen(addr.Network(), addr.String())
	if err != nil {
		return err
	}
	defer l.Close()

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}

		err = remote.Serve(store, c, nil, false)
		if err != nil {
			return err
		}
	}
}

func withRemoteKeyValueStore(addr net.Addr, fn func(*remote.Store) error) error {
	c, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	return fn(remote.ConnectStore(c))
}

func openDbUrl(arg string, writable bool) (_ dbCloser, _ net.Addr) {
	u, err := url.Parse(arg)
	check(err)

	plainKeys := u.Query().Get("keyFormat") == "plain"

	switch u.Scheme {
	case "unix":
		addr, err := net.ResolveUnixAddr(u.Scheme, u.Path)
		check(err)
		return nil, addr

	case "tcp":
		addr, err := net.ResolveTCPAddr(u.Scheme, u.Host)
		check(err)
		return nil, addr

	case "udp":
		addr, err := net.ResolveUDPAddr(u.Scheme, u.Host)
		check(err)
		return nil, addr

	case "snapshot":
		if writable {
			fatalf("cannot write to a snapshot like this")
		}
		f := openSnapshotFile(u.Path)
		db := openSnapshotAsDb(f)
		return db, nil

	case "badger":
		var opts []badger.Option
		if plainKeys {
			opts = append(opts, badger.WithPlainKeys)
		}
		db, err := badger.New(u.Path, opts...)
		check(err)
		return db, nil

	case "bolt":
		var opts []bolt.Option
		if plainKeys {
			opts = append(opts, bolt.WithPlainKeys)
		}
		db, err := bolt.Open(u.Path, opts...)
		check(err)
		return db, nil

	case "leveldb":
		db, err := leveldb.OpenFile(u.Path)
		check(err)
		return db, nil

	default:
		fatalf("unknown database kind %q", u.Scheme)
		panic("not reached")
	}
}

func openSnapshotAsDb(file ioutil.SectionReader) dbCloser {
	ver, err := sv2.GetVersion(file)
	check(err)

	switch ver {
	case sv1.Version1:
		fatalf("cannot use a v1 snapshot as a store")

	case sv2.Version2:
		snap, err := sv2.Open(file)
		check(err)
		store, err := snap.AsStore()
		check(err)
		return &snapDb{file, store}

	default:
		fatalf("unsupported snapshot version %d", ver)
	}
	panic("not reached")
}

type dbCloser interface {
	keyvalue.Beginner
	io.Closer
}

type snapDb struct {
	file  ioutil.SectionReader
	store *sv2.Store
}

func (s *snapDb) Close() error {
	if f, ok := s.file.(io.Closer); ok {
		return f.Close()
	}
	return nil
}

func (s *snapDb) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return snapStore{s.store}
}

type snapStore struct {
	*sv2.Store
}

func (s snapStore) Begin(*record.Key, bool) keyvalue.ChangeSet { return s }
func (snapStore) Commit() error                                { panic("cannot commit to a snapshot") }
func (snapStore) Discard()                                     {}
