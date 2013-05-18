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
	"strings"
)

const gollvmpkgpath = "github.com/axw/gollvm/llvm"
const llgopkgpath = "github.com/axw/llgo/llgo"

var (
	// llgobin is the path to the llgo command.
	llgobin string
)

func buildLlgo() error {
	log.Println("Building llgo")

	cmd := exec.Command("go", "get", "-d", gollvmpkgpath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}
	pkg, err := build.Import(gollvmpkgpath, "", build.FindOnly)
	if err != nil {
		return err
	}

	cgoCflags := fmt.Sprintf("%s -I %s/../include", llvmcflags, pkg.Dir)
	cgoLdflags := fmt.Sprintf("-Wl,-L%s", llvmlibdir)
	ldflags := fmt.Sprintf("-r %q", llvmlibdir)

	if sharedllvm {
		cgoLdflags += fmt.Sprintf(" -lLLVM-%s ", llvmversion)
	} else {
		cgoLdflags += " " + llvmlibs + " -lstdc++ "
	}
	cgoLdflags += " " + llvmldflags

	args := []string{"get", "-ldflags", ldflags}
	llvmtag := "llvm" + llvmversion
	if strings.HasSuffix(llvmversion, "svn") {
		llvmtag = "llvmsvn"
	}
	args = append(args, []string{"-tags", llvmtag}...)
	args = append(args, llgopkgpath)

	cmd = exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CGO_CFLAGS="+cgoCflags)
	cmd.Env = append(cmd.Env, "CGO_LDFLAGS="+cgoLdflags)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}

	pkg, err = build.Import(llgopkgpath, "", build.FindOnly)
	if err != nil {
		return err
	}
	llgobin = path.Join(pkg.BinDir, "llgo")

	// If the user did not specify -triple on the command
	// line, ask llgo for it now.
	if triple == "" {
		output, err = exec.Command(llgobin, "-print-triple").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			return err
		}
		triple = strings.TrimSpace(string(output))
	}
	if err = initGOVARS(triple); err != nil {
		return err
	}
	log.Printf("GOARCH = %s, GOOS = %s", GOARCH, GOOS)
	log.Printf("Built %s", llgobin)
	return nil
}
