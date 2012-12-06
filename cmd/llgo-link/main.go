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
	flag.StringVar(&llvmlink, "llvm-link", "llvm-link", "Path to llvm-link binary")
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

func reorderGlobalConstructors(m llvm.Module) error {
	ctors := m.NamedGlobal("llvm.global_ctors")
	if ctors.IsNil() {
		// No global constructors.
		return nil
	}

	init := ctors.Initializer()
	arraylength := init.Type().ArrayLength()
	zeroindex := []uint32{0}

	// The constructors are ordered within each package, but the packages
	// are in reverse order. We must go backwards through the constructors,
	// reassigning priorities.
	ceiling, npackagectors := -1, -1
	for i := arraylength - 1; i >= 0; i-- {
		indices := []uint32{uint32(i)}
		ctor := llvm.ConstExtractValue(init, indices)
		priority := int(llvm.ConstExtractValue(ctor, zeroindex).ZExtValue())
		if npackagectors == -1 {
			ceiling = arraylength - (i + 1) + priority
			npackagectors = priority
		}
		newpriority := ceiling - (npackagectors - priority)
		newpriorityvalue := llvm.ConstInt(llvm.Int32Type(), uint64(newpriority), false)
		ctor = llvm.ConstInsertValue(ctor, newpriorityvalue, zeroindex)
		if priority == 1 {
			npackagectors = -1
		}
		init = llvm.ConstInsertValue(init, ctor, indices)
	}
	ctors.SetInitializer(init)
	return nil
}

func link(files []string) error {
	tempf, err := ioutil.TempFile("", "llgolink")
	if err != nil {
		return err
	}
	defer os.Remove(tempf.Name())
	defer tempf.Close()

	args := append([]string{"-o", tempf.Name()}, files...)
	err = exec.Command(llvmlink, args...).Run()
	if err != nil {
		return err
	}

	module, err := llvm.ParseBitcodeFile(tempf.Name())
	if err != nil {
		return err
	}
	defer module.Dispose()

	err = reorderGlobalConstructors(module)
	if err != nil {
		return err
	}

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
