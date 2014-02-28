// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// linkdeps links dependencies into the specified output file.
func linkdeps(pkg *build.Package, output string) error {
	depslist := []string{"runtime"}
	deps := make(map[string]bool)
	deps["runtime"] = true
	deps["unsafe"] = true
	if len(pkg.CgoFiles) > 0 {
		pkg.Imports = append(pkg.Imports, "runtime/cgo")
		pkg.Imports = append(pkg.Imports, "syscall")
	}

	var mkdeps func(pkg *build.Package, imports []string) error
	mkdeps = func(pkg *build.Package, imports []string) error {
		for _, path := range imports {
			if path == "C" {
				continue
			}
			if !deps[path] {
				deps[path] = true
				pkg, err := build.Import(path, "", 0)
				if err != nil {
					return err
				}
				if err = mkdeps(pkg, pkg.Imports); err != nil {
					return err
				}
				depslist = append(depslist, path)
			}
		}
		return nil
	}

	err := mkdeps(pkg, pkg.Imports)
	if err != nil {
		return err
	}
	if test {
		if err = mkdeps(pkg, pkg.TestImports); err != nil {
			return err
		}
		if err = mkdeps(pkg, pkg.XTestImports); err != nil {
			return err
		}
	}

	for i := 0; i < len(depslist)/2; i++ {
		j := len(depslist) - i - 1
		depslist[i], depslist[j] = depslist[j], depslist[i]
	}

	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	args := []string{"-o", output, output}
	for _, path := range depslist {
		if path == pkg.ImportPath {
			continue
		}
		bcfile := filepath.Join(pkgroot, path+".bc")
		if buildDeps {
			if _, err := os.Stat(bcfile); err != nil {
				if err = buildPackages([]string{path}); err != nil {
					return err
				}
			}
		}
		args = append(args, bcfile)
	}
	cmd := exec.Command(llvmlink, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = runCmd(cmd); err != nil {
		return err
	}

	if !emitllvm || triple == "pnacl" {
		input := output
		if strings.Contains(triple, "darwin") || strings.Contains(triple, "mac") {
			// Not doing this intermediate step will make it invoke "dsymutil"
			// which then asserts and kills the build.
			// See discussion in issue #49 for more details.
			input += ".o"
			args := []string{"-g", "-c", "-o", input, output}
			cmd := exec.Command(clang, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err = runCmd(cmd); err != nil {
				return err
			}
		}

		args := []string{"-pthread", "-g", "-o", output, input}
		if triple == "pnacl" {
			args = append(args, "-l", "ppapi")
		}
		cmd := exec.Command(clang, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = runCmd(cmd); err != nil {
			return err
		}
	}

	return nil
}
