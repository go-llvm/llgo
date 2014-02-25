// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	llgobuild "github.com/axw/llgo/build"
)

const llgoPkgPrefix = "github.com/axw/llgo/pkg/"

type renamedFileInfo struct {
	os.FileInfo
	name string
}

func (r *renamedFileInfo) Name() string {
	return r.name
}

func runCmd(cmd *exec.Cmd) error {
	if printcommands {
		if cmd.Dir != "" {
			log.Printf("[cwd=%s]", cmd.Dir)
		}
		s := fmt.Sprint(cmd.Args)
		log.Println(s[1 : len(s)-1])
	}
	return cmd.Run()
}

func getPackage(pkgpath string) (pkg *build.Package, err error) {
	// These packages are special: they're mostly written from
	// scratch, so we don't both with the overlay.
	if pkgpath == "runtime" || pkgpath == "runtime/cgo" {
		defer func(pkgpath string) { pkg.ImportPath = pkgpath }(pkgpath)
		pkgpath = llgoPkgPrefix + pkgpath
	}

	// Make a copy, as we'll be modifying ReadDir/OpenFile.
	buildctx := *buildctx

	// Attempt to find an overlay package path,
	// which we'll use in ReadDir below.
	overlayentries := make(map[string]bool)
	overlaypkgpath := llgoPkgPrefix + pkgpath
	overlaypkg, err := buildctx.Import(overlaypkgpath, "", build.FindOnly)
	if err != nil {
		overlaypkg = nil
	}

	// ReadDir is overridden to return a fake ".s"
	// file for each ".ll" file in the directory.
	buildctx.ReadDir = func(dir string) (fi []os.FileInfo, err error) {
		fi, err = ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		entries := make(map[string]os.FileInfo)
		for _, info := range fi {
			entries[info.Name()] = info
		}
		// Overlay all files in the overlay package dir.
		// If we find any .ll files, replace the suffix
		// with .s.
		if overlaypkg != nil {
			fi, err = ioutil.ReadDir(overlaypkg.Dir)
		}
		if err == nil {
			// Check for .ll files in the overlay dir if
			// we have one, else in the standard package dir.
			for _, info := range fi {
				name := info.Name()
				if strings.HasSuffix(name, ".ll") {
					name = name[:len(name)-3] + ".s"
					info = &renamedFileInfo{info, name}
				}
				overlayentries[name] = true
				entries[name] = info
			}
		}
		fi = make([]os.FileInfo, 0, len(entries))
		for _, info := range entries {
			fi = append(fi, info)
		}
		return fi, nil
	}

	// OpenFile is overridden to return the contents
	// of the ".ll" file found in ReadDir above. The
	// returned ReadCloser is wrapped to transform
	// LLVM IR comments to use "//", as expected by
	// go/build when looking for build tags.
	buildctx.OpenFile = func(path string) (io.ReadCloser, error) {
		base := filepath.Base(path)
		overlay := overlayentries[base]
		if overlay {
			if overlaypkg != nil {
				path = filepath.Join(overlaypkg.Dir, base)
			}
			if strings.HasSuffix(path, ".s") {
				path := path[:len(path)-2] + ".ll"
				var r io.ReadCloser
				var err error
				r, err = os.Open(path)
				if err == nil {
					r = llgobuild.NewLLVMIRReader(r)
				}
				return r, err
			}
		}
		return os.Open(path)
	}

	pkg, err = buildctx.Import(pkgpath, "", 0)
	if err != nil {
		return nil, err
	} else {
		if overlaypkg == nil {
			overlaypkg = pkg
		}
		// TODO(axw) factor out the repeated code
		for i, filename := range pkg.GoFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.GoFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.CgoFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.CgoFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.TestGoFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.TestGoFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.XTestGoFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.XTestGoFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.CFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.CFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.SFiles {
			pkgdir := pkg.Dir
			if strings.HasSuffix(filename, ".S") {
				// .S files go straight to clang
			} else if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
				filename = filename[:len(filename)-2] + ".ll"
			} else {
				err := fmt.Errorf("No matching .ll file for %q", filename)
				return nil, err
			}
			pkg.SFiles[i] = path.Join(pkgdir, filename)
		}
	}
	return pkg, nil
}

func buildPackages(pkgpaths []string) error {
	if len(pkgpaths) > 0 && strings.HasSuffix(pkgpaths[0], ".go") {
		pkg, err := goFilesPackage(pkgpaths)
		if err != nil {
			return err
		}
		output := output
		if output == "" && pkg.IsCommand() {
			first := pkg.GoFiles[0]
			output = first[:len(first)-len(".go")]
		}
		for i, filename := range pkg.GoFiles {
			pkg.GoFiles[i] = filepath.Join(pkg.Dir, filename)
		}
		log.Printf("building %s\n", pkg.Name)
		return buildPackage(pkg, output)
	}

	for _, pkgpath := range pkgpaths {
		log.Printf("building %s\n", pkgpath)
		pkg, err := getPackage(pkgpath)
		if err != nil {
			return err
		}
		err = buildPackage(pkg, output)
		if err != nil {
			return err
		}
	}
	return nil
}

var cgoRe = regexp.MustCompile(`[/\\:]`)

