// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func buildRuntime() (reterr error) {
	log.Println("Building runtime")

	// TODO(axw) generate platform-specific parts of runtime,
	// e.g. Go-equivalent jmp_buf structs.

	badPackages := []string{
		"net",         // Issue #71
		"os/user",     // Issue #72
		"runtime/cgo", // Issue #73
	}

	output, err := command("go", "list", "std").CombinedOutput()
	if err != nil {
		return err
	}
	runtimePackages := strings.Split(strings.TrimSpace(string(output)), "\n")
outer:
	for _, pkg := range runtimePackages {
		// cmd's aren't packages
		if strings.HasPrefix(pkg, "cmd/") {
			continue
		}
		for _, bad := range badPackages {
			if pkg == bad {
				continue outer
			}
		}
		log.Printf("- %s", pkg)
		output, err := command(llgobuildbin, pkg).CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			reterr = err
		}
	}

	return
}
