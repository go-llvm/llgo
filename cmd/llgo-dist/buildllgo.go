// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	gobuild "go/build"
	"log"
	"os"
	"strings"

	"github.com/go-llvm/llgo/build"
)

const llvmpkgpath = "github.com/go-llvm/llvm"
const llgopkgpath = "github.com/go-llvm/llgo/llgo"

var (
	// llgobin is the path to the llgo command.
	llgobin string
)

func buildLlgo() error {
	log.Println("Building llgo")

	cmd := command("go", "get", "-d", llvmpkgpath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}
	pkg, err := gobuild.Import(llvmpkgpath, "", gobuild.FindOnly)
	if err != nil {
		return err
	}
	if alwaysbuild {
		if _, err := os.Stat(pkg.PkgObj); err == nil {
			log.Println("- Rebuilding go-llvm/llvm")
			os.Remove(pkg.PkgObj)
		}
	}

	cgoCflags := fmt.Sprintf("%s -I %s/../include", llvmcflags, pkg.Dir)
	cgoLdflags := fmt.Sprintf("-Wl,-L%s", llvmlibdir)
	ldflags := fmt.Sprintf("-r %q", llvmlibdir)

	if sharedllvm {
		cgoLdflags += fmt.Sprintf(" -lLLVM-%s ", llvmversion)
	} else {
		cgoLdflags += " " + llvmlibs + " -lstdc++ -lm "
	}
	cgoLdflags += " " + llvmldflags

	args := []string{"get", "-ldflags", ldflags}
	llvmtag := "llvm" + llvmversion
	if strings.HasSuffix(llvmversion, "svn") {
		llvmtag = "llvmsvn"
	}
	args = append(args, []string{"-tags", llvmtag}...)
	args = append(args, llgopkgpath)

	cmd = command("go", args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CGO_CFLAGS="+cgoCflags)
	cmd.Env = append(cmd.Env, "CGO_LDFLAGS="+cgoLdflags)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", string(output))
		return err
	}

	if llgobin, err = findCommand(llgopkgpath); err != nil {
		return err
	}

	// If the user did not specify -triple on the command
	// line, ask llgo for it now.
	if triple == "" {
		output, err = command(llgobin, "-print-triple").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			return err
		}
		triple = strings.TrimSpace(string(output))
	}
	llgobuildctx, err := build.ContextFromTriple(triple)
	if err != nil {
		return err
	}
	buildctx = &llgobuildctx.Context
	log.Printf("GOARCH = %s, GOOS = %s", buildctx.GOARCH, buildctx.GOOS)
	log.Printf("Built %s", llgobin)
	if install_name_tool && sharedllvm {
		// TODO: this was with the LLVM shipped with the pnacl sdk and might not be true of *all* libLLVM-xxx.dylibs.
		//       Is there a link time commandline option that has the same effect?
		cmd = command("install_name_tool", "-change", fmt.Sprintf("@executable_path/../lib/libLLVM-%s.dylib", llvmversion), fmt.Sprintf("%s/libLLVM-%s.dylib", llvmlibdir, llvmversion), llgobin)
		output, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", string(output))
			return err
		}
		log.Printf("Successfully changed shared libLLVM path")
	}
	return nil
}
