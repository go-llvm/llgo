// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var (
	llvmconfig  string
	llvmversion string
	llvmcflags  string
	llvmlibdir  string
	llvmldflags string
	llvmbindir  string

	triple string
)

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func init() {
	flag.StringVar(&llvmconfig, "llvm-config", "llvm-config", "Path to the llvm-config executable")
	flag.StringVar(&triple, "triple", "", "The target triple")
}

func llvmconfigValue(option string) (string, error) {
	output, err := exec.Command(llvmconfig, option).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func initLlvm() error {
	var err error

	// Locate llvm-config.
	llvmconfig, err = userpath(llvmconfig)
	if err != nil {
		return err
	}
	llvmconfig, err = exec.LookPath(llvmconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		errorf("Specify the path with \"-llvm-config=...\"\n")
	}

	// llvm-config --version
	llvmversion, err = llvmconfigValue("--version")
	if err != nil {
		return err
	}
	log.Printf("LLVM version: %s", llvmversion)

	// llvm-config --bindir
	llvmbindir, err = llvmconfigValue("--bindir")
	if err != nil {
		return err
	}
	log.Printf("LLVM executables directory: %s", llvmbindir)

	// llvm-config --libdir
	llvmlibdir, err = llvmconfigValue("--libdir")
	if err != nil {
		return err
	}
	log.Printf("LLVM library directory: %s", llvmlibdir)

	// llvm-config --ldflags
	llvmldflags, err = llvmconfigValue("--ldflags")
	if err != nil {
		return err
	}
	log.Printf("LLVM LDFLAGS: %s", llvmldflags)

	// llvm-config --cflags
	llvmcflags, err = llvmconfigValue("--cflags")
	if err != nil {
		return err
	}
	log.Printf("LLVM CFLAGS: %s", llvmcflags)

	return nil
}

func main() {
	flag.Parse()

	actions := []func() error{
		initLlvm,
		buildLlgo,
		genSyscall,
		genMath,
		buildRuntime,
		buildLinker,
	}
	for _, action := range actions {
		err := action()
		if err != nil {
			errorf("%s\n", err)
		}
	}
}
