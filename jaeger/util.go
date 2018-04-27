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

package jaeger

import (
	"context"

	opentracing "github.com/opentracing/opentracing-go"
)

func StartSpan(
	parentCtx context.Context,
	opName string,
	tags opentracing.Tags,
	reference ...opentracing.SpanContext,
) (opentracing.Span, context.Context) {
	var ref opentracing.SpanContext
	if len(reference) > 0 {
		ref = reference[0]
	}
	span := opentracing.StartSpan(opName, opentracing.ChildOf(ref), tags)
	ctx := opentracing.ContextWithSpan(parentCtx, span)
	return span, ctx
}

func FinishSpan(ctx context.Context, err error) {
	if ctx == nil {
		return
	}
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return
	}
	defer span.Finish()
	if err != nil {
		LogError(span, err.Error())
	}
}
