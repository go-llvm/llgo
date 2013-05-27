// Copyright 2013 The llgo Authors.
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
	"strings"
)

const llgoBuildPath = "github.com/axw/llgo/cmd/llgo-build"

var llgobuildbin string

func buildLlgoTools() error {
	log.Println("Building llgo-build")

	// We set default values in the llgo-build binary.
	ldflags := []string{
		fmt.Sprintf("-X main.llgobin %q", llgobin),
		fmt.Sprintf("-X main.llvmbindir %q", llvmbindir),
		fmt.Sprintf("-X main.defaulttriple %q", triple),
	}
	args := []string{"install", "-ldflags", strings.Join(ldflags, " "), llgoBuildPath}
	output, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}

	pkg, err := build.Import(llgoBuildPath, "", build.FindOnly)
	if err != nil {
		return err
	}
	llgobuildbin = path.Join(pkg.BinDir, "llgo-build")
	log.Printf("Built %s", llgobuildbin)
	return nil
}
