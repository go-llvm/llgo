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

	badPackages := []string{
		"net",         // Issue #71
		"os/user",     // Issue #72
	}
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

	output, err := command("go", "list", "std").CombinedOutput()
	if err != nil {
		return err
	}

	// Always build runtime and syscall first
	// TODO: Real import dependency discovery to build packages in the order they depend on each other
	runtimePackages := append([]string{"runtime", "syscall"}, strings.Split(strings.TrimSpace(string(output)), "\n")...)
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
