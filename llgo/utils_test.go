package main

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"unsafe"
)

func testdata(files ...string) []string {
	for i, f := range files {
		files[i] = "testdata/" + f
	}
	return files
}

func init() {
	llvm.LinkInJIT()
	llvm.InitializeNativeTarget()
}

func readPipe(p int, c chan<- string) {
	var s string
	buf := make([]byte, 4096)
	n, _ := syscall.Read(p, buf)
	for n > 0 {
		s += string(buf[:n])
		n, _ = syscall.Read(p, buf)
	}
	c <- s
}

func addExterns(m *llgo.Module) {
	CharPtr := llvm.PointerType(llvm.Int8Type(), 0)
	fn_type := llvm.FunctionType(
		llvm.Int32Type(), []llvm.Type{CharPtr}, false)
	fflush := llvm.AddFunction(m.Module, "fflush", fn_type)
	fflush.SetFunctionCallConv(llvm.CCallConv)
}

func runFunction(m *llgo.Module, name string) (output []string, err error) {
	addExterns(m)
	engine, err := llvm.NewJITCompiler(m.Module, 0)
	if err != nil {
		return
	}
	defer engine.Dispose()

	fn := engine.FindFunction(name)
	if fn.IsNil() {
		err = fmt.Errorf("Couldn't find function '%s'", name)
		return
	}

	// Redirect stdout to a pipe.
	pipe_fds := make([]int, 2)
	err = syscall.Pipe(pipe_fds)
	if err != nil {
		return
	}
	defer syscall.Close(pipe_fds[0])
	defer syscall.Close(pipe_fds[1])
	old_stdout, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return
	}
	defer syscall.Close(old_stdout)
	err = syscall.Dup2(pipe_fds[1], syscall.Stdout)
	if err != nil {
		return
	}
	defer syscall.Dup2(old_stdout, syscall.Stdout)

	c := make(chan string)
	go readPipe(pipe_fds[0], c)

	exec_args := []llvm.GenericValue{}
	engine.RunStaticConstructors()
	engine.RunFunction(fn, exec_args)
	defer engine.RunStaticDestructors()

	// Call fflush to flush stdio (printf), then sync and close the write
	// end of the pipe.
	fflush := engine.FindFunction("fflush")
	ptr0 := unsafe.Pointer(uintptr(0))
	exec_args = []llvm.GenericValue{llvm.NewGenericValueFromPointer(ptr0)}
	engine.RunFunction(fflush, exec_args)
	syscall.Fsync(pipe_fds[1])
	syscall.Close(pipe_fds[1])
	syscall.Close(syscall.Stdout)

	output_str := <-c
	output = strings.Split(strings.TrimSpace(output_str), "\n")
	return
}

func checkStringsEqual(out, expectedOut []string) error {
	if !reflect.DeepEqual(out, expectedOut) {
		return fmt.Errorf("Output did not match: %q (actual) != %q (expected)",
			out, expectedOut)
	}
	return nil
}

func runAndCheckMain(files []string, check func(a, b []string) error) error {
	// First run with "go run" to get the expected output.
	cmd := exec.Command("go", append([]string{"run"}, files...)...)
	gorun_out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	expected := strings.Split(strings.TrimSpace(string(gorun_out)), "\n")

	// Now compile to and interpret the LLVM bitcode, comparing the output to
	// the output of "go run" above.
	m, err := compileFiles(files)
	if err != nil {
		return err
	}
	output, err := runFunction(m, "main")
	if err == nil {
		err = check(output, expected)
	}
	return err
}

// vim: set ft=go:
