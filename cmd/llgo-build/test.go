package main

import (
	"bytes"
	"fmt"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func buildPackageTests(pkgpath string) error {
	if strings.HasSuffix(pkgpath, ".go") {
		return fmt.Errorf("-test needs a package path, not a .go file")
	}
	return buildPackages([]string{pkgpath})
}

func writeTestMain(pkg *build.Package, linkfile string) (err error) {
	w := bytes.NewBuffer(nil)
	tests := bytes.NewBuffer(nil)
	fset := token.NewFileSet()

	w.WriteString(`package main

import (
	"testing"
`)

	tests.WriteString("var tests = []testing.InternalTest{\n")

	imports := make(map[string]bool)

	addtests := func(pkgname, pkgpath string, gofiles []string) error {
		for _, tf := range gofiles {
			f, err := parser.ParseFile(fset, tf, nil, 0)
			if err != nil {
				return err
			}
			for k := range f.Scope.Objects {
				if strings.HasPrefix(k, "Test") {
					if !imports[pkgpath] {
						w.WriteString(fmt.Sprintf("\t\"%s\"\n", pkgpath))
						imports[pkgpath] = true
					}
					tests.WriteString(fmt.Sprintf("\t{\"%s\", %s.%s},\n", k, pkgname, k))
				}
			}
		}
		return nil
	}
	if err = addtests(pkg.Name, pkg.ImportPath, pkg.TestGoFiles); err != nil {
		return err
	}
	if err = addtests(pkg.Name+"_test", pkg.ImportPath+"_test", pkg.XTestGoFiles); err != nil {
		return err
	}
	w.WriteString(")\n")
	tests.WriteString("}\n")
	w.Write(tests.Bytes())
	w.WriteString(`

var benchmarks = []testing.InternalBenchmark{}
var examples = []testing.InternalExample{}

func matchString(pat, str string) (bool, error) {
	return true, nil
}

func main() {
	testing.Main(matchString, tests, benchmarks, examples)
}

`)
	data, err := format.Source(w.Bytes())
	if err != nil {
		return err
	}
	gofile := filepath.Join(filepath.Dir(linkfile), "main.go")
	if err = ioutil.WriteFile(gofile, data, 0644); err != nil {
		return err
	}

	test_bc := ""
	if len(pkg.XTestGoFiles) > 0 {
		args2 := []string{"-c", "-triple", triple, "-importpath", pkg.ImportPath + "_test"}
		file := filepath.Base(linkfile)
		file = file[:len(file)-3]
		test_bc = filepath.Join(workdir, file+"_test.bc")
		args2 = append(args2, "-o", test_bc)
		args2 = append(args2, pkg.XTestGoFiles...)
		cmd := exec.Command("llgo", args2...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := runCmd(cmd)
		if err != nil {
			return err
		}
	}

	mainbc := gofile[:len(gofile)-3] + ".bc"
	args := []string{"-c", "-triple", triple, "-o", mainbc, gofile}
	cmd := exec.Command("llgo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = runCmd(cmd)
	if err != nil {
		return err
	}
	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	args = []string{"-o", linkfile, linkfile, mainbc}
	if test_bc != "" {
		args = append(args, test_bc)
	}

	cmd = exec.Command(llvmlink, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = runCmd(cmd); err != nil {
		return err
	}

	return nil
}

func linktest(pkg *build.Package, linkfile string) error {
	if err := writeTestMain(pkg, linkfile); err != nil {
		return err
	}
	if err := linkdeps(pkg, linkfile); err != nil {
		return err
	}
	return nil
}
