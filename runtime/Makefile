CC=clang
CFLAGS=-emit-llvm

all:	libllgoruntime.a

libllgoruntime.a:	goroutine.o
					llvm-ld -link-as-library $< -o $@
