// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"os/user"
	"path/filepath"
	"strings"
)

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
