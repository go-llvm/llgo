package main

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var (
	testCompiler      llgo.Compiler
	tempdir           string
	runtimemodulefile string
)

func testdata(files ...string) []string {
	for i, f := range files {
		files[i] = "testdata/" + f
	}
	return files
}

func init() {
	tempdir = os.Getenv("LLGO_TESTDIR")
	if tempdir != "" {
		llvm.InitializeNativeTarget()
	} else {
		// Set LLGO_TESTDIR to a new temporary directory,
		// then execute the tests in a new process, clean
		// up and remove the temporary directory.
		tempdir, err := ioutil.TempDir("", "llgo")
		if err != nil {
			panic(err)
		}
		os.Setenv("LLGO_TESTDIR", tempdir)
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Some operating systems won't kill the child on Ctrl-C.
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			err = cmd.Run()
			c <- nil
		}()
		if sig := <-c; sig != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}

		os.RemoveAll(tempdir)
		if err != nil {
			panic(err)
		}
		os.Exit(0)
	}
}

func getRuntimeFiles() (gofiles []string, llfiles []string, cfiles []string, err error) {
	var pkg *build.Package
	pkgpath := "github.com/axw/llgo/pkg/runtime"
	pkg, err = build.Import(pkgpath, "", 0)
	if err != nil {
		return
	}
	gofiles = make([]string, len(pkg.GoFiles))
	for i, filename := range pkg.GoFiles {
		gofiles[i] = path.Join(pkg.Dir, filename)
	}
	llfiles, err = filepath.Glob(pkg.Dir + "/*.ll")
	if err != nil {
		gofiles = nil
		return
	}
	cfiles = make([]string, len(pkg.CFiles))
	for i, filename := range pkg.CFiles {
		cfiles[i] = path.Join(pkg.Dir, filename)
	}
	return
}

func getRuntimeModuleFile() (string, error) {
	if runtimemodulefile != "" {
		return runtimemodulefile, nil
	}

	gofiles, llfiles, cfiles, err := getRuntimeFiles()
	if err != nil {
		return "", err
	}

	var runtimeModule *llgo.Module
	runtimeModule, err = compileFiles(testCompiler, gofiles, "runtime")
	defer runtimeModule.Dispose()
	if err != nil {
		return "", err
	}

	outfile := filepath.Join(tempdir, "runtime.bc")
	f, err := os.Create(outfile)
	if err != nil {
		return "", err
	}
	err = llvm.WriteBitcodeToFile(runtimeModule.Module, f)
	if err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	for i, cfile := range cfiles {
		bcfile := filepath.Join(tempdir, fmt.Sprintf("%d.bc", i))
		cmd := exec.Command("clang", "-g", "-c", "-emit-llvm", "-o", bcfile, cfile)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("clang failed: %s", err)
		}
		llfiles = append(llfiles, bcfile)
	}

	if llfiles != nil {
		args := append([]string{"-o", outfile, outfile}, llfiles...)
		cmd := exec.Command("llvm-link", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("llvm-link failed: %s", err)
			fmt.Fprintf(os.Stderr, string(output))
			return "", err
		}
	}

	runtimemodulefile = outfile
	return runtimemodulefile, nil
}

func runMainFunction(m *llgo.Module) (output []string, err error) {
	runtimeModule, err := getRuntimeModuleFile()
	if err != nil {
		return
	}

	err = llvm.VerifyModule(m.Module, llvm.ReturnStatusAction)
	if err != nil {
		err = fmt.Errorf("Verification failed: %v", err)
		return
	}

	bcpath := filepath.Join(tempdir, "test.bc")
	bcfile, err := os.Create(bcpath)
	if err != nil {
		return nil, err
	}
	llvm.WriteBitcodeToFile(m.Module, bcfile)
	bcfile.Close()

	cmd := exec.Command("llvm-link", "-o", bcpath, bcpath, runtimeModule)
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return
	}

	exepath := filepath.Join(tempdir, "test")
	cmd = exec.Command("clang++", "-pthread", "-g", "-o", exepath, bcpath)
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return
	}

	cmd = exec.Command(exepath)
	data, err := cmd.Output()
	if err != nil {
		return
	}
	output = strings.Split(strings.TrimSpace(string(data)), "\n")
	return
}

func checkStringsEqual(out, expectedOut []string) error {
	if !reflect.DeepEqual(out, expectedOut) {
		return fmt.Errorf("Output did not match: %q (actual) != %q (expected)",
			out, expectedOut)
	}
	return nil
}

func checkStringsEqualUnordered(out, expectedOut []string) error {
	outSorted := make([]string, len(out))
	expectedOutSorted := make([]string, len(expectedOut))
	copy(outSorted, out)
	copy(expectedOutSorted, expectedOut)
	sort.Strings(outSorted)
	sort.Strings(expectedOutSorted)
	if !reflect.DeepEqual(outSorted, expectedOutSorted) {
		return fmt.Errorf("Output did not match: %q (actual) != %q (expected)",
			outSorted, expectedOutSorted)
	}
	return nil
}

func runAndCheckMain(check func(a, b []string) error, files []string) error {
	var err error
	testCompiler, err = initCompiler()
	if err != nil {
		return fmt.Errorf("Failed to initialise compiler: %s", err)
	}
	defer testCompiler.Dispose()

	// First run with "go run" to get the expected output.
	cmd := exec.Command("go", append([]string{"run"}, files...)...)
	gorun_out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go run failed: %s", err)
	}
	expected := strings.Split(strings.TrimSpace(string(gorun_out)), "\n")

	// Now compile to and interpret the LLVM bitcode, comparing the output to
	// the output of "go run" above.
	m, err := compileFiles(testCompiler, files, "main")
	if err != nil {
		return err
	}
	output, err := runMainFunction(m)
	if err == nil {
		err = check(output, expected)
	}
	return err
}

// checkOutputEqual compiles and runs the specified files using gc and llgo,
// and checks that their output matches exactly.
func checkOutputEqual(t *testing.T, files ...string) {
	err := runAndCheckMain(checkStringsEqual, testdata(files...))
	if err != nil {
		t.Fatal(err)
	}
}

// checkOutputEqualUnordered compiles and runs the specified files using gc
// and llgo, and checks that their output, when split by line and sorted,
// matches.
func checkOutputEqualUnordered(t *testing.T, files ...string) {
	err := runAndCheckMain(checkStringsEqualUnordered, testdata(files...))
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
