// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

type Dir struct {
	Name  string
	Files []DirEntry
}

type DirEntry interface {
	fs.File
	stat() fsFileInfo
}

func (d *Dir) Read(b []byte) (int, error) { return 0, io.EOF }
func (d *Dir) Close() error               { return nil }
func (d *Dir) Stat() (fs.FileInfo, error) { return d.stat(), nil }

func (d *Dir) stat() fsFileInfo {
	return fsFileInfo{d.Name, 0, true}
}

func (d *Dir) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	s := strings.SplitN(name, "/", 2)
	var f DirEntry
	f, ok := d.open(s[0])
	if !ok {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	if len(s) == 1 {
		return f, nil
	}

	e, ok := f.(*Dir)
	if !ok {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fmt.Errorf("%w: %q is not a directory", fs.ErrInvalid, s[0]),
		}
	}

	return e.Open(s[1])
}

func (d *Dir) open(name string) (DirEntry, bool) {
	for _, f := range d.Files {
		if f.stat().name == name {
			return f, true
		}
	}
	return nil, false
}

func (d *Dir) ReadDir(n int) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	for _, e := range d.Files {
		entries = append(entries, fsDirEntry(e.stat()))
		if n--; n == 0 {
			break
		}
	}
	return entries, nil
}

type File struct {
	Name string
	Data FileData
}

type FileData interface {
	io.Reader
	Len() int
}

func (f *File) Read(b []byte) (int, error) { return f.Data.Read(b) }
func (f *File) Close() error               { return nil }
func (f *File) Stat() (fs.FileInfo, error) { return f.stat(), nil }

func (f *File) stat() fsFileInfo {
	return fsFileInfo{f.Name, int64(f.Data.Len()), false}
}

type fsDirEntry fsFileInfo

func (e fsDirEntry) Name() string               { return e.name }
func (e fsDirEntry) IsDir() bool                { return e.isDir }
func (e fsDirEntry) Type() fs.FileMode          { return fsFileInfo(e).Mode() & fs.ModeType }
func (e fsDirEntry) Info() (fs.FileInfo, error) { return fsFileInfo(e), nil }

type fsFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (f fsFileInfo) Name() string       { return f.name }
func (f fsFileInfo) Size() int64        { return f.size }
func (f fsFileInfo) ModTime() time.Time { return time.Now() }
func (f fsFileInfo) IsDir() bool        { return f.isDir }
func (f fsFileInfo) Sys() any           { return nil }

func (f fsFileInfo) Mode() fs.FileMode {
	if f.isDir {
		return fs.ModeDir | fs.ModePerm
	}
	return fs.ModePerm
}
