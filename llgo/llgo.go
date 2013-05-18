// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Portions (from gotype):
//     Copyright 2011 The Go Authors. All rights reserved.
//     Use of this source code is governed by a BSD-style
//     license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
)

var dump = flag.Bool(
	"dump", false,
	"Dump the LLVM assembly to stderr and exit")

var trace = flag.Bool(
	"trace", false,
	"Trace the compilation process")

var importpath = flag.String(
	"importpath", "",
	"Package import path of the source being compiled (empty means the same as package name)")

var version = flag.Bool(
	"version", false,
	"Display version information and exit")

var os_ = flag.String("os", runtime.GOOS, "Set the target OS")
var arch = flag.String("arch", runtime.GOARCH, "Set the target architecture")
var triple = flag.String("triple", "", "Set the target triple")
var printTriple = flag.Bool("print-triple", false, "Print out target triple and exit")
var compileOnly = flag.Bool("c", false, "Compile only, don't link")
var outputFile = flag.String("o", "-", "Output filename")

var exitCode = 0

func report(err error) {
	scanner.PrintError(os.Stderr, err)
	exitCode = 2
}

func parseFile(fset *token.FileSet, filename string) *ast.File {
	// parse entire file
	mode := parser.DeclarationErrors | parser.ParseComments
	//if *allErrors {
	//    mode |= parser.SpuriousErrors
	//}
	//if *printTrace {
	//    mode |= parser.Trace
	//}
	file, err := parser.ParseFile(fset, filename, nil, mode)
	if err != nil {
		report(err)
		return nil
	}
	return file
}

func parseFiles(fset *token.FileSet, filenames []string) (files []*ast.File) {
	sort.Strings(filenames)
	for i, filename := range filenames {
		if i > 0 && filenames[i-1] == filename {
			report(errors.New(fmt.Sprintf("%q: duplicate file", filename)))
		} else {
			file := parseFile(fset, filename)
			if file != nil {
				files = append(files, file)
			}
		}
	}
	return
}

func isGoFilename(filename string) bool {
	// ignore non-Go files
	return !strings.HasPrefix(filename, ".") &&
		strings.HasSuffix(filename, ".go")
}

func compileFiles(compiler llgo.Compiler, filenames []string, importpath string) (*llgo.Module, error) {
	i := 0
	for _, filename := range filenames {
		switch _, err := os.Stat(filename); {
		case err != nil:
			report(err)
		default:
			filenames[i] = filename
			i++
		}
	}
	if i == 0 {
		return nil, errors.New("No Go source files were specified")
	}
	fset := token.NewFileSet()
	files := parseFiles(fset, filenames[0:i])
	return compiler.Compile(fset, files, importpath)
}

func writeObjectFile(m *llgo.Module) error {
	var outfile *os.File
	switch *outputFile {
	case "-":
		outfile = os.Stdout
	default:
		var err error
		outfile, err = os.Create(*outputFile)
		if err != nil {
			return err
		}
	}
	err := llvm.VerifyModule(m.Module, llvm.ReturnStatusAction)
	if err != nil {
		return fmt.Errorf("Verification failed: %v", err)
	}
	return llvm.WriteBitcodeToFile(m.Module, outfile)
}

func displayVersion() {
	fmt.Printf("llgo version %s (Go %s)\n", llgo.LLGOVersion, runtime.Version())
	fmt.Println()

	fmt.Println("  Available targets:")
	longestTargetName := 0
	targetDescriptions := make(map[string]string)
	targetNames := make([]string, 0)
	for target := llvm.FirstTarget(); target.C != nil; target = target.NextTarget() {
		targetName := target.Name()
		targetNames = append(targetNames, targetName)
		targetDescriptions[targetName] = target.Description()
		if len(targetName) > longestTargetName {
			longestTargetName = len(targetName)
		}
	}
	sort.Strings(targetNames)
	for _, targetName := range targetNames {
		var paddingLen int = longestTargetName - len(targetName)
		fmt.Printf("    %s %*s %s\n", targetName, paddingLen+1, "-",
			targetDescriptions[targetName])
	}
	fmt.Println()

	os.Exit(0)
}

// Convert the architecture name to the string used in LLVM triples.
// See: llvm::Triple::getArchTypeName.
//
// TODO move this into the LLVM C API.
func getTripleArchName(llvmArch string) string {
	switch llvmArch {
	case "x86":
		return "i386"
	case "x86-64":
		return "x86_64"
	case "ppc32":
		return "powerpc"
	case "ppc64":
		return "powerpc64"
	}
	return llvmArch
}

var tripleArchOsError = errors.New("-triple must not be specified as well as -os/-arch")

func computeTriple() string {
	if *triple != "" {
		// Ensure os/arch aren't specified if triple/ is specified.
		//
		// This is an ugly way of telling whether or not -os or -arch were
		// specified. We can't just check the value, as it will have a default.
		archFlag := flag.Lookup("arch")
		osFlag := flag.Lookup("os")
		flag.Visit(func(f *flag.Flag) {
			switch f {
			case archFlag, osFlag:
				fmt.Fprintln(os.Stderr, tripleArchOsError)
				os.Exit(1)
			}
		})
		return *triple
	}

	// -arch is either an architecture name recognised by
	// the gc compiler, or an LLVM architecture name.
	targetArch := *arch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	switch targetArch {
	case "386":
		targetArch = "x86"
	case "amd64", "x86_64":
		targetArch = "x86-64"
	}

	// -os is either an OS name recognised by the gc
	// compiler, or an LLVM OS name.
	targetOS := *os_
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	switch targetOS {
	case "windows":
		targetOS = "win32"
	}

	tripleArch := getTripleArchName(targetArch)
	return fmt.Sprintf("%s-unknown-%s", tripleArch, targetOS)
}

func initCompiler() (llgo.Compiler, error) {
	opts := llgo.CompilerOptions{TargetTriple: computeTriple()}
	if *trace {
		opts.Logger = log.New(os.Stderr, "", 0)
	}
	return llgo.NewCompiler(opts)
}

func main() {
	llvm.InitializeAllTargets()
	llvm.InitializeAllTargetMCs()
	llvm.InitializeAllTargetInfos()
	flag.Parse()

	if *version {
		displayVersion()
	}

	if *printTriple {
		fmt.Println(computeTriple())
		os.Exit(0)
	}

	opts := llgo.CompilerOptions{}
	opts.TargetTriple = computeTriple()
	if *trace {
		opts.Logger = log.New(os.Stderr, "", 0)
	}

	compiler, err := initCompiler()
	if err != nil {
		fmt.Fprintf(os.Stderr, "initCompiler failed: %s\n", err)
		os.Exit(1)
	}
	defer compiler.Dispose()

	module, err := compileFiles(compiler, flag.Args(), *importpath)
	if err == nil {
		defer module.Dispose()
		if exitCode == 0 {
			if *dump {
				module.Dump()
			} else {
				err := writeObjectFile(module)
				if err != nil {
					fmt.Println(err)
				}
			}
		}
	} else {
		report(err)
	}
	os.Exit(exitCode)
}
