// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/axw/llgo/build"

	"code.google.com/p/go.tools/go/gcimporter"
	"code.google.com/p/go.tools/go/types"
)

var (
	importsRe = regexp.MustCompile(`import \w+ "([\w/]+)"`)
	tracing   = os.Getenv("LLGO_TRACE_IMPORTER") == "1"
)

type Importer struct {
	context *build.Context

	// myimports contains go/types Packages loaded by the importer.
	myimports map[string]*types.Package
}

// TODO(axw) consolidate the path logic in llgo/build
func packageExportsFile(ctx *build.Context, path string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = runtime.GOROOT()
	} else {
		gopath = filepath.SplitList(gopath)[0]
	}
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
		context:   ctx,
		myimports: make(map[string]*types.Package),
	}
}

func (imp *Importer) Import(imports map[string]*types.Package, path string) (pkg *types.Package, err error) {
	// myimports is different from imports. Imports can contain
	// dummy packages not loaded by us, while myimports will
	// be "pure".
	if pkg, ok := imp.myimports[path]; ok {
		if pkg == nil {
			return nil, fmt.Errorf("Previous attempt at loading package failed")
		}
		return pkg, nil
	}

	var data []byte
	pkgfile := packageExportsFile(imp.context, path)
	if path == "unsafe" {
		// Importing these packages have issues
		//
		// unsafe:
		// 		If this is actually imported, go.types will panic due to invalid type conversions.
		//		This because it is a built in package  (http://tip.golang.org/doc/spec#Package_unsafe)
		// 		and thus shouldn't be treated as a normal package anyway.
	} else {
		data, _ = ioutil.ReadFile(pkgfile)
	}

	if data != nil {
		// Need to load dependencies first
		for _, match := range importsRe.FindAllStringSubmatch(string(data), -1) {
			if _, ok := imp.myimports[match[1]]; !ok {
				_, err := imp.Import(imports, match[1])
				if err != nil {
					return nil, err
				}
			}
		}
		pkg, err = gcimporter.ImportData(imports, pkgfile, path, bufio.NewReader(bytes.NewBuffer(data)))
		if err == nil {
			tracef("Loaded import data from: %s", pkgfile)
		}
	}

	if pkg == nil || err != nil {
		if data != nil {
			return nil, fmt.Errorf("Failed to load package %s: %v", path, err)
		}
		// Package has not been compiled yet, so fall back to
		// the standard GcImport.
		tracef("Falling back to gc import data for: %s", path)
		pkg, err = gcimporter.Import(imports, path)
	}

	imp.myimports[path] = pkg
	imports[path] = pkg
	return pkg, err
}
