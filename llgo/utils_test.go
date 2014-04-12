package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-llvm/llgo"
	"github.com/go-llvm/llvm"
)

var (
	testCompiler      *llgo.Compiler
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

func runMainFunction(files []string, importPath string) (output []string, err error) {
	bcpath := filepath.Join(tempdir, "test.bc")
	args := []string{"-importpath=" + importPath, "-o", bcpath}
	args = append(args, files...)
	cmd := exec.Command("./llgo", args...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		output = strings.Split(strings.TrimSpace(string(data)), "\n")
		return output, fmt.Errorf("llgo failed: %v", err)
	}

	optbcpath := filepath.Join(tempdir, "test.opt.bc")
	cmd = exec.Command("opt", "-O3", "-o", optbcpath, bcpath)
	data, err = cmd.CombinedOutput()
	if err != nil {
		output = strings.Split(strings.TrimSpace(string(data)), "\n")
		return output, fmt.Errorf("opt failed: %v", err)
	}

	objpath := filepath.Join(tempdir, "test.o")
	cmd = exec.Command("llc", "-filetype=obj", "-disable-tail-calls", "-O3", "-o", objpath, optbcpath)
	data, err = cmd.CombinedOutput()
	if err != nil {
		output = strings.Split(strings.TrimSpace(string(data)), "\n")
		return output, fmt.Errorf("llc failed: %v", err)
	}

	exepath := filepath.Join(tempdir, "test")
	cmd = exec.Command("gccgo", "-o", exepath, objpath)
	data, err = cmd.CombinedOutput()
	if err != nil {
		output = strings.Split(strings.TrimSpace(string(data)), "\n")
		return output, fmt.Errorf("gccgo failed: %v", err)
	}

	cmd = exec.Command(exepath)
	data, err = cmd.CombinedOutput()
	output = strings.Split(strings.TrimSpace(string(data)), "\n")
	return output, err
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
	// First run with "go run" to get the expected output.
	cmd := exec.Command("go", append([]string{"run"}, files...)...)
	gorun_out, err := cmd.CombinedOutput()
	expected := strings.Split(strings.TrimSpace(string(gorun_out)), "\n")
	if err != nil {
		return fmt.Errorf("go run failed: \n\t%s\n\t", err, strings.Join(expected, "\n\t"))
	}

	// Now compile to and interpret the LLVM bitcode, comparing the output to
	// the output of "go run" above.
	output, err := runMainFunction(files, "main")
	if err == nil {
		err = check(output, expected)
	} else {
		return fmt.Errorf("runMainFunction failed: \n\t%s\n\t%s", err, strings.Join(output, "\n\t"))
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
