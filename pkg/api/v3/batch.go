// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import "context"

type BatchData struct {
	context context.Context
	values  map[any]any
}

func (d *BatchData) Context() context.Context { return d.context }
func (d *BatchData) Get(k any) any            { return d.values[k] }
func (d *BatchData) Put(k, v any)             { d.values[k] = v }

type contextKeyBatch struct{}

func (contextKeyBatch) String() string { return "batch context key" }

// ContextWithBatchData will return a context with batch data. If the incoming
// context already has batch data, ContextWithBatch returns the incoming context
// and a noop cancel function. If the incoming context does not have batch data,
// ContextWithBatch will create a new cancellable context with batch data.
func ContextWithBatchData(ctx context.Context) (context.Context, context.CancelFunc, *BatchData) {
	v, _ := ctx.Value(contextKeyBatch{}).(*BatchData)
	if v != nil {
		return ctx, func() {}, v
	}

	ctx, cancel := context.WithCancel(ctx)
	bd := new(BatchData)
	bd.context = ctx
	bd.values = map[any]any{}
	ctx = context.WithValue(ctx, contextKeyBatch{}, bd)
	return ctx, cancel, bd
}

func GetBatchData(ctx context.Context) *BatchData {
	v, _ := ctx.Value(contextKeyBatch{}).(*BatchData)
	return v
}
