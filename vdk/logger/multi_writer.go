package logger

import (
	"errors"
	"fmt"
	"io"
)

type multiWriter []io.Writer

func (w multiWriter) Write(b []byte) (int, error) {
	for _, w := range w {
		n, err := w.Write(b)
		if err != nil {
			return n, err
		}
		if n != len(b) {
			return n, io.ErrShortWrite
		}
	}
	return len(b), nil
}

func (w multiWriter) Close() error {
	var errs []error
	for _, w := range w {
		c, ok := w.(io.Closer)
		if !ok {
			continue
		}
		err := c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return nil
	default:
		return errors.New(fmt.Sprint(errs))
	}
}
