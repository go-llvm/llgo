// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build pnacl

package syscall

import (
	"runtime"
	"sync"
	"unsafe"
)

var (
	Stdin  = 0
	Stdout = 1
	Stderr = 2
)

func Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, EPERM
}

func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, EPERM
}

func RawSyscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, EPERM
}

func RawSyscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno) {
	return 0, 0, EPERM
}

func Getegid() (egid int) {
	return 0
}

func Getgid() (gid int) {
	return 0
}

func Geteuid() (euid int) {
	return 0
}

func Getuid() (uid int) {
	return 0
}

func Getpid() (egid int) {
	return 1
}

func Getppid() (egid int) {
	return 0
}

func Getpagesize() int {
	return 4096
}

// Mmap manager, for use by operating system-specific implementations.

/*
type mmapper struct {
	sync.Mutex
	active map[*byte][]byte // active mappings; key is last byte in mapping
	mmap   func(addr, length uintptr, prot, flags, fd int, offset int64) (uintptr, error)
	munmap func(addr uintptr, length uintptr) error
}

func (m *mmapper) Mmap(fd int, offset int64, length int, prot int, flags int) (data []byte, err error) {
	if length <= 0 {
		return nil, EINVAL
	}

	// Map the requested memory.
	addr, errno := m.mmap(0, uintptr(length), prot, flags, fd, offset)
	if errno != nil {
		return nil, errno
	}

	// Slice memory layout
	var sl = struct {
		addr uintptr
		len  int
		cap  int
	}{addr, length, length}

	// Use unsafe to turn sl into a []byte.
	b := *(*[]byte)(unsafe.Pointer(&sl))

	// Register mapping in m and return it.
	p := &b[cap(b)-1]
	m.Lock()
	defer m.Unlock()
	m.active[p] = b
	return b, nil
}

func (m *mmapper) Munmap(data []byte) (err error) {
	if len(data) == 0 || len(data) != cap(data) {
		return EINVAL
	}

	// Find the base of the mapping.
	p := &data[cap(data)-1]
	m.Lock()
	defer m.Unlock()
	b := m.active[p]
	if b == nil || &b[0] != &data[0] {
		return EINVAL
	}

	// Unmap the memory and update m.
	if errno := m.munmap(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))); errno != nil {
		return errno
	}
	delete(m.active, p)
	return nil
}
*/

func TimespecToNsec(ts Timespec) int64 { return int64(ts.Sec)*1e9 + int64(ts.Nsec) }

func NsecToTimespec(nsec int64) (ts Timespec) {
	ts.Sec = nsec / 1e9
	ts.Nsec = int32(nsec % 1e9)
	return
}

func TimevalToNsec(tv Timeval) int64 { return tv.Sec*1e9 + int64(tv.Usec)*1e3 }

func NsecToTimeval(nsec int64) (tv Timeval) {
	nsec += 999 // round up to microsecond
	tv.Sec = nsec / 1e9
	tv.Usec = int32(nsec % 1e9 / 1e3)
	return
}

// An Errno is an unsigned number describing an error condition.
// It implements the error interface.  The zero Errno is by convention
// a non-error, so code to convert from Errno to error should use:
//	err = nil
//	if errno != 0 {
//		err = errno
//	}
type Errno uintptr

func (e Errno) Error() string {
	if 0 <= int(e) && int(e) < len(errors) {
		s := errors[e]
		if s != "" {
			return s
		}
	}
	return "errno " + itoa(int(e))
}

func (e Errno) Temporary() bool {
	return e == EINTR || e == EMFILE || e.Timeout()
}

func (e Errno) Timeout() bool {
	return e == EAGAIN || e == EWOULDBLOCK || e == ETIMEDOUT
}

// A Signal is a number describing a process signal.
// It implements the os.Signal interface.
type Signal int

func (s Signal) Signal() {}

func (s Signal) String() string {
	if 0 <= s && int(s) < len(signals) {
		str := signals[s]
		if str != "" {
			return str
		}
	}
	return "signal " + itoa(int(s))
}

func Exit(status int) {
	_exit(status)
}

func Fstat(path string, buf *Stat_t) (err error) {
	return EPERM
}

func Mkdir(path string, mode uint32) (err error) {
	return EPERM
}

func Rmdir(path string) (err error) {
	return EPERM
}

