// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package irgen

import (
	"code.google.com/p/go.tools/go/types"
)

func deref(t types.Type) types.Type {
	return t.Underlying().(*types.Pointer).Elem()
}
