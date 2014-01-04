// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const llgoPkgRuntime = "github.com/axw/llgo/pkg/runtime"

func buildRuntimeCgo() error {
	log.Println(" - generating platform-specific code")
	runtimepkg, err := build.Import(llgoPkgRuntime, "", build.FindOnly)
	if err != nil {
		return err
	}
	// Generate platform-specific parts of runtime,
	// e.g. Go-equivalent jmp_buf structs.
	objdir := filepath.Join(runtimepkg.Dir, "cgo_objdir")
	if err := os.Mkdir(objdir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(objdir)
	cmd := command(
		"go", "tool", "cgo",
		"-objdir="+objdir,
		"-import_runtime_cgo=false",
		"-import_syscall=false",
		"ctypes.go",
	)
	cmd.Dir = runtimepkg.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(output))
		if len(output) > 0 {
			err = fmt.Errorf("%v (%v)", err, output)
		}
		return fmt.Errorf("failed to process ctypes.go: %v", err)
	}
	for _, prefix := range []string{"_cgo_gotypes", "ctypes.cgo1"} {
		src := filepath.Join(objdir, prefix+".go")
		dst := fmt.Sprintf("z%s_%s_%s.go", prefix, buildctx.GOOS, buildctx.GOARCH)
		log.Printf("   - runtime/%s", dst)
		dst = filepath.Join(runtimepkg.Dir, dst)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func buildRuntime() (reterr error) {
	log.Println("Building runtime")

	if err := buildRuntimeCgo(); err != nil {
		return err
	}

	badPackages := []string{
		"net",         // Issue #71
		"os/user",     // Issue #72
		"runtime/cgo", // Issue #73
	}
	isBad := func(path string) bool {
		for _, bad := range badPackages {
			if path == bad {
				return true
			}
		}
		return false
	}

	output, err := command("go", "list", "std").CombinedOutput()
	if err != nil {
		return err
	}
	runtimePackages := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, pkg := range runtimePackages {
		if strings.HasPrefix(pkg, "cmd/") || isBad(pkg) {
			continue
		}
		log.Printf("- %s", pkg)
		output, err := command(llgobuildbin, pkg).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			reterr = err
		}
	}

	return
}
