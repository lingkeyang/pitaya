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
	"context"

	"github.com/topfreegames/pitaya/constants"
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
