package runtime

// #llgo name: sync/atomic.CompareAndSwapUint32
func cas(addr *uint32, old, new uint32) (swapped bool)

// #llgo name: sync/atomic.LoadUint32
func atomicload(addr *uint32) uint32

func xchg(addr *uint32, new uint32) uint32 {
	// TODO provide arch-specific implementations where possible.
	for {
		old := atomicload(addr)
		if cas(addr, old, new) {
			return old
		}
	}
}
