// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
)

var (
	outfile  string
	llvmlink string
)

func init() {
	flag.StringVar(&outfile, "o", "-", "Output filename (- for stdout)")
	flag.StringVar(&llvmlink, "llvm-link", "llvm-link", "Path to the llvm-link binary")
}

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func usage() {
	exename := path.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <files...>\n\n", exename)
	flag.PrintDefaults()
}

func link(files []string) error {
	tempf, err := ioutil.TempFile("", "llgolink")
	if err != nil {
		return err
	}
	defer os.Remove(tempf.Name())
	defer tempf.Close()

	// TODO? Fix order of modules to be based on package dependency.
	args := append([]string{"-o", tempf.Name()}, files...)
	cmd := exec.Command(llvmlink, args...)
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	module, err := llvm.ParseBitcodeFile(tempf.Name())
	if err != nil {
		return err
	}
	defer module.Dispose()

	f := os.Stdout
	if outfile != "-" {
		f, err = os.Create(outfile)
		if err != nil {
			return err
		}
		defer f.Close()
	}
	return llvm.WriteBitcodeToFile(module, f)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Error: at least one file must be specified\n")
		fmt.Fprintln(os.Stderr)
		usage()
		os.Exit(1)
	}

	err := link(flag.Args())
	if err != nil {
		errorf("link failed: %s\n", err)
	}
}
