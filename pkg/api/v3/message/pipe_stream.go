package message

import (
	"context"
	stderr "errors"
	"io"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type pipe[T encoding.BinaryValue] struct {
	rd        pipedir[<-chan []byte]
	wr        pipedir[chan<- []byte]
	unmarshal func([]byte) (T, error)
}

type pipedir[T any] struct {
	ch               T
	ctx, dlCtx       context.Context
	cancel, dlCancel context.CancelFunc
}

func newPipeDir[T any](ch T, ctx context.Context) pipedir[T] {
	ctx, cancel := context.WithCancel(ctx)
	return pipedir[T]{ch, ctx, context.Background(), cancel, func() {}}
}

func Pipe(ctx context.Context) *pipe[Message] {
	return newSimplex(ctx, Unmarshal)
}

func PipeOf[T any, PT valuePtr[T]](ctx context.Context) *pipe[PT] {
	return newSimplex(ctx, func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	})
}

func newSimplex[T encoding.BinaryValue](ctx context.Context, unmarshal func([]byte) (T, error)) *pipe[T] {
	ch := make(chan []byte)
	p := new(pipe[T])
	p.unmarshal = unmarshal
	p.rd = newPipeDir[<-chan []byte](ch, ctx)
	p.wr = newPipeDir[chan<- []byte](ch, ctx)
	return p
}

func DuplexPipe(ctx context.Context) (p, q *pipe[Message]) {
	return newDuplex(ctx, Unmarshal)
}

func DuplexPipeOf[T any, PT valuePtr[T]](ctx context.Context) (p, q *pipe[PT]) {
	return newDuplex(ctx, func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	})
}

func newDuplex[T encoding.BinaryValue](ctx context.Context, unmarshal func([]byte) (T, error)) (p, q *pipe[T]) {
	p, q = new(pipe[T]), new(pipe[T])
	p.unmarshal, q.unmarshal = unmarshal, unmarshal

	// p → q
	pq := make(chan []byte)
	p.wr = newPipeDir[chan<- []byte](pq, ctx)
	q.rd = newPipeDir[<-chan []byte](pq, ctx)

	// p → q
	qp := make(chan []byte)
	q.wr = newPipeDir[chan<- []byte](qp, ctx)
	p.rd = newPipeDir[<-chan []byte](qp, ctx)

	return p, q
}

func piperr(ctx context.Context, onCancel error) error {
	err := ctx.Err()
	if errors.Is(err, context.Canceled) {
		return onCancel
	}
	return nil
}

var ErrDeadline = stderr.New("deadline exceeded")

func (p *pipe[T]) Read() (T, error) {
	var b []byte
	var z T
	select {
	case b = <-p.rd.ch:
		// Ok
	case <-p.rd.dlCtx.Done():
		return z, piperr(p.rd.dlCtx, ErrDeadline)
	case <-p.rd.ctx.Done():
		return z, piperr(p.rd.ctx, io.EOF)
	}

	// See Write
	v, err := p.unmarshal(b)
	if err != nil {
		return z, errors.EncodingError.Wrap(err)
	}
	return v, nil
}

func (s *pipe[T]) Write(v T) error {
	// Marshal the message to ensure pipeStream behaves the same as a network
	// stream. Otherwise there will be subtle differences between directly
	// passing a pointer vs streaming bytes.
	b, err := v.MarshalBinary()
	if err != nil {
		return errors.EncodingError.Wrap(err)
	}

	select {
	case s.wr.ch <- b:
		return nil
	case <-s.wr.dlCtx.Done():
		return piperr(s.wr.dlCtx, ErrDeadline)
	case <-s.wr.ctx.Done():
		return piperr(s.wr.ctx, io.EOF)
	}
}

func (s *pipe[T]) Close() error {
	_ = s.CloseRead()
	_ = s.CloseWrite()
	return nil
}

func (s *pipe[T]) CloseWrite() error {
	s.rd.cancel()
	return nil
}

func (s *pipe[T]) CloseRead() error {
	s.wr.cancel()
	return nil
}

func (s *pipe[T]) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	_ = s.SetWriteDeadline(t)
	return nil
}

func (s *pipe[T]) SetReadDeadline(t time.Time) error {
	s.rd.dlCtx, s.rd.dlCancel = context.WithDeadline(s.rd.ctx, t)
	return nil
}

func (s *pipe[T]) SetWriteDeadline(t time.Time) error {
	s.wr.dlCtx, s.wr.dlCancel = context.WithDeadline(s.wr.ctx, t)
	return nil
}
