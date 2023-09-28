package ioutil

import (
	"errors"
	"io"
)

type Discard struct {
	offset int64
	end    int64
}

func (d *Discard) Write(p []byte) (n int, err error) {
	d.offset += int64(len(p))
	return len(p), nil
}

func (d *Discard) Seek(offset int64, whence int) (int64, error) {
	if d.offset > d.end {
		d.end = d.offset
	}
	switch whence {
	case io.SeekCurrent:
		d.offset += offset
	case io.SeekStart:
		d.offset = offset
	case io.SeekEnd:
		d.offset = d.end + offset
	default:
		return 0, errors.New("invalid whence")
	}
	return d.offset, nil
}
