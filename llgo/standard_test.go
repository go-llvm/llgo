// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go_test

// Run runs tests in the test directory.
//
// TODO(bradfitz): docs of some sort, once we figure out how we're changing
// headers of files
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	numParallel    = flag.Int("n", runtime.NumCPU(), "number of parallel tests to run")
	summary        = flag.Bool("summary", false, "show summary of results")
	showSkips      = flag.Bool("show_skips", false, "show skipped tests")
	runoutputLimit = flag.Int("l", defaultRunOutputLimit(), "number of parallel runoutput tests to run")
)

var (
	// gc and ld are [568][gl].
	gc, ld string

	// letter is the build.ArchChar
	letter string

	// dirs are the directories to look for *.go files in.
	// TODO(bradfitz): just use all directories?
	dirs = []string{".", "ken", "chan", "interface", "syntax", "dwarf", "fixedbugs", "bugs"}

	// ratec controls the max number of tests running at a time.
	ratec chan bool

	// toRun is the channel of tests to run.
	// It is nil until the first test is started.
	toRun chan *test

	// rungatec controls the max number of runoutput tests
	// executed in parallel as they can each consume a lot of memory.
	rungatec chan bool
)

var known_llgo_broken = []string{
	// These appear to hang
	"torture.go",
	"bug361.go",
	"append.go",
	"ddd.go",
	"mapnan.go",
}

// maxTests is an upper bound on the total number of tests.
// It is used as a channel buffer size to make sure sends don't block.
const maxTests = 5000

func TestStandardTests(t *testing.T) {
	// Disable parallelism if printing
	if testing.Verbose() {
		*numParallel = 1
	}

	ratec = make(chan bool, *numParallel)
	rungatec = make(chan bool, *runoutputLimit)
	var err error
	letter, err = build.ArchChar(build.Default.GOARCH)
	check(err)
	gc = letter + "g"
	ld = letter + "l"

	for i, dir := range dirs {
		dirs[i] = filepath.Join(runtime.GOROOT(), "test", dir)
	}
	fmt.Println(dirs)

	var tests []*test
	if flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			if arg == "-" || arg == "--" {
				// Permit running:
				// $ go run run.go - env.go
				// $ go run run.go -- env.go
				// $ go run run.go - ./fixedbugs
				// $ go run run.go -- ./fixedbugs
				continue
			}
			if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
				for _, baseGoFile := range goFiles(arg) {
					tests = append(tests, startTest(arg, baseGoFile))
				}
			} else if strings.HasSuffix(arg, ".go") {
				dir, file := filepath.Split(arg)
				tests = append(tests, startTest(dir, file))
			} else {
				log.Fatalf("can't yet deal with non-directory and non-go file %q", arg)
			}
		}
	} else {
		for _, dir := range dirs {
			for _, baseGoFile := range goFiles(dir) {
				tests = append(tests, startTest(dir, baseGoFile))
			}
		}
	}

	failed := false
	resCount := map[string]int{}
	for _, test := range tests {
		<-test.donec
		_, isSkip := test.err.(skipError)
		errStr := "pass"
		if test.err != nil {
			errStr = test.err.Error()
			if !isSkip {
				failed = true
			}
		}
		if isSkip && !skipOkay[path.Join(test.dir, test.gofile)] {
			errStr = "unexpected skip for " + path.Join(test.dir, test.gofile) + ": " + errStr
			isSkip = false
			failed = true
		}
		if failed {
			t.Fail()
		}
		resCount[errStr]++
		if isSkip && !testing.Verbose() && !*showSkips {
			continue
		}
		if !testing.Verbose() && test.err == nil {
			continue
		}
		fmt.Printf("# finished -- %s\n%-20s %-20s: %s\n", path.Join(test.dir, test.gofile), test.action, test.goFileName(), errStr)
	}

	if *summary {
		for k, v := range resCount {
			fmt.Printf("%5d %s\n", v, k)
		}
	}

	if failed {
		os.Exit(1)
	}
}

func toolPath(name string) string {
	p := filepath.Join(os.Getenv("GOROOT"), "bin", "tool", name)
	if _, err := os.Stat(p); err != nil {
		log.Fatalf("didn't find binary at %s", p)
	}
	return p
}

func goFiles(dir string) []string {
	f, err := os.Open(dir)
	check(err)
	dirnames, err := f.Readdirnames(-1)
	check(err)
	names := []string{}
	for _, name := range dirnames {
		if !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".go") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

type runCmd func(...string) ([]byte, error)

func compileFile(runcmd runCmd, longname string) (out []byte, err error) {
	return runcmd("llgo", "-c=true", longname)
}

func compileInDir(runcmd runCmd, dir string, names ...string) (out []byte, err error) {
	cmd := []string{"llgo", "-c=true", "-importpath", dir}
	for _, name := range names {
		cmd = append(cmd, filepath.Join(dir, name))
	}
	return runcmd(cmd...)
}

func linkFile(runcmd runCmd, goname string) (err error) {
	pfile := strings.Replace(goname, ".go", "."+letter, -1)
	_, err = runcmd("go", "tool", ld, "-o", "a.exe", "-L", ".", pfile)
	return
}

// skipError describes why a test was skipped.
type skipError string

func (s skipError) Error() string { return string(s) }

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// test holds the state of a test.
type test struct {
	dir, gofile string
	donec       chan bool // closed when done

	src    string
	action string // "compile", "build", etc.

	tempDir string
	err     error
}

// startTest
func startTest(dir, gofile string) *test {
	t := &test{
		dir:    dir,
		gofile: gofile,
		donec:  make(chan bool, 1),
	}
	if toRun == nil {
		toRun = make(chan *test, maxTests)
		go runTests()
	}
	select {
	case toRun <- t:
	default:
		panic("toRun buffer size (maxTests) is too small")
	}
	return t
}

// runTests runs tests in parallel, but respecting the order they
// were enqueued on the toRun channel.
func runTests() {
	for {
		ratec <- true
		t := <-toRun
		go func() {
			t.run()
			<-ratec
		}()
	}
}

//var cwd, _ = os.Getwd()

func (t *test) goFileName() string {
	return filepath.Join(t.dir, t.gofile)
}

func (t *test) goDirName() string {
	return filepath.Join(t.dir, strings.Replace(t.gofile, ".go", ".dir", -1))
}

func goDirFiles(longdir string) (filter []os.FileInfo, err error) {
	files, dirErr := ioutil.ReadDir(longdir)
	if dirErr != nil {
		return nil, dirErr
	}
	for _, gofile := range files {
		if filepath.Ext(gofile.Name()) == ".go" {
			filter = append(filter, gofile)
		}
	}
	return
}

var packageRE = regexp.MustCompile(`(?m)^package (\w+)`)

func goDirPackages(longdir string) ([][]string, error) {
	files, err := goDirFiles(longdir)
	if err != nil {
		return nil, err
	}
	var pkgs [][]string
	m := make(map[string]int)
	for _, file := range files {
		name := file.Name()
		data, err := ioutil.ReadFile(filepath.Join(longdir, name))
		if err != nil {
			return nil, err
		}
		pkgname := packageRE.FindStringSubmatch(string(data))
		if pkgname == nil {
			return nil, fmt.Errorf("cannot find package name in %s", name)
		}
		i, ok := m[pkgname[1]]
		if !ok {
			i = len(pkgs)
			pkgs = append(pkgs, nil)
			m[pkgname[1]] = i
		}
		pkgs[i] = append(pkgs[i], name)
	}
	return pkgs, nil
}

type context struct {
	GOOS   string
	GOARCH string
}

// shouldTest looks for build tags in a source file and returns
// whether the file should be used according to the tags.
func shouldTest(src string, goos, goarch string) (ok bool, whyNot string) {
	if idx := strings.Index(src, "\npackage"); idx >= 0 {
		src = src[:idx]
	}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") {
			line = line[2:]
		} else {
			continue
		}
		line = strings.TrimSpace(line)
		if len(line) == 0 || line[0] != '+' {
			continue
		}
		ctxt := &context{
			GOOS:   goos,
			GOARCH: goarch,
		}
		words := strings.Fields(line)
		if words[0] == "+build" {
			ok := false
			for _, word := range words[1:] {
				if ctxt.match(word) {
					ok = true
					break
				}
			}
			if !ok {
				// no matching tag found.
				return false, line
			}
		}
	}
	// no build tags
	return true, ""
}

func (ctxt *context) match(name string) bool {
	if name == "" {
		return false
	}
	if i := strings.Index(name, ","); i >= 0 {
		// comma-separated list
		return ctxt.match(name[:i]) && ctxt.match(name[i+1:])
	}
	if strings.HasPrefix(name, "!!") { // bad syntax, reject always
		return false
	}
	if strings.HasPrefix(name, "!") { // negation
		return len(name) > 1 && !ctxt.match(name[1:])
	}

	// Tags must be letters, digits, underscores or dots.
	// Unlike in Go identifiers, all digits are fine (e.g., "386").
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '.' {
			return false
		}
	}

	if name == ctxt.GOOS || name == ctxt.GOARCH {
		return true
	}

	return false
}

func init() { checkShouldTest() }

// run runs a test.
func (t *test) run() {
	if testing.Verbose() {
		fmt.Printf("# entering -- %s/%s\n", t.dir, t.gofile)
	}
	defer close(t.donec)

	for _, broken := range known_llgo_broken {
		if t.gofile == broken {
			t.action = "skip"
			return
		}
	}

	srcBytes, err := ioutil.ReadFile(t.goFileName())
	if err != nil {
		t.err = err
		return
	}
	t.src = string(srcBytes)
	if t.src[0] == '\n' {
		t.err = skipError("starts with newline")
		return
	}
	pos := strings.Index(t.src, "\n\n")
	if pos == -1 {
		t.err = errors.New("double newline not found")
		return
	}
	if ok, why := shouldTest(t.src, runtime.GOOS, runtime.GOARCH); !ok {
		t.action = "skip"
		if *showSkips {
			fmt.Printf("%-20s %-20s: %s\n", t.action, t.goFileName(), why)
		}
		return
	}
	action := t.src[:pos]
	if nl := strings.Index(action, "\n"); nl >= 0 && strings.Contains(action[:nl], "+build") {
		// skip first line
		action = action[nl+1:]
	}
	if strings.HasPrefix(action, "//") {
		action = action[2:]
	}

	var args, flags []string
	wantError := false
	_ = wantError
	f := strings.Fields(action)
	if len(f) > 0 {
		action = f[0]
		args = f[1:]
	}

	switch action {
	case "rundircmpout":
		action = "rundir"
		t.action = "rundir"
	case "cmpout":
		action = "run" // the run case already looks for <dir>/<test>.out files
		fallthrough
	case "compile", "compiledir", "build", "run", "runoutput", "rundir":
		t.action = action
	case "errorcheck", "errorcheckdir", "errorcheckoutput":
		t.action = action
		wantError = true
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			if args[0] == "-0" {
				wantError = false
			} else {
				flags = append(flags, args[0])
			}
			args = args[1:]
		}
	case "skip":
		t.action = "skip"
		return
	default:
		t.err = skipError("skipped; unknown pattern: " + action)
		t.action = "??"
		return
	}

	t.makeTempDir()
	defer os.RemoveAll(t.tempDir)

	err = ioutil.WriteFile(filepath.Join(t.tempDir, t.gofile), srcBytes, 0644)
	check(err)

	// A few tests (of things like the environment) require these to be set.
	os.Setenv("GOOS", runtime.GOOS)
	os.Setenv("GOARCH", runtime.GOARCH)

	useTmp := true
	runcmd := func(args ...string) ([]byte, error) {
		cmd := exec.Command(args[0], args[1:]...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if useTmp {
			cmd.Dir = t.tempDir
			cmd.Env = envForDir(cmd.Dir)
		}
		err := cmd.Run()
		if err != nil {
			err = fmt.Errorf("%s\n%s", err, buf.Bytes())
		}
		return buf.Bytes(), err
	}

	long := t.goFileName()
	switch action {
	default:
		t.err = fmt.Errorf("unimplemented action %q", action)
	case "errorcheck":
		// This one kind of works in that many tests do indeed report errors on the expected error lines, but then
		// it fails because the error message doesn't match what gc is reporting.
		t.err = fmt.Errorf("errorcheck tests disabled")
		/*
			cmdline := []string{"llgo", "-c", "-o", "a." + letter}
			cmdline = append(cmdline, flags...)
			cmdline = append(cmdline, long)
			out, err := runcmd(cmdline...)
			if wantError {
				if err == nil {
					t.err = fmt.Errorf("compilation succeeded unexpectedly\n%s", out)
					return
				}
			} else {
				if err != nil {
					t.err = err
					return
				}
			}
			t.err = t.errorCheck(string(out), long, t.gofile)
			return
		*/
	case "compile":
		_, t.err = compileFile(runcmd, long)
		/*
			case "compiledir":
				// Compile all files in the directory in lexicographic order.
				longdir := t.goDirName()
				pkgs, err := goDirPackages(longdir)
				if err != nil {
					t.err = err
					return
				}
				for _, gofiles := range pkgs {
					_, t.err = compileInDir(runcmd, longdir, gofiles...)
					if t.err != nil {
						return
					}
				}
		*/
		/*
			case "errorcheckdir":
				// errorcheck all files in lexicographic order
				// useful for finding importing errors
				longdir := t.goDirName()
				pkgs, err := goDirPackages(longdir)
				if err != nil {
					t.err = err
					return
				}
				for i, gofiles := range pkgs {
					out, err := compileInDir(runcmd, longdir, gofiles...)
					if i == len(pkgs)-1 {
						if wantError && err == nil {
							t.err = fmt.Errorf("compilation succeeded unexpectedly\n%s", out)
							return
						} else if !wantError && err != nil {
							t.err = err
							return
						}
					} else if err != nil {
						t.err = err
						return
					}
					var fullshort []string
					for _, name := range gofiles {
						fullshort = append(fullshort, filepath.Join(longdir, name), name)
					}
					t.err = t.errorCheck(string(out), fullshort...)
					if t.err != nil {
						break
					}
				}

			case "rundir":
				// Compile all files in the directory in lexicographic order.
				// then link as if the last file is the main package and run it
				longdir := t.goDirName()
				pkgs, err := goDirPackages(longdir)
				if err != nil {
					t.err = err
					return
				}
				for i, gofiles := range pkgs {
					_, err := compileInDir(runcmd, longdir, gofiles...)
					if err != nil {
						t.err = err
						return
					}
					if i == len(pkgs)-1 {
						err = linkFile(runcmd, gofiles[0])
						if err != nil {
							t.err = err
							return
						}
						out, err := runcmd(append([]string{filepath.Join(t.tempDir, "a.exe")}, args...)...)
						if err != nil {
							t.err = err
							return
						}
						if strings.Replace(string(out), "\r\n", "\n", -1) != t.expectedOutput() {
							t.err = fmt.Errorf("incorrect output\n%s", out)
						}
					}
				}
		*/
	case "build":
		_, err := runcmd("llgo-build", "-o", "a.exe", long)
		if err != nil {
			t.err = err
		}
	case "run":
		useTmp = false
		tfile := filepath.Join(t.tempDir, "llgo.tmp")

		_, err := runcmd(append([]string{"llgo-build", "-o", tfile}, t.goFileName())...)
		if err != nil {
			t.err = err
			return
		}
		out, err := runcmd(append([]string{tfile}, args...)...)
		if err != nil {
			t.err = err
			return
		}
		if strings.Replace(string(out), "\r\n", "\n", -1) != t.expectedOutput() {
			t.err = fmt.Errorf("incorrect output\n%s", out)
		}
	case "runoutput":
		rungatec <- true
		defer func() {
			<-rungatec
		}()
		useTmp = false
		tfile := filepath.Join(t.tempDir, "llgo.tmp")
		cmd := append([]string{"llgo-build", "-o", tfile}, t.goFileName())
		log.Println(cmd)
		_, err := runcmd(cmd...)
		if err != nil {
			t.err = err
			return
		}
		out, err := runcmd(append([]string{tfile}, args...)...)
		if err != nil {
			t.err = err
			panic(err)
			return
		}
		tfile2 := filepath.Join(t.tempDir, "tmp__.go")
		if err := ioutil.WriteFile(tfile2, out, 0666); err != nil {
			t.err = fmt.Errorf("write tempfile:%s", err)
			return
		}
		out, err = runcmd("llgo-build", "-o", tfile, tfile2)
		if err != nil {
			t.err = err
			return
		}
		out, err = runcmd(tfile)
		if err != nil {
			t.err = err
			return
		}

		if string(out) != t.expectedOutput() {
			t.err = fmt.Errorf("incorrect output\n%s", out)
		}
		/*
			case "errorcheckoutput":
				useTmp = false
				out, err := runcmd(append([]string{"go", "run", t.goFileName()}, args...)...)
				if err != nil {
					t.err = err
				}
				tfile := filepath.Join(t.tempDir, "tmp__.go")
				err = ioutil.WriteFile(tfile, out, 0666)
				if err != nil {
					t.err = fmt.Errorf("write tempfile:%s", err)
					return
				}
				cmdline := []string{"go", "tool", gc, "-e", "-o", "a." + letter}
				cmdline = append(cmdline, flags...)
				cmdline = append(cmdline, tfile)
				out, err = runcmd(cmdline...)
				if wantError {
					if err == nil {
						t.err = fmt.Errorf("compilation succeeded unexpectedly\n%s", out)
						return
					}
				} else {
					if err != nil {
						t.err = err
						return
					}
				}
				t.err = t.errorCheck(string(out), tfile, "tmp__.go")
				return
		*/
	}
}

func (t *test) String() string {
	return filepath.Join(t.dir, t.gofile)
}

func (t *test) makeTempDir() {
	var err error
	t.tempDir, err = ioutil.TempDir("", "")
	check(err)
}

func (t *test) expectedOutput() string {
	filename := filepath.Join(t.dir, t.gofile)
	filename = filename[:len(filename)-len(".go")]
	filename += ".out"
	b, _ := ioutil.ReadFile(filename)
	return string(b)
}

func (t *test) errorCheck(outStr string, fullshort ...string) (err error) {
	defer func() {
		if testing.Verbose() && err != nil {
			log.Printf("%s gc output:\n%s", t, outStr)
		}
	}()
	var errs []error

	var out []string
	// 6g error messages continue onto additional lines with leading tabs.
	// Split the output at the beginning of each line that doesn't begin with a tab.
	for _, line := range strings.Split(outStr, "\n") {
		if strings.HasSuffix(line, "\r") { // remove '\r', output by compiler on windows
			line = line[:len(line)-1]
		}
		if strings.HasPrefix(line, "\t") {
			out[len(out)-1] += "\n" + line
		} else if strings.HasPrefix(line, "go tool") {
			continue
		} else if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}

	// Cut directory name.
	for i := range out {
		for j := 0; j < len(fullshort); j += 2 {
			full, short := fullshort[j], fullshort[j+1]
			out[i] = strings.Replace(out[i], full, short, -1)
		}
	}

	var want []wantedError
	for j := 0; j < len(fullshort); j += 2 {
		full, short := fullshort[j], fullshort[j+1]
		want = append(want, t.wantedErrors(full, short)...)
	}

	for _, we := range want {
		var errmsgs []string
		errmsgs, out = partitionStrings(we.filterRe, out)
		if len(errmsgs) == 0 {
			errs = append(errs, fmt.Errorf("%s:%d: missing error %q", we.file, we.lineNum, we.reStr))
			continue
		}
		matched := false
		n := len(out)
		for _, errmsg := range errmsgs {
			if we.re.MatchString(errmsg) {
				matched = true
			} else {
				out = append(out, errmsg)
			}
		}
		if !matched {
			errs = append(errs, fmt.Errorf("%s:%d: no match for %#q in:\n\t%s", we.file, we.lineNum, we.reStr, strings.Join(out[n:], "\n\t")))
			continue
		}
	}

	if len(out) > 0 {
		errs = append(errs, fmt.Errorf("Unmatched Errors:"))
		for _, errLine := range out {
			errs = append(errs, fmt.Errorf("%s", errLine))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\n")
	for _, err := range errs {
		fmt.Fprintf(&buf, "%s\n", err.Error())
	}
	return errors.New(buf.String())

}

func partitionStrings(rx *regexp.Regexp, strs []string) (matched, unmatched []string) {
	for _, s := range strs {
		if rx.MatchString(s) {
			matched = append(matched, s)
		} else {
			unmatched = append(unmatched, s)
		}
	}
	return
}

type wantedError struct {
	reStr    string
	re       *regexp.Regexp
	lineNum  int
	file     string
	filterRe *regexp.Regexp // /^file:linenum\b/m
}

var (
	errRx       = regexp.MustCompile(`// (?:GC_)?ERROR (.*)`)
	errQuotesRx = regexp.MustCompile(`"([^"]*)"`)
	lineRx      = regexp.MustCompile(`LINE(([+-])([0-9]+))?`)
)

func (t *test) wantedErrors(file, short string) (errs []wantedError) {
	src, _ := ioutil.ReadFile(file)
	for i, line := range strings.Split(string(src), "\n") {
		lineNum := i + 1
		if strings.Contains(line, "////") {
			// double comment disables ERROR
			continue
		}
		m := errRx.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		all := m[1]
		mm := errQuotesRx.FindAllStringSubmatch(all, -1)
		if mm == nil {
			log.Fatalf("%s:%d: invalid errchk line: %s", t.goFileName(), lineNum, line)
		}
		for _, m := range mm {
			rx := lineRx.ReplaceAllStringFunc(m[1], func(m string) string {
				n := lineNum
				if strings.HasPrefix(m, "LINE+") {
					delta, _ := strconv.Atoi(m[5:])
					n += delta
				} else if strings.HasPrefix(m, "LINE-") {
					delta, _ := strconv.Atoi(m[5:])
					n -= delta
				}
				return fmt.Sprintf("%s:%d", short, n)
			})
			re, err := regexp.Compile(rx)
			if err != nil {
				log.Fatalf("%s:%d: invalid regexp in ERROR line: %v", t.goFileName(), lineNum, err)
			}
			filterPattern := fmt.Sprintf(`^(\w+/)?%s:%d[:[]`, regexp.QuoteMeta(short), lineNum)
			errs = append(errs, wantedError{
				reStr:    rx,
				re:       re,
				filterRe: regexp.MustCompile(filterPattern),
				lineNum:  lineNum,
				file:     short,
			})
		}
	}

	return
}

var skipOkay = map[string]bool{
	"linkx.go":            true, // like "run" but wants linker flags
	"sinit.go":            true,
	"fixedbugs/bug248.go": true, // combines errorcheckdir and rundir in the same dir.
	"fixedbugs/bug302.go": true, // tests both .$O and .a imports.
	"fixedbugs/bug345.go": true, // needs the appropriate flags in gc invocation.
	"fixedbugs/bug369.go": true, // needs compiler flags.
	"fixedbugs/bug429.go": true, // like "run" but program should fail
	"bugs/bug395.go":      true,
}

// defaultRunOutputLimit returns the number of runoutput tests that
// can be executed in parallel.
func defaultRunOutputLimit() int {
	const maxArmCPU = 2

	cpu := runtime.NumCPU()
	if runtime.GOARCH == "arm" && cpu > maxArmCPU {
		cpu = maxArmCPU
	}
	return cpu
}

// checkShouldTest runs sanity checks on the shouldTest function.
func checkShouldTest() {
	assert := func(ok bool, _ string) {
		if !ok {
			panic("fail")
		}
	}
	assertNot := func(ok bool, _ string) { assert(!ok, "") }

	// Simple tests.
	assert(shouldTest("// +build linux", "linux", "arm"))
	assert(shouldTest("// +build !windows", "linux", "arm"))
	assertNot(shouldTest("// +build !windows", "windows", "amd64"))

	// A file with no build tags will always be tested.
	assert(shouldTest("// This is a test.", "os", "arch"))

	// Build tags separated by a space are OR-ed together.
	assertNot(shouldTest("// +build arm 386", "linux", "amd64"))

	// Build tags seperated by a comma are AND-ed together.
	assertNot(shouldTest("// +build !windows,!plan9", "windows", "amd64"))
	assertNot(shouldTest("// +build !windows,!plan9", "plan9", "386"))

	// Build tags on multiple lines are AND-ed together.
	assert(shouldTest("// +build !windows\n// +build amd64", "linux", "amd64"))
	assertNot(shouldTest("// +build !windows\n// +build amd64", "windows", "amd64"))

	// Test that (!a OR !b) matches anything.
	assert(shouldTest("// +build !windows !plan9", "windows", "amd64"))
}

// envForDir returns a copy of the environment
// suitable for running in the given directory.
// The environment is the current process's environment
// but with an updated $PWD, so that an os.Getwd in the
// child will be faster.
func envForDir(dir string) []string {
	env := os.Environ()
	for i, kv := range env {
		if strings.HasPrefix(kv, "PWD=") {
			env[i] = "PWD=" + dir
			return env
		}
	}
	env = append(env, "PWD="+dir)
	return env
}
