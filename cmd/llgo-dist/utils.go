// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"regexp"
	"strings"
)

var (
	GOARCH, GOOS string
)

// initGOVARS initilizes GOARCH and GOOS from the given triple.
func initGOVARS(triple string) error {
	type REs struct{ re, out string }
	goarchREs := []REs{
		{"x86_64", "amd64"},
		{"i?86", "386"},
		{"arm.*", "arm"},
	}
	goosREs := []REs{
		{"linux", "linux"},
		{"darwin.*", "darwin"},
		{"macosx.*", "darwin"},
	}
	match := func(list []REs, s string) string {
		for _, t := range list {
			if matched, _ := regexp.MatchString(t.re, s); matched {
				return t.out
			}
		}
		return ""
	}

	s := strings.Split(triple, "-")
	switch l := len(s); l {
	default:
		return errors.New("triple should be made up of 2 or 3 parts.")
	case 2, 3:
		GOARCH = s[0]
		GOOS = s[l-1]
	}
	GOARCH = match(goarchREs, GOARCH)
	GOOS = match(goosREs, GOOS)

	if GOARCH == "" {
		return errors.New("unknown architecture in triple")
	}
	if GOOS == "" {
		return errors.New("unknown OS in triple")
	}
	return nil
}

// goodOSArchFile returns false if the name contains a $GOOS or $GOARCH
// suffix which does not match the current system.
// The recognized name formats are:
//
//     name_$(GOOS).*
//     name_$(GOARCH).*
//     name_$(GOOS)_$(GOARCH).*
//     name_$(GOOS)_test.*
//     name_$(GOARCH)_test.*
//     name_$(GOOS)_$(GOARCH)_test.*
//
// Adapted from src/pkg/go/build/build.go
func goodOSArchFile(name string) bool {
	if dot := strings.Index(name, "."); dot != -1 {
		name = name[:dot]
	}
	l := strings.Split(name, "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)
	if n >= 2 && knownOS[l[n-2]] && knownArch[l[n-1]] {
		return l[n-2] == GOOS && l[n-1] == GOARCH
	}
	if n >= 1 && knownOS[l[n-1]] {
		return l[n-1] == GOOS
	}
	if n >= 1 && knownArch[l[n-1]] {
		return l[n-1] == GOARCH
	}
	return true
}

var knownOS = make(map[string]bool)
var knownArch = make(map[string]bool)

func init() {
	for _, v := range strings.Fields(goosList) {
		knownOS[v] = true
	}
	for _, v := range strings.Fields(goarchList) {
		knownArch[v] = true
	}
}

const goosList = "darwin freebsd linux netbsd openbsd plan9 windows "
const goarchList = "386 amd64 arm "
