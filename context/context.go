// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package context

import (
	"bytes"
	"context"
	"encoding/gob"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/topfreegames/pitaya/constants"
	"github.com/topfreegames/pitaya/util"
)

// AddToPropagateCtx adds a key and value that will be propagated through RPC calls
func AddToPropagateCtx(ctx context.Context, key string, val interface{}) context.Context {
	propagate := ToMap(ctx)
	propagate[key] = val
	return context.WithValue(ctx, constants.PropagateCtxKey, propagate)
}

// GetFromPropagateCtx adds a key and value to the propagate
func GetFromPropagateCtx(ctx context.Context, key string) interface{} {
	propagate := ToMap(ctx)
	if val, ok := propagate[key]; ok {
		return val
	}
	return nil
}

// ToMap returns the values that will be propagated through RPC calls in map[string]interface{} format
func ToMap(ctx context.Context) map[string]interface{} {
	p := ctx.Value(constants.PropagateCtxKey)
	if p != nil {
		return p.(map[string]interface{})
	}
	return map[string]interface{}{}
}

// FromMap creates a new context with the propagated values
func FromMap(val map[string]interface{}) context.Context {
	return context.WithValue(context.Background(), constants.PropagateCtxKey, val)
}

// Encode returns the given span propagatable context encoded in binary format
func Encode(ctx context.Context) ([]byte, error) {
	m := ToMap(ctx)
	if len(m) > 0 {
		return util.GobEncode(m)
	}
	return nil, nil
}

// Decode returns a context given a binary encoded message
func Decode(m []byte) (context.Context, error) {
	if len(m) == 0 {
		// TODO maybe return an error
		return nil, nil
	}
	ctxMap := map[string]interface{}{}
	args := make([]interface{}, 0)
	err := gob.NewDecoder(bytes.NewReader(m)).Decode(&args)
	if err != nil {
		return nil, err
	}
	ctxMap = args[0].(map[string]interface{})
	return FromMap(ctxMap), nil
}

// GetSpanContext retrieves an opentracing span context from the given context
// The span context can be received directly or via an RPC call
func GetSpanContext(ctx context.Context) (opentracing.SpanContext, error) {
	var spanCtx opentracing.SpanContext
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		spanData := GetFromPropagateCtx(ctx, constants.SpanPropagateCtxKey).([]byte)
		tracer := opentracing.GlobalTracer()
		var err error
		spanCtx, err = tracer.Extract(opentracing.Binary, bytes.NewBuffer(spanData))
		if err != nil {
			return nil, err
		}
	} else {
		spanCtx = span.Context()
	}
	return spanCtx, nil
}

// InjectSpan retrieves an opentrancing span from the current context and injects it
// encoded in binary format into the propagate context
func InjectSpan(ctx context.Context) (context.Context, error) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return ctx, nil
	}
	spanData := new(bytes.Buffer)
	tracer := opentracing.GlobalTracer()
	err := tracer.Inject(span.Context(), opentracing.Binary, spanData)
	if err != nil {
		return nil, err
	}
	return AddToPropagateCtx(ctx, constants.SpanPropagateCtxKey, spanData.Bytes()), nil
}
