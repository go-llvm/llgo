// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"time"
)

type ppbCore1_0 struct {
	addRefResource   func(res Resource)
	releaseResource  func(res Resource)
	getTime          func() ppTime
	getTimeTicks     func() ppTimeTicks
	callOnMainThread func(delayMillis int32, callback ppCompletionCallback, result int32)
	isMainThread     func() ppBool
}

func (p *ppbCore1_0) AddRefResource(r Resource) {
	p.addRefResource(r)
}

func (p *ppbCore1_0) ReleaseResource(r Resource) {
	p.ReleaseResource(r)
}

func (p *ppbCore1_0) GetTime() time.Time {
	seconds := p.getTime()
	return seconds.asTime()
}

func (p *ppbCore1_0) IsMainThread() bool {
	return p.isMainThread().toBool()
}

type Core interface {
	AddRefResource(Resource)
	ReleaseResource(Resource)
	GetTime() time.Time
	//GetTimeTicks()
	//CallOnMainThread(after time.Duration, ...)
	IsMainThread() bool
}
