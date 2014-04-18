// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Portions (from gotype):
//     Copyright 2011 The Go Authors. All rights reserved.
//     Use of this source code is governed by a BSD-style
//     license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"go/scanner"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-llvm/llgo"
	"github.com/go-llvm/llvm"
)

func report(err error) {
	if list, ok := err.(scanner.ErrorList); ok {
		for _, e := range list {
			fmt.Fprintf(os.Stderr, "%s\n", e)
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "gllgo: error: %s\n", err)
	}
}

func displayVersion() {
	fmt.Printf("llgo version %s (Go %s)\n", llgo.LLGOVersion, runtime.Version())
	fmt.Println()
	os.Exit(0)
}

func initCompiler(opts *driverOptions) (*llgo.Compiler, error) {
	copts := llgo.CompilerOptions{
		TargetTriple:  opts.triple,
		GenerateDebug: opts.generateDebug,
		GccgoPath:     opts.gccgoPath,
		ImportPaths:   opts.importPaths,
	}
	return llgo.NewCompiler(copts)
}

type actionKind int

const (
	actionAssemble = actionKind(iota)
	actionCompile
	actionLink
	actionVersion
)

type action struct {
	kind   actionKind
	inputs []string
}

type driverOptions struct {
	actions []action
	output  string

	gccgoPath     string
	generateDebug bool
	importPaths   []string
	lto           bool
	optLevel      int
	pic           bool
	pkgpath       string
	sizeLevel     int
	staticLink    bool
	triple        string
}

func parseArguments(args []string) (opts driverOptions, err error) {
	var goInputs, otherInputs []string
	actionKind := actionLink
	opts.gccgoPath = "gccgo"
	opts.triple = llvm.DefaultTargetTriple()

	for len(args) > 0 {
		consumedArgs := 1

		switch {
		case !strings.HasPrefix(args[0], "-"):
			if strings.HasSuffix(args[0], ".go") {
				goInputs = append(goInputs, args[0])
			} else {
				otherInputs = append(otherInputs, args[0])
			}

		case strings.HasPrefix(args[0], "-Wl,"):
			// TODO(pcc): Handle these correctly.
			otherInputs = append(otherInputs, args[0])

		case args[0] == "-I":
			if len(args) == 1 {
				return opts, errors.New("missing path after '-I'")
			}
			opts.importPaths = append(opts.importPaths, args[1])
			consumedArgs = 2

		case strings.HasPrefix(args[0], "-I"):
			opts.importPaths = append(opts.importPaths, args[0][2:])

		case args[0] == "-O0":
			opts.optLevel = 0

		case args[0] == "-O1", args[0] == "-O":
			opts.optLevel = 1

		case args[0] == "-O2":
			opts.optLevel = 2

		case args[0] == "-Os":
			opts.optLevel = 2
			opts.sizeLevel = 1

		case args[0] == "-O3":
			opts.optLevel = 3

		case args[0] == "-S":
			actionKind = actionAssemble

		case args[0] == "-c":
			actionKind = actionCompile

		case strings.HasPrefix(args[0], "-fgccgo-path="):
			opts.gccgoPath = args[0][13:]

		case strings.HasPrefix(args[0], "-fgo-pkgpath="):
			opts.pkgpath = args[0][13:]

		case strings.HasPrefix(args[0], "-fgo-relative-import-path="):
			// TODO(pcc): Handle this.

		case args[0] == "-emit-llvm", args[0] == "-flto":
			opts.lto = true

		case args[0] == "-fPIC":
			opts.pic = true

		case args[0] == "-g":
			// TODO(pcc): Turn this on once we start generating debug info
			// in the current format, as the current output crashes the code
			// generator. Presumably we didn't observe this before because
			// we were serializing/deserializing the IR, which causes old
			// debug info to be dropped.

		case strings.HasPrefix(args[0], "-m"), args[0] == "-funsafe-math-optimizations":
			// TODO(pcc): Handle code generation options.

		case args[0] == "-o":
			if len(args) == 1 {
				return opts, errors.New("missing path after '-o'")
			}
			opts.output = args[1]
			consumedArgs = 2

		case args[0] == "-static":
			opts.staticLink = true

		case args[0] == "--version":
			actionKind = actionVersion

		default:
			return opts, fmt.Errorf("unrecognized command line option '%s'", args[0])
		}

		args = args[consumedArgs:]
	}

	if actionKind != actionVersion && len(goInputs) == 0 && len(otherInputs) == 0 {
		return opts, errors.New("no input files")
	}

	switch actionKind {
	case actionLink:
		if len(goInputs) != 0 {
			opts.actions = []action{action{actionCompile, goInputs}}
		}
		opts.actions = append(opts.actions, action{actionLink, otherInputs})

	case actionCompile, actionAssemble:
		if len(goInputs) != 0 {
			opts.actions = []action{action{actionKind, goInputs}}
		}

	case actionVersion:
		opts.actions = []action{action{actionVersion, nil}}
	}

	if opts.output == "" && len(opts.actions) != 0 {
		switch actionKind {
		case actionCompile, actionAssemble:
			base := filepath.Base(goInputs[0])
			base = base[0 : len(base)-3]
			if actionKind == actionCompile {
				opts.output = base + ".o"
			} else {
				opts.output = base + ".s"
			}

		case actionLink:
			opts.output = "a.out"
		}
	}

	return opts, nil
}

