j = 1
prefix = /usr/local
bootstrap = quick
gollvmdir = $(shell go list -f '{{.Dir}}' github.com/go-llvm/llvm)
llvmdir = $(gollvmdir)/workdir/llvm_build

bootstrap: workdir/.bootstrap-stamp

install: bootstrap
	./install.sh $(prefix)

check-libgo: bootstrap
	$(MAKE) -C workdir/gofrontend_build/libgo-stage1 check

check-llgo: bootstrap
	$(llvmdir)/bin/llvm-lit -s test

workdir/.bootstrap-stamp: workdir/.build-libgodeps-stamp bootstrap.sh build/*.go cmd/gllgo/*.go cmd/cc-wrapper/*.go debug/*.go irgen/*.go ssaopt/*.go
	./bootstrap.sh $(bootstrap) -j$(j)

workdir/.build-libgodeps-stamp: workdir/.update-clang-stamp workdir/.update-libgo-stamp bootstrap.sh
	./bootstrap.sh libgodeps -j$(j)

workdir/.update-clang-stamp: update_clang.sh $(gollvmdir)/llvm_dep.go
	./update_clang.sh

workdir/.update-libgo-stamp: update_libgo.sh
	./update_libgo.sh

.SUFFIXES:
