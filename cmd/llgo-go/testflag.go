// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The flag handling part of go test is large and distracting.
// We can't use the flag package because some of the flags from
// our command line are for us, and some are for 6.out, and
// some are for both.

var usageMessage = `Usage of go test:
  -c=false: compile but do not run the test binary
  -file=file_test.go: specify file to use for tests;
      use multiple times for multiple files
  -p=n: build and test up to n packages in parallel
  -x=false: print command lines as they are executed

  // These flags can be passed with or without a "test." prefix: -v or -test.v.
  -bench="": passes -test.bench to test
  -benchmem=false: print memory allocation statistics for benchmarks
  -benchtime=1s: passes -test.benchtime to test
  -cpu="": passes -test.cpu to test
  -cpuprofile="": passes -test.cpuprofile to test
  -memprofile="": passes -test.memprofile to test
  -memprofilerate=0: passes -test.memprofilerate to test
  -blockprofile="": pases -test.blockprofile to test
  -blockprofilerate=0: passes -test.blockprofilerate to test
  -parallel=0: passes -test.parallel to test
  -run="": passes -test.run to test
  -short=false: passes -test.short to test
  -timeout=0: passes -test.timeout to test
  -v=false: passes -test.v to test
`

// usage prints a usage message and exits.
func testUsage() {
	fmt.Fprint(os.Stderr, usageMessage)
	setExitStatus(2)
	exit()
}

// testFlagSpec defines a flag we know about.
type testFlagSpec struct {
	name       string
	boolVar    *bool
	passToTest bool // pass to Test
	multiOK    bool // OK to have multiple instances
	present    bool // flag has been seen
}

// testFlagDefn is the set of flags we process.
var testFlagDefn = []*testFlagSpec{
	// local.
	{name: "c", boolVar: &testC},
	{name: "file", multiOK: true},
	{name: "i", boolVar: &testI},

	// build flags.
	{name: "a", boolVar: &buildA},
	{name: "n", boolVar: &buildN},
	{name: "p"},
	{name: "x", boolVar: &buildX},
	{name: "work", boolVar: &buildWork},
	{name: "gcflags"},
	{name: "ldflags"},
	{name: "gccgoflags"},
	{name: "tags"},
	{name: "compiler"},
	{name: "race", boolVar: &buildRace},

	// passed to 6.out, adding a "test." prefix to the name if necessary: -v becomes -test.v.
	{name: "bench", passToTest: true},
	{name: "benchmem", boolVar: new(bool), passToTest: true},
	{name: "benchtime", passToTest: true},
	{name: "cpu", passToTest: true},
	{name: "cpuprofile", passToTest: true},
	{name: "memprofile", passToTest: true},
	{name: "memprofilerate", passToTest: true},
	{name: "blockprofile", passToTest: true},
	{name: "blockprofilerate", passToTest: true},
	{name: "parallel", passToTest: true},
	{name: "run", passToTest: true},
	{name: "short", boolVar: new(bool), passToTest: true},
	{name: "timeout", passToTest: true},
	{name: "v", boolVar: &testV, passToTest: true},
}

// testFlags processes the command line, grabbing -x and -c, rewriting known flags
// to have "test" before them, and reading the command line for the 6.out.
// Unfortunately for us, we need to do our own flag processing because go test
// grabs some flags but otherwise its command line is just a holding place for
// pkg.test's arguments.
// We allow known flags both before and after the package name list,
// to allow both
//	go test fmt -custom-flag-for-fmt-test
//	go test -x math
func testFlags(args []string) (packageNames, passToTest []string) {
	inPkg := false
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			if !inPkg && packageNames == nil {
				// First package name we've seen.
				inPkg = true
			}
			if inPkg {
				packageNames = append(packageNames, args[i])
				continue
			}
		}

		if inPkg {
			// Found an argument beginning with "-"; end of package list.
			inPkg = false
		}

		f, value, extraWord := testFlag(args, i)
		if f == nil {
			// This is a flag we do not know; we must assume
			// that any args we see after this might be flag
			// arguments, not package names.
			inPkg = false
			if packageNames == nil {
				// make non-nil: we have seen the empty package list
				packageNames = []string{}
			}
			passToTest = append(passToTest, args[i])
			continue
		}
		var err error
		switch f.name {
		// bool flags.
		case "a", "c", "i", "n", "x", "v", "work", "race":
			setBoolFlag(f.boolVar, value)
		case "p":
			setIntFlag(&buildP, value)
		case "gcflags":
			buildGcflags, err = splitQuotedFields(value)
			if err != nil {
				fatalf("invalid flag argument for -%s: %v", f.name, err)
			}
		case "ldflags":
			buildLdflags, err = splitQuotedFields(value)
			if err != nil {
				fatalf("invalid flag argument for -%s: %v", f.name, err)
			}
		case "gccgoflags":
			buildGccgoflags, err = splitQuotedFields(value)
			if err != nil {
				fatalf("invalid flag argument for -%s: %v", f.name, err)
			}
		case "tags":
			buildContext.BuildTags = strings.Fields(value)
		case "compiler":
			buildCompiler{}.Set(value)
		case "file":
			testFiles = append(testFiles, value)
		case "bench":
			// record that we saw the flag; don't care about the value
			testBench = true
		case "timeout":
			testTimeout = value
		case "blockprofile", "cpuprofile", "memprofile":
			testProfile = true
		}
		if extraWord {
			i++
		}
		if f.passToTest {
			passToTest = append(passToTest, "-test."+f.name+"="+value)
		}
	}
	return
}

// testFlag sees if argument i is a known flag and returns its definition, value, and whether it consumed an extra word.
func testFlag(args []string, i int) (f *testFlagSpec, value string, extra bool) {
	arg := args[i]
	if strings.HasPrefix(arg, "--") { // reduce two minuses to one
		arg = arg[1:]
	}
	switch arg {
	case "-?", "-h", "-help":
		usage()
	}
	if arg == "" || arg[0] != '-' {
		return
	}
	name := arg[1:]
	// If there's already "test.", drop it for now.
	name = strings.TrimPrefix(name, "test.")
	equals := strings.Index(name, "=")
	if equals >= 0 {
		value = name[equals+1:]
		name = name[:equals]
	}
	for _, f = range testFlagDefn {
		if name == f.name {
			// Booleans are special because they have modes -x, -x=true, -x=false.
			if f.boolVar != nil {
				if equals < 0 { // otherwise, it's been set and will be verified in setBoolFlag
					value = "true"
				} else {
					// verify it parses
					setBoolFlag(new(bool), value)
				}
			} else { // Non-booleans must have a value.
				extra = equals < 0
				if extra {
					if i+1 >= len(args) {
						usage()
					}
					value = args[i+1]
				}
			}
			if f.present && !f.multiOK {
				usage()
			}
			f.present = true
			return
		}
	}
	f = nil
	return
}

// setBoolFlag sets the addressed boolean to the value.
func setBoolFlag(flag *bool, value string) {
	x, err := strconv.ParseBool(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go test: illegal bool flag value %s\n", value)
		usage()
	}
	*flag = x
}

// setIntFlag sets the addressed integer to the value.
func setIntFlag(flag *int, value string) {
	x, err := strconv.Atoi(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go test: illegal int flag value %s\n", value)
		usage()
	}
	*flag = x
}
