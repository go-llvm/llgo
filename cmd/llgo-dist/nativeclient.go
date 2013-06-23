// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const llgoPkgSyscall = "github.com/axw/llgo/pkg/syscall"

// pnaclClang is the path to the pnacl-clang script.
var pnaclClang string

type pnaclToolchain struct {
	root      string // root dir (pepper_X)
	toolchain string // toolchain dir
	host      string // host dir
	newlib    string // newlib dir
	newlibbin string // newlib bin dir
	clang     string // clang binary
	clanglib  string // lib/clang/<version>
}

// initPepper configures the llgo-dist parameters to build
// for PNaCl (Portable Native Client), given the path to a
// Native Client SDK Pepper version.
func initPepper() error {
	var err error
	pepper, err = userpath(pepper)
	if err != nil {
		return err
	}

	var platform string
	switch runtime.GOOS {
	case "linux":
		platform = "linux"
	case "windows":
		platform = "win"
	case "darwin":
		platform = "mac"
	default:
		return fmt.Errorf("Unsupported host OS: %s", runtime.GOOS)
	}

	host := "x86_32"
	if runtime.GOARCH == "amd64" {
		host = "x86_64"
	}

	pnaclToolchainDir := filepath.Join(pepper, "toolchain", platform+"_x86_pnacl")
	hostDir := filepath.Join(pnaclToolchainDir, "host_"+host)
	hostBinDir := filepath.Join(hostDir, "bin")

	newlibDir := filepath.Join(pnaclToolchainDir, "newlib")
	newlibBinDir := filepath.Join(newlibDir, "bin")
	if runtime.GOARCH == "amd64" {
		newlibBinDir += "64"
	}

	// Get the clang version, using which we'll
	// determine the lib/clang directory.
	clang := filepath.Join(hostBinDir, "clang")
	output, err := exec.Command(clang, "--version").CombinedOutput()
	if err != nil {
		return err
	}
	var clangVersion string
	if output := string(output); strings.HasPrefix(output, "clang version") {
		// Output of `clang --version` is, e.g. "clang version 3.3"
		clangVersion = strings.Fields(output)[2]
	} else {
		return fmt.Errorf("Unexpected output: %q", output)
	}
	clangLibDir := filepath.Join(hostDir, "lib", "clang", clangVersion)

	// Set globals
	pnaclClang = filepath.Join(newlibBinDir, "pnacl-clang")
	llvmconfig = filepath.Join(hostBinDir, "llvm-config")
	triple = "pnacl"

	toolchain := pnaclToolchain{
		root:      pepper,
		toolchain: pnaclToolchainDir,
		host:      hostDir,
		newlib:    newlibDir,
		newlibbin: newlibBinDir,
		clang:     clang,
		clanglib:  clangLibDir,
	}

	err = toolchain.makeSyscall()

	return nil
}

func (t *pnaclToolchain) makeSyscall() error {
	// Generate syscall bits.
	// All PNaCl code is generated as if 32-bit, hence GOARCH=386.
	syscallpkg, err := build.Import(llgoPkgSyscall, "", build.FindOnly)
	if err != nil {
		return err
	}

	objdir, err := ioutil.TempDir("", "llgo_dist")
	if err != nil {
		return fmt.Errorf("Failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(objdir)

	args := []string{
		"tool", "cgo", "-godefs", "-objdir", objdir, "--",
		"-nostdinc",
		"-D__native_client__",
		"-isystem", filepath.Join(t.newlib, "usr", "include"),
		"-isystem", filepath.Join(t.root, "include"),
		"-isystem", filepath.Join(t.clanglib, "include"),
		"-isystem", filepath.Join(t.newlib, "sysroot", "include"),
		filepath.Join(syscallpkg.Dir, "types_pnacl.go"),
	}
	f, err := os.Create(filepath.Join(syscallpkg.Dir, "ztypes_pnacl.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Add "// +build pnacl" to the beginning of the file,
	// with newlines on either side.
	fmt.Fprintln(f)
	fmt.Fprintln(f, "// +build pnacl")
	fmt.Fprintln(f)

	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CC="+t.clang)
	cmd.Env = append(cmd.Env, "GOARCH=386")
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