func runPasses(opts *driverOptions, m llvm.Module) {
	fpm := llvm.NewFunctionPassManagerForModule(m)
	defer fpm.Dispose()

	mpm := llvm.NewPassManager()
	defer mpm.Dispose()

	pmb := llvm.NewPassManagerBuilder()
	defer pmb.Dispose()

	pmb.SetOptLevel(opts.optLevel)
	pmb.SetSizeLevel(opts.sizeLevel)

	pmb.Populate(mpm)
	pmb.PopulateFunc(mpm)

	fpm.InitializeFunc()
	for fn := m.FirstFunction(); !fn.IsNil(); fn = llvm.NextFunction(fn) {
		fpm.RunFunc(fn)
	}
	fpm.FinalizeFunc()

	mpm.Run(m)
}

func performAction(opts *driverOptions, kind actionKind, inputs []string, output string) error {
	switch kind {
	case actionVersion:
		displayVersion()
		return nil

	case actionCompile, actionAssemble:
		compiler, err := initCompiler(opts)
		if err != nil {
			return err
		}

		module, err := compiler.Compile(inputs, opts.pkgpath)
		if err != nil {
			return err
		}

		defer module.Dispose()

		// TODO(pcc): Decide how we want to expose export data for LTO.
		if !opts.lto && module.ExportData != nil {
			edatainit := llvm.ConstString(string(module.ExportData), false)
			edataglobal := llvm.AddGlobal(module.Module, edatainit.Type(), module.Path+".export")
			edataglobal.SetInitializer(edatainit)
			edataglobal.SetSection(".go_export")
		}

		runPasses(opts, module.Module)

		var file *os.File
		if output == "-" {
			file = os.Stdout
		} else {
			file, err = os.Create(output)
			if err != nil {
				return err
			}
			defer file.Close()
		}

		switch {
		case !opts.lto:
			target, err := llvm.GetTargetFromTriple(opts.triple)
			if err != nil {
				return err
			}

			optLevel := [...]llvm.CodeGenOptLevel{
				llvm.CodeGenLevelNone,
				llvm.CodeGenLevelLess,
				llvm.CodeGenLevelDefault,
				llvm.CodeGenLevelAggressive,
			}[opts.optLevel]

			relocMode := llvm.RelocStatic
			if opts.pic {
				relocMode = llvm.RelocPIC
			}

			tm := target.CreateTargetMachine(opts.triple, "", "", optLevel,
				relocMode, llvm.CodeModelDefault)
			defer tm.Dispose()

			fileType := llvm.AssemblyFile
			if kind == actionCompile {
				fileType = llvm.ObjectFile
			}
			mb, err := tm.EmitToMemoryBuffer(module.Module, fileType)
			if err != nil {
				return err
			}
			defer mb.Dispose()

			bytes := mb.Bytes()
			_, err = file.Write(bytes)
			return err

		case kind == actionCompile:
			err := llvm.WriteBitcodeToFile(module.Module, file)
			return err

		case kind == actionAssemble:
			_, err := file.WriteString(module.Module.String())
			return err

		default:
			panic("unexpected action kind")
		}

	case actionLink:
		// TODO(pcc): Teach this to link without depending on gccgo, and to do LTO.
		args := []string{"-o", output}
		if opts.pic {
			args = append(args, "-fPIC")
		}
		if opts.staticLink {
			args = append(args, "-static")
		}
		args = append(args, inputs...)

		cmd := exec.Command(opts.gccgoPath, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			os.Stderr.Write(out)
		}
		return err

	default:
		panic("unexpected action kind")
	}
}

func performActions(opts *driverOptions) error {
	var extraInput string

	for i, action := range opts.actions {
		var output string
		if i == len(opts.actions)-1 {
			output = opts.output
		} else {
			tmpfile, err := ioutil.TempFile("", "llgo")
			if err != nil {
				return err
			}
			output = tmpfile.Name() + ".o"
			tmpfile.Close()
			err = os.Remove(tmpfile.Name())
			if err != nil {
				return err
			}
			defer os.Remove(output)
		}

		inputs := action.inputs
		if extraInput != "" {
			inputs = append([]string{extraInput}, inputs...)
		}

		err := performAction(opts, action.kind, inputs, output)
		if err != nil {
			return err
		}

		extraInput = output
	}

	return nil
}

func main() {
	llvm.InitializeAllTargets()
	llvm.InitializeAllTargetMCs()
	llvm.InitializeAllTargetInfos()
	llvm.InitializeAllAsmParsers()
	llvm.InitializeAllAsmPrinters()

	opts, err := parseArguments(os.Args[1:])
	if err != nil {
		report(err)
		os.Exit(1)
	}

	err = performActions(&opts)
	if err != nil {
		report(err)
		os.Exit(1)
	}
}
