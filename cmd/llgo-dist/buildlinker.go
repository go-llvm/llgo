// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"log"
	"os"
	"os/exec"
	"path"
)

const llgolinkpath = "github.com/axw/llgo/cmd/llgo-link"

func buildLinker() error {
	log.Println("Building llgo-link")

	cmd := exec.Command("go", "get", llgolinkpath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}

	pkg, err := build.Import(llgolinkpath, "", build.FindOnly)
	if err != nil {
		return err
	}
	llgolinkbin := path.Join(pkg.BinDir, "llgo-link")
	log.Printf("Built %s", llgolinkbin)

	return nil
}
