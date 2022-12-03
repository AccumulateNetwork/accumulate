package message

import (
	"context"
	stderr "errors"
	"io"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// pipe is a Stream backed by channels.
type pipe[T encoding.BinaryValue] struct {
	rd        pipedir[<-chan []byte]  // Read
	wr        pipedir[chan<- []byte]  // Write
	unmarshal func([]byte) (T, error) // Unmarshal
}

// pipedir half of a pipe.
type pipedir[T any] struct {
	ch               T                  // Channel
	ctx, dlCtx       context.Context    // Main context
	cancel, dlCancel context.CancelFunc // Deadline context
}

// newPipeDir constructs a new pipedir.
func newPipeDir[T any](ch T, ctx context.Context) pipedir[T] {
	ctx, cancel := context.WithCancel(ctx)
	return pipedir[T]{ch, ctx, context.Background(), cancel, func() {}}
}

// Pipe allocates a simplex [Message] [Stream] backed by an unbuffered channel.
func Pipe(ctx context.Context) *pipe[Message] {
	return newSimplex(ctx, Unmarshal)
}

// PipeOf allocates a simplex [Stream] of *T backed by an unbuffered channel. *T
// must implement [encoding.BinaryValue].
func PipeOf[T any, PT valuePtr[T]](ctx context.Context) *pipe[PT] {
	return newSimplex(ctx, func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	})
}

// newSimplex allocates a simplex pipe.
func newSimplex[T encoding.BinaryValue](ctx context.Context, unmarshal func([]byte) (T, error)) *pipe[T] {
	ch := make(chan []byte)
	p := new(pipe[T])
	p.unmarshal = unmarshal
	p.rd = newPipeDir[<-chan []byte](ch, ctx)
	p.wr = newPipeDir[chan<- []byte](ch, ctx)
	return p
}

// DuplexPipe allocates a pair of duplex [Message] [Stream]s backed by a pair of
// unbuffered channels.
func DuplexPipe(ctx context.Context) (p, q *pipe[Message]) {
	return newDuplex(ctx, Unmarshal)
}

// DuplexPipeOf allocates a pair of duplex [Stream]s of *T backed by a pair of
// unbuffered channels. *T must implement [encoding.BinaryValue].
func DuplexPipeOf[T any, PT valuePtr[T]](ctx context.Context) (p, q *pipe[PT]) {
	return newDuplex(ctx, func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	})
}

// newDuplex allocates a pair of duplex pipes.
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

// piperr returns onCancel if the context error is [context.Canceled].
func piperr(ctx context.Context, onCancel error) error {
	err := ctx.Err()
	if errors.Is(err, context.Canceled) {
		return onCancel
	}
	return err
}

// ErrDeadline is returned if a pipe read or write exceeds the deadline.
var ErrDeadline = stderr.New("deadline exceeded")

// Read reads a value from the read channel. Read will only return an error if
// the pipe is closed, a deadline is hit, or unmarshalling fails.
func (p *pipe[T]) Read() (T, error) {
	// Use select so closes and deadlines are respected
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

// Write writes a value to the write channel. Write will only return an error if
// the pipe is closed, a deadline is hit, or marshalling fails.
func (s *pipe[T]) Write(v T) error {
	// Marshal the message to ensure pipeStream behaves the same as a network
	// stream. Otherwise there will be subtle differences between directly
	// passing a pointer vs streaming bytes.
	b, err := v.MarshalBinary()
	if err != nil {
		return errors.EncodingError.Wrap(err)
	}

	// Use select so closes and deadlines are respected
	select {
	case s.wr.ch <- b:
		return nil
	case <-s.wr.dlCtx.Done():
		return piperr(s.wr.dlCtx, ErrDeadline)
	case <-s.wr.ctx.Done():
		return piperr(s.wr.ctx, io.EOF)
	}
}

// Close closes the pipe.
func (s *pipe[T]) Close() error {
	_ = s.CloseRead()
	_ = s.CloseWrite()
	return nil
}

// CloseWrite closes the write side of the pipe.
func (s *pipe[T]) CloseWrite() error {
	s.wr.cancel()
	return nil
}

// CloseRead closes the read side of the pipe.
func (s *pipe[T]) CloseRead() error {
	s.rd.cancel()
	return nil
}

// SetDeadline sets a read and write deadline.
func (s *pipe[T]) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	_ = s.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline sets a read deadline.
func (s *pipe[T]) SetReadDeadline(t time.Time) error {
	s.rd.dlCtx, s.rd.dlCancel = context.WithDeadline(s.rd.ctx, t)
	return nil
}

// SetWriteDeadline sets a write deadline.
func (s *pipe[T]) SetWriteDeadline(t time.Time) error {
	s.wr.dlCtx, s.wr.dlCancel = context.WithDeadline(s.wr.ctx, t)
	return nil
}