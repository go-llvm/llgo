// Copyright 2013 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package atomic

import "unsafe"

// Defined in atomic.ll
func LoadUint32(addr *uint32) (val uint32)
func LoadUint64(addr *uint64) (val uint64)
func LoadInt32(addr *uint32) (val uint32)
func LoadInt64(addr *uint64) (val uint64)
func StoreUint32(addr *uint32, val uint32)
func StoreUint64(addr *uint64, val uint64)
func StoreInt32(addr *int32, val int32)
func StoreInt64(addr *int64, val int64)
func AddInt32(val *int32, delta int32) (new int32)
func AddUint32(val *uint32, delta uint32) (new uint32)
func AddInt64(val *int64, delta int64) (new int64)
func AddUint64(val *uint64, delta uint64) (new uint64)
func CompareAndSwapInt32(val *int32, old, new int32) (swapped bool)
func CompareAndSwapInt64(val *int64, old, new int64) (swapped bool)
func CompareAndSwapUint32(val *uint32, old, new uint32) (swapped bool)
func CompareAndSwapUint64(val *uint64, old, new uint64) (swapped bool)

func StoreUintptr(addr *uintptr, val uintptr) {
	if unsafe.Sizeof(*addr) == unsafe.Sizeof(int64(0)) {
		StoreUint64((*uint64)(unsafe.Pointer(addr)), uint64(val))
	} else {
		StoreUint32((*uint32)(unsafe.Pointer(addr)), uint32(val))
	}
}

func StorePointer(addr *unsafe.Pointer, val unsafe.Pointer) {
	StoreUintptr((*uintptr)(unsafe.Pointer(addr)), uintptr(val))
}

func LoadUintptr(addr *uintptr) uintptr {
	if unsafe.Sizeof(*addr) == unsafe.Sizeof(int64(0)) {
		return uintptr(LoadUint64((*uint64)(unsafe.Pointer(addr))))
	}
	return uintptr(LoadUint32((*uint32)(unsafe.Pointer(addr))))
}

func LoadPointer(addr *unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(LoadUintptr((*uintptr)(unsafe.Pointer(addr))))
}

func AddUintptr(val *uintptr, delta uintptr) (new uintptr) {
	if unsafe.Sizeof(*val) == unsafe.Sizeof(int64(0)) {
		return uintptr(AddUint64((*uint64)(unsafe.Pointer(val)), uint64(delta)))
	}
	return uintptr(AddUint32((*uint32)(unsafe.Pointer(val)), uint32(delta)))
}

func CompareAndSwapUintptr(val *uintptr, old, new uintptr) (swapped bool) {
	if unsafe.Sizeof(*val) == unsafe.Sizeof(int64(0)) {
		return CompareAndSwapUint64((*uint64)(unsafe.Pointer(val)), uint64(old), uint64(new))
	}
	return CompareAndSwapUint32((*uint32)(unsafe.Pointer(val)), uint32(old), uint32(new))
}

func CompareAndSwapPointer(val *unsafe.Pointer, old, new unsafe.Pointer) (swapped bool) {
	return CompareAndSwapUintptr((*uintptr)(unsafe.Pointer(val)), uintptr(old), uintptr(new))
}
