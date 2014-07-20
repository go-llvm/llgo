j = 1
prefix = /usr/local
bootstrap = quick
llvmdir = $(shell go list -f '{{.Dir}}' github.com/go-llvm/llvm)/workdir/llvm_build

bootstrap: workdir/.bootstrap-stamp

install: bootstrap
	./install.sh $(prefix)

check-libgo: bootstrap
	$(MAKE) -C workdir/gofrontend_build/libgo-stage1 check

check-llgo: bootstrap
	$(llvmdir)/bin/llvm-lit -s test

workdir/.bootstrap-stamp: workdir/.update-libgo-stamp workdir/.update-clang-stamp bootstrap.sh *.go build/*.go cmd/gllgo/*.go cmd/cc-wrapper/*.go debug/*.go ssaopt/*.go
	./bootstrap.sh $(bootstrap) -j$(j)

workdir/.update-clang-stamp: update_clang.sh
	./update_clang.sh

workdir/.update-libgo-stamp: workdir/.update-clang-stamp update_libgo.sh
	./update_libgo.sh

.SUFFIXES:
