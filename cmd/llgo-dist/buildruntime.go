// Copyright 2012-2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
)

func buildRuntime() error {
	log.Println("Building runtime")

	// TODO just use "go list std"
	runtimePackages := [...]string{
		"runtime",
		"syscall",
		"sync/atomic",
		"math",
		"sync",
		"time",
		"os",
		"io",
		"fmt",
		"strconv",
		"errors",
		"reflect",
		"unicode/utf8",
	}
	for _, pkg := range runtimePackages {
		log.Printf("- %s", pkg)
		output, err := command(llgobuildbin, pkg).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			return err
		}
	}

	return nil
}