func runCgo(pkgpath string, cgofiles, cppflags, cflags []string) (gofiles, cfiles []string, err error) {
	args := []string{
		"tool", "cgo",
		"-gccgo",
		"-gccgopkgpath", pkgpath,
		"-gccgoprefix", "woobie",
		"-objdir", workdir,
		"--",
	}
	args = append(args, cppflags...)
	args = append(args, cflags...)
	args = append(args, cgofiles...)
	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = runCmd(cmd)
	if err != nil {
		return nil, nil, err
	}
	// Get the names of the generated Go and C files.
	gofiles = []string{filepath.Join(workdir, "_cgo_gotypes.go")}
	cfiles = []string{filepath.Join(workdir, "_cgo_export.c")}
	for _, fn := range cgofiles {
		f := cgoRe.ReplaceAllString(fn[:len(fn)-2], "_")
		gofiles = append(gofiles, filepath.Join(workdir, f+"cgo1.go"))
		cfiles = append(cfiles, filepath.Join(workdir, f+"cgo2.c"))
	}

	// gccgo uses "//extern" to name external symbols;
	// translate them to "// #llgo name:".
	for _, gofile := range gofiles {
		if err := translateGccgoExterns(gofile); err != nil {
			return nil, nil, fmt.Errorf("failed to translate gccgo extern comments for %q: %v", gofile, err)
		}
	}

	return gofiles, cfiles, nil
}

func buildPackage(pkg *build.Package, output string) error {
	args := []string{"-c", "-triple", triple}
	dir, file := path.Split(pkg.ImportPath)
	if pkg.IsCommand() || test {
		if output == "" {
			output = file
		}
	} else {
		dir = filepath.Join(pkgroot, dir)
		err := os.MkdirAll(dir, os.FileMode(0755))
		if err != nil {
			return err
		}
		if output == "" {
			output = path.Join(dir, file+".bc")
		}
	}
	if !pkg.IsCommand() || test {
		args = append(args, "-importpath", pkg.ImportPath)
	}

	var cgoCFLAGS, cgoCPPFLAGS []string
	if len(pkg.CFiles) > 0 || len(pkg.CgoFiles) > 0 {
		cgoCFLAGS = append(envFields("CGO_CFLAGS"), pkg.CgoCFLAGS...)
		cgoCPPFLAGS = append(envFields("CGO_CPPFLAGS"), pkg.CgoCPPFLAGS...)
		//cgoCXXFLAGS = append(envFields("CGO_CXXFLAGS"), pkg.CgoCXXFLAGS...)
		//cgoLDFLAGS = append(envFields("CGO_LDFLAGS"), pkg.CgoLDFLAGS...)
		// TODO(axw) process pkg-config
		cgoCPPFLAGS = append(cgoCPPFLAGS, "-I", workdir, "-I", pkg.Dir)
	}
	var gofiles, cfiles []string
	if len(pkg.CgoFiles) > 0 {
		var err error
		gofiles, cfiles, err = runCgo(pkg.ImportPath, pkg.CgoFiles, cgoCPPFLAGS, cgoCFLAGS)
		if err != nil {
			return err
		}
	}

	gofiles = append(gofiles, pkg.GoFiles...)
	cfiles = append(cfiles, pkg.CFiles...)

	_, file = path.Split(output)
	tempfile := path.Join(workdir, file+".bc")
	args = append(args, fmt.Sprintf("-g=%v", generateDebug))
	args = append(args, "-o", tempfile)
	args = append(args, gofiles...)
	if test {
		args = append(args, pkg.TestGoFiles...)
	}
	cmd := exec.Command(llgobin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := runCmd(cmd); err != nil {
		return err
	}

	// Remove the .S files and add them to cfiles.
	for i := 0; i < len(pkg.SFiles); i++ {
		sfile := pkg.SFiles[i]
		if strings.HasSuffix(sfile, ".S") {
			cfiles = append(cfiles, sfile)
			pkg.SFiles = append(pkg.SFiles[:i], pkg.SFiles[i+1:]...)
			i--
		}
	}

	// Compile and link .c files in.
	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	for _, cfile := range cfiles {
		bcfile := filepath.Join(workdir, filepath.Base(cfile+".bc"))
		args = []string{"-c", "-o", bcfile}
		if triple != "pnacl" {
			args = append(args, "-target", triple, "-emit-llvm")
		}
		args = append(args, cgoCFLAGS...)
		args = append(args, cgoCPPFLAGS...)
		args = append(args, cfile)
		cmd := exec.Command(clang, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := runCmd(cmd)
		if err != nil {
			os.Remove(bcfile)
			return err
		}
		cmd = exec.Command(llvmlink, "-o", tempfile, tempfile, bcfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = runCmd(cmd)
		os.Remove(bcfile)
		if err != nil {
			return err
		}
	}

	// Link .ll files in.
	if len(pkg.SFiles) > 0 {
		args = []string{"-o", tempfile, tempfile}
		args = append(args, pkg.SFiles...)
		cmd := exec.Command(llvmlink, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := runCmd(cmd); err != nil {
			return err
		}
	}

	// If it's a command, link in the dependencies.
	if pkg.IsCommand() {
		if err := linkdeps(pkg, tempfile); err != nil {
			return err
		}
		if run {
			cmd := exec.Command(tempfile)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return runCmd(cmd)
		}
	} else if test {
		if err := linktest(pkg, tempfile); err != nil {
			return err
		}
	}
	return moveFile(tempfile, output)
}
