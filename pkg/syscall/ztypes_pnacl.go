
// +build pnacl

// Created by cgo -godefs - DO NOT EDIT
// cgo -godefs -objdir /var/folders/n1/yjqlqd2s04x7th1152h48hsr0000gn/T/llgo_dist381379107 -- -nostdinc -D__native_client__ -isystem /Users/quarnster/Downloads/nacl_sdk/pepper_29/toolchain/mac_x86_pnacl/newlib/usr/include -isystem /Users/quarnster/Downloads/nacl_sdk/pepper_29/include -isystem /Users/quarnster/Downloads/nacl_sdk/pepper_29/toolchain/mac_x86_pnacl/host_x86_64/lib/clang/3.3/include -isystem /Users/quarnster/Downloads/nacl_sdk/pepper_29/toolchain/mac_x86_pnacl/newlib/sysroot/include /Users/quarnster/code/go/src/github.com/axw/llgo/pkg/syscall/types_pnacl.go

package syscall

const (
	sizeofPtr	= 0x4
	sizeofShort	= 0x2
	sizeofInt	= 0x4
	sizeofLong	= 0x4
	sizeofLongLong	= 0x8
	PathMax		= 0x1000
)

type (
	_C_short	int16
	_C_int		int32
	_C_long		int32
	_C_long_long	int64
)

type Timespec struct {
	Sec	int64
	Nsec	int32
}

type Timeval struct {
	Sec	int64
	Usec	int32
}

type Timex [0]byte

type Time_t int64

type Tms [0]byte

type Utimbuf struct {
	Actime	int64
	Modtime	int64
}

type Rusage struct {
	Utime	Timeval
	Stime	Timeval
}

type Rlimit [0]byte

type _Gid_t uint32

type Stat_t struct {
	Dev	int64
	Ino	uint64
	Mode	uint32
	Nlink	uint32
	Uid	uint32
	Gid	uint32
	Rdev	int64
	Size	int64
	Blksize	int32
	Blocks	int32
	Atime	int64
	Atimensec	int64
	Mtime	int64
	Mtimensec	int64
	Ctime	int64
	Ctimensec	int64
}

type Statfs_t [0]byte

type Dirent struct {
	Ino	uint64
	Off	int64
	Reclen	uint16
	Name	[256]int8
	Pad_cgo_0	[2]byte
}
