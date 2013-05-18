// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import "code.google.com/p/go.tools/go/types"

// pkgpath returns a package path suitable for naming symbols.
func pkgpath(p *types.Package) string {
	path := p.Path()
	name := p.Name()
	if path == "" || name == "main" {
		path = p.Name()
	}
	return path
}
