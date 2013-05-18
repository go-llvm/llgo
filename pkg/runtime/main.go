// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

// #llgo linkage: appending
var ctors [1]*int8 // nil, used to find end of list

// helper function to call a raw function pointer.
// TODO move all of this code to .ll/.c
func callNiladicFunc(f *int8)

// A Go program will enter this function before doing anything else.
func main(argc int32, argv **byte, envp **byte, mainmain *int8) int32 {
	// Initialise the runtime before calling any constructors.
	setosargs(argc, argv, envp)

	// Constructors are in reverse order (see llgo/compiler.go for
	// an explanation). Since runtime module must always come last,
	// we can use a sentinel nil value to find the end.
	for i := 0; ; i++ {
		if ctors[i] == nil {
			for i > 0 {
				i--
				callNiladicFunc(ctors[i])
			}
			break
		}
	}

	// All done, call "main.main".
	// TODO recover from panics, alter return code accordingly.
	callNiladicFunc(mainmain)

	return 0
}
