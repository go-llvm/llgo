// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package importer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/axw/llgo/build"

	"code.google.com/p/go.tools/go/gcimporter"
	"code.google.com/p/go.tools/go/importer"
	"code.google.com/p/go.tools/go/types"
)

var tracing = os.Getenv("LLGO_TRACE_IMPORTER") == "1"

type Importer struct {
	context *build.Context
}

// TODO(axw) consolidate the path logic in llgo/build
func packageExportsFile(ctx *build.Context, path string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = runtime.GOROOT()
	} else {
		gopath = filepath.SplitList(gopath)[0]
	}
	// FIXME(axw) ctx.Triple is wrong for pnacl
	return filepath.Join(gopath, "pkg", "llgo", ctx.Triple, path+".lgx")
}

func tracef(format string, args ...interface{}) {
	if tracing {
		fmt.Fprintf(os.Stderr, "[llgo/importer] "+format+"\n", args...)
	}
}

// NewImporter creates a new Importer that operates in the given build context.
func NewImporter(ctx *build.Context) *Importer {
	return &Importer{
		context: ctx,
	}
}

func (imp *Importer) Import(imports map[string]*types.Package, path string) (pkg *types.Package, err error) {
	exportsPath := packageExportsFile(imp.context, path)
	data, err := ioutil.ReadFile(exportsPath)
	if err != nil {
		// Package has not been compiled yet, so
		// fall back to the standard GcImport.
		tracef("Falling back to gc import data for: %s", path)
		return gcimporter.Import(imports, path)
	}
	tracef("Loading import data for %q from %q", path, exportsPath)
	return importer.ImportData(imports, data)
}

// Export generates a file containing package export data
// suitable for importing with Importer.Import.
func Export(ctx *build.Context, pkg *types.Package) error {
	exportsPath := packageExportsFile(ctx, pkg.Path())
	err := os.MkdirAll(filepath.Dir(exportsPath), 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	tracef("Writing import data to %q", exportsPath)
	return ioutil.WriteFile(exportsPath, importer.ExportData(pkg), 0644)
}
