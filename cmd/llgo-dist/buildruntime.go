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

	badPackages := []string{
		"compress/flate",      // Issue #61
		"crypto/tls",          // Issue #63
		"crypto/x509",         // Issue #70
		"database/sql",        // Issue #64
		"database/sql/driver", // Issue #65
		"encoding/json",       // Issue #66
		"go/parser",           // Issue #67
		"hash",                // Issue #56
		"net",                 // Issue #71
		"net/http",            // Issue #68
		"net/rpc",             // Issue #69 cap(chan) not implemented
		"os/exec",             // Issue #62
		"os/user",             // Issue #72
		"runtime/cgo",         // Issue #73
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
