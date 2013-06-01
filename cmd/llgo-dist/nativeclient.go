// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// pnaclClang is the path to the pnacl-clang script.
var pnaclClang string

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

	newlibBinDir := filepath.Join(pnaclToolchainDir, "newlib", "bin")
	if runtime.GOARCH == "amd64" {
		newlibBinDir += "64"
	}

	pnaclClang = filepath.Join(newlibBinDir, "pnacl-clang")
	llvmconfig = filepath.Join(hostBinDir, "llvm-config")
	triple = "pnacl"
	return nil
}