func Chdir(path string) (err error) {
	return EPERM
}

func Fchdir(fd int) (err error) {
	return EPERM
}

func Lstat(path string, buf *Stat_t) (err error) {
	return EPERM
}

func Chmod(path string, mode uint32) (err error) {
	return EPERM
}

func Fchmod(fd int, mode uint32) (err error) {
	return EPERM
}

func Chown(path string, uid int, gid int) (err error) {
	return EPERM
}

func Fchown(fd int, uid int, gid int) (err error) {
	return EPERM
}

func Lchown(path string, uid int, gid int) (err error) {
	return EPERM
}

func Unlink(path string) (err error) {
	return EPERM
}

func Link(oldpath, newpath string) (err error) {
	return EPERM
}

func Symlink(oldpath, newpath string) (err error) {
	return EPERM
}

func Rename(oldpath, newpath string) (err error) {
	return EPERM
}

func Getwd() (wd string, err error) {
	return "", EPERM
}

func Pwrite(fd int, p []byte, offset int64) (n int, err error) {
	return -1, EPERM
}

func Pread(fd int, p []byte, offset int64) (n int, err error) {
	return -1, EPERM
}

func Fsync(fd int) (err error) {
	return EPERM
}

func ReadDirent(fd int, buf []byte) (n int, err error) {
	return -1, EPERM
}

func ParseDirent(buf []byte, max int, names []string) (consumed int, count int, newnames []string) {
	return 0, 0, nil
}

func Readlink(path string, buf []byte) (n int, err error) {
	return -1, EPERM
}

func Truncate(path string, length int64) (err error) {
	return EPERM
}

func Ftruncate(fd int, length int64) (err error) {
	return EPERM
}

func Getgroups() (gids []int, err error) {
	return nil, EPERM
}

func CloseOnExec(fd int) {
	panic("unimplemented")
}

func UtimesNano(path string, ts []Timespec) error {
    // TODO
    return EPERM
}

var ioSync int64

func Read(fd int, p []byte) (n int, err error) {
	if len(p) > 0 {
		n = read(fd, &p[0], uint(len(p)))
		if n == -1 {
			err = errno()
		}
		if raceenabled && err == nil {
			raceAcquire(unsafe.Pointer(&ioSync))
		}
	}
	return
}

func Write(fd int, p []byte) (n int, err error) {
	if raceenabled {
		raceReleaseMerge(unsafe.Pointer(&ioSync))
	}
	if len(p) == 0 {
		n = write(fd, nil, 0)
	} else {
		n = write(fd, &p[0], uint(len(p)))
	}
	if n == -1 {
		err = errno()
	}
	return
}

func Close(fd int) (err error) {
	if _close(fd) == -1 {
		err = errno()
	}
	return
}

func Open(path string, mode int, perm uint32) (fd int, err error) {
	var p0 *byte
	p0, err = BytePtrFromString(path)
	if err != nil {
		return
	}
	fd = open(p0, mode, perm)
	if fd == -1 {
		err = errno()
	}
	return
}

// Implemented in syscall_pnacl.c, as we can't represent the
// C signature here due to its varargs parameter.
// #llgo name: syscall_open
func open(path *byte, mode int, perm uint32) int

func Seek(fd int, offset int64, whence int) (off int64, err error) {
	// TODO check offset range
	off = int64(lseek(fd, int(offset), whence))
	if off == -1 {
		err = errno()
	}
	return
}

func Stat(path string, buf *Stat_t) (err error) {
	var p0 *byte
	p0, err = BytePtrFromString(path)
	if err != nil {
		return
	}
	if stat(p0, buf) == -1 {
		err = errno()
	}
	return
}

// #llgo name: __errno
func __errno() *int32

func errno() (err Errno) {
	if e := *__errno(); e != 0 {
		err = Errno(uintptr(err))
	}
	return err
}

// The functions below map to the declarations in nacl_syscalls.h.

// #llgo name: read
func read(fd int, p *byte, n uint) int

// #llgo name: write
func write(fd int, p *byte, n uint) int

// #llgo name: close
func _close(fd int) int

// #llgo name: _exit
// #llgo attr: noreturn
func _exit(status int)

// #llgo name: lseek
func lseek(desc int, offset int, whence int) int

// #llgo name: stat
func stat(path *byte, buf *Stat_t) int
