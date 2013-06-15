// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type ppbMessaging1_0 struct {
	postMessage uintptr //func(i PP_Instance, msg Var)
}

// #llgo name: ppapi_callPostMessage
func callPostMessage(m *ppbMessaging1_0, i PP_Instance, msg *Var)
