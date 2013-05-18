// Copyright 2012 The llgo Authors.
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
	// reference: http://llvm.org/docs/doxygen/html/Triple_8cpp_source.html
	goarchREs := []REs{
		{"amd64|x86_64", "amd64"},
		{"i[3-9]86", "386"},
		{"xscale|((arm|thumb)(v.*)?)", "arm"},
	}
	goosREs := []REs{
		{"linux.*", "linux"},
		{"(darwin|macosx|ios).*", "darwin"},
		{"k?freebsd.*", "freebsd"},
		{"netbsd.*", "netbsd"},
		{"openbsd.*", "openbsd"},
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
		return errors.New("triple should be made up of 2, 3, or 4 parts.")
	case 2, 3: // ARCHITECTURE-(VENDOR-)OPERATING_SYSTEM
		GOARCH = s[0]
		GOOS = s[l-1]
	case 4: // ARCHITECTURE-VENDOR-OPERATING_SYSTEM-ENVIRONMENT
		GOARCH = s[0]
		GOOS = s[2]
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

