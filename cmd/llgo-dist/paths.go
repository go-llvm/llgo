// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

var gobin = os.Getenv("GOBIN")

// userpath takes a path and resolves a leading "~" pattern to
// the current or specified user's home directory. Note that the
// path will not be converted to an absolute path if there is no
// leading "~" pattern.
func userpath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		slashindex := strings.IndexRune(path, filepath.Separator)
		var username string
		if slashindex == -1 {
			username = path[1:]
		} else {
			username = path[1:slashindex]
		}

		var u *user.User
		var err error
		if username == "" {
			u, err = user.Current()
		} else {
			u, err = user.Lookup(username)
		}
		if err != nil {
			return "", err
		}

		if slashindex == -1 {
			path = u.HomeDir
		} else {
			path = filepath.Join(u.HomeDir, path[slashindex+1:])
		}
	}
	return path, nil
}

// findCommand returns the path to the binary from
// the specified package path.
func findCommand(pkgpath string) (string, error) {
	_, cmdname := path.Split(pkgpath)
	if gobin != "" {
		return filepath.Join(gobin, cmdname), nil
	}
	pkg, err := build.Import(pkgpath, "", build.FindOnly)
	if err != nil {
		return "", err
	}
	return filepath.Join(pkg.BinDir, cmdname), nil
}
