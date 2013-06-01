// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	pepper      string
	llvmconfig  string
	llvmversion string
	llvmcflags  string
	llvmlibdir  string
	llvmlibs    string
	llvmldflags string
	llvmbindir  string

	triple      string
	buildctx    *build.Context
	sharedllvm  bool
	alwaysbuild bool
)

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func init() {
	flag.StringVar(&pepper, "pepper", "", "Path to the Native Client Pepper version to target (e.g. nacl_sdk/pepper_canary)")
	flag.StringVar(&llvmconfig, "llvm-config", "llvm-config", "Path to the llvm-config executable")
	flag.StringVar(&triple, "triple", "", "The target triple")
	flag.BoolVar(&sharedllvm, "shared", false, "If possible, dynamically link against LLVM")

	// We default this to true, as the eventually intended usage
	// of llgo-dist is for building binary distributions.
	flag.BoolVar(&alwaysbuild, "a", true, "Force rebuilding packages that are already up-to-date")
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

	// llvm-config --libs
	llvmlibs, err = llvmconfigValue("--libs")
	if err != nil {
		return err
	}
	log.Printf("LLVM libraries: %s", llvmlibs)

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

// checkLlvmLibs checks if static/shared libraries
// are available, and switches "sharedllvm" if necessary.
// If neither are available, returns an error.
func checkLlvmLibs() error {
	for i := 0; i < 2; i++ {
		if sharedllvm {
			// Look for a file that starts "libLLVM-<version>".
			prefix := fmt.Sprintf("libLLVM-%s", llvmversion)
			d, err := os.Open(llvmlibdir)
			if err != nil {
				return err
			}
			defer d.Close()
			names, err := d.Readdirnames(-1)
			if err != nil {
				return err
			}
			for _, name := range names {
				if strings.HasPrefix(name, prefix) {
					// Found the .so file.
					log.Printf("Located shared library: %s", name)
					return nil
				}
			}
		} else {
			// llvm-config --libnames
			llvmlibnames, err := llvmconfigValue("--libnames")
			if err != nil {
				return err
			}
			for _, f := range strings.Fields(llvmlibnames) {
				_, err = os.Stat(filepath.Join(llvmlibdir, f))
				if err != nil {
					break
				}
			}
			if err == nil {
				// Found all the .a files.
				log.Printf("Located static libraries")
				return nil
			}
		}
		if i == 0 {
			a, b := "shared library", "static libraries"
			if !sharedllvm {
				a, b = b, a
			}
			log.Printf("Failed to locate %s, will try %s", a, b)
			sharedllvm = !sharedllvm
		}
	}
	return fmt.Errorf("No static or shared libraries found in %q", llvmlibdir)
}

func main() {
	flag.Parse()

	var actions []func() error
	if pepper != "" {
		actions = append(actions, initPepper)
	}
	actions = append(actions,
		initLlvm,
		checkLlvmLibs,
		buildLlgo,
		buildLlgoTools,
		buildRuntime,
	)

	for _, action := range actions {
		err := action()
		if err != nil {
			errorf("%s\n", err)
		}
	}
}
