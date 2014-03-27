// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const llgoPkgRuntime = "github.com/go-llvm/llgo/pkg/runtime"

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
		data, err := ioutil.ReadFile(src)
		if err != nil {
			return err
		}
		dst = filepath.Join(runtimepkg.Dir, dst)
		// The GOOS/GOARCH llgo generates may not be known by the go tool,
		// and so will not be considered in filename filtering (file_$GOOS_$GOARCH).
		// Add tags to the generated files.
		data = append([]byte(
			"//\n\n"+
				"// +build "+buildctx.GOOS+"\n"+
				"// +build "+buildctx.GOARCH+"\n\n",
		), data...)
		if err := ioutil.WriteFile(dst, data, 0644); err != nil {
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

	var badPackages []string
	if triple == "pnacl" {
		badPackages = append(badPackages,
			"crypto/md5",
			"crypto/rc4",
			"hash/crc32",
			"mime",
			"os/exec",
			"os/signal",
			"path/filepath",
		)
	}
	isBad := func(path string) bool {
		for _, bad := range badPackages {
			if path == bad {
				return true
			}
		}
		return false
	}

	visited := make(map[string]bool)
	var stdpkgs []string
	var mkdeps func(imports []string) error
	mkdeps = func(imports []string) error {
		for _, path := range imports {
			if path == "C" || path == "unsafe" || visited[path] {
				continue
			}
			visited[path] = true
			pkg, err := build.Import(path, "", 0)
			if err != nil {
				return err
			}
			if err = mkdeps(pkg.Imports); err != nil {
				return err
			}
			stdpkgs = append(stdpkgs, path)
		}
		return nil
	}

	output, err := command("go", "list", "std").CombinedOutput()
	if err != nil {
		return err
	}
	var unordered []string
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if !strings.HasPrefix(path, "cmd/") {
			unordered = append(unordered, path)
		}
	}
	err = mkdeps(unordered)
	if err != nil {
		return err
	}
	for _, pkg := range stdpkgs {
		if pkg == "unsafe" || isBad(pkg) {
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
