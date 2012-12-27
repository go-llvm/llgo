// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type ppbMessaging1_0 struct {
	postMessage func(i PP_Instance, msg Var)
}
