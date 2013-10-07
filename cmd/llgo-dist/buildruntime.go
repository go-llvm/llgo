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
		"crypto/x509",         // Issue #70
		"database/sql/driver", // Issue #65
		"net",         // Issue #71
		"net/http",    // Issue #68
		"os/user",     // Issue #72
		"runtime/cgo", // Issue #73
	}
	if triple == "pnacl" {
		badPackages = append(badPackages,
			"crypto/md5",
			"crypto/rc4",
			"hash/crc32",
			"mime",
			"os/exec",
			"os/signal",
			"path/filepath",
		)
	}

	output, err := command("go", "list", "std").CombinedOutput()
	if err != nil {
		return err
	}

	// Always build runtime and syscall first
	// TODO: Real import dependency discovery to build packages in the order they depend on each other
	runtimePackages := append([]string{"runtime", "syscall"}, strings.Split(strings.TrimSpace(string(output)), "\n")...)
outer:
	for _, pkg := range runtimePackages {
		// cmd's aren't packages
		if strings.HasPrefix(pkg, "cmd/") {
			continue
		}
		// drone.io keeps various appengine packages in std,
		// which fail due to required third-party dependencies.
		// this is a kludge. FIXME
		if strings.HasPrefix(pkg, "appengine") {
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
