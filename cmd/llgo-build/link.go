// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"io/ioutil"
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

	var mkdeps func(imports ...string) error
	mkdeps = func(imports ...string) error {
		for _, path := range imports {
			if path == "C" {
				if err := mkdeps("runtime/cgo", "syscall"); err != nil {
					return err
				}
				continue
			}
			if !deps[path] {
				deps[path] = true
				pkg, err := build.Import(path, "", 0)
				if err != nil {
					return err
				}
				if err = mkdeps(pkg.Imports...); err != nil {
					return err
				}
				depslist = append(depslist, path)
			}
		}
		return nil
	}

	err := mkdeps(pkg.Imports...)
	if err != nil {
		return err
	}
	if test {
		if err = mkdeps(pkg.TestImports...); err != nil {
			return err
		}
		if err = mkdeps(pkg.XTestImports...); err != nil {
			return err
		}
	}

	for i := 0; i < len(depslist)/2; i++ {
		j := len(depslist) - i - 1
		depslist[i], depslist[j] = depslist[j], depslist[i]
	}

	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	args := []string{"-o", output, output}
	var ldflags []string
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
		if pkgldflags, err := readLdflags(path); err != nil {
			return err
		} else {
			ldflags = append(ldflags, pkgldflags...)
		}
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

		args := []string{"-pthread", "-v", "-g", "-o", output, input}
		if triple == "pnacl" {
			args = append(args, "-l", "ppapi")
		}
		args = append(args, ldflags...)
		cmd := exec.Command(clang+"++", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = runCmd(cmd); err != nil {
			return err
		}
	}

	return nil
}

// writeLdflags writes CGO_LDFLAGS flags to a file, one argument per line.
func writeLdflags(pkgpath string, flags []string) error {
	file := filepath.Join(pkgroot, pkgpath+".ldflags")
	return ioutil.WriteFile(file, []byte(strings.Join(flags, "\n")), 0644)
}

// readLdflags reads CGO_LDFLAGS written to a file by writeLdflags.
func readLdflags(pkgpath string) ([]string, error) {
	data, err := ioutil.ReadFile(filepath.Join(pkgroot, pkgpath+".ldflags"))
	if err == nil {
		return strings.Split(string(data), "\n"), nil
	} else if os.IsNotExist(err) {
		return nil, nil
	}
	return nil, err
}
