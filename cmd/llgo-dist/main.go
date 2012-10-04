// Copyright 2012 Andrew Wilkins.
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
)

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func init() {
	flag.StringVar(&llvmconfig, "llvm-config", "llvm-config",
		"Path to the llvm-config executable")
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
	var output []byte
	output, err = exec.Command(llvmconfig, "--version").CombinedOutput()
	if err != nil {
		return err
	}
	llvmversion = strings.TrimSpace(string(output))
	log.Printf("LLVM version: %s", llvmversion)

	// llvm-config --libdir
	output, err = exec.Command(llvmconfig, "--libdir").CombinedOutput()
	if err != nil {
		return err
	}
	llvmlibdir = strings.TrimSpace(string(output))
	log.Printf("LLVM library directory: %s", llvmlibdir)

	// llvm-config --ldflags
	output, err = exec.Command(llvmconfig, "--ldflags").CombinedOutput()
	if err != nil {
		return err
	}
	llvmldflags = strings.TrimSpace(string(output))
	log.Printf("LLVM LDFLAGS: %s", llvmldflags)

	// llvm-config --cflags
	output, err = exec.Command(llvmconfig, "--cflags").CombinedOutput()
	if err != nil {
		return err
	}
	llvmcflags = strings.TrimSpace(string(output))
	log.Printf("LLVM CFLAGS: %s", llvmcflags)

	return nil
}

func main() {
	flag.Parse()

	actions := []func() error{
		initLlvm,
		buildLlgo,
		buildRuntime,
	}
	for _, action := range actions {
		err := action()
		if err != nil {
			errorf("%s\n", err)
		}
	}
}
