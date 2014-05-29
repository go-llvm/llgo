j = 1
prefix = /usr/local
bootstrap = quick

bootstrap: workdir/.bootstrap-stamp

install: bootstrap
	./install.sh $(prefix)

workdir/.bootstrap-stamp: workdir/.update-libgo-stamp workdir/.update-clang-stamp bootstrap.sh *.go build/*.go cmd/gllgo/*.go cmd/cc-wrapper/*.go debug/*.go
	./bootstrap.sh $(bootstrap) -j$(j)

workdir/.update-clang-stamp: update_clang.sh
	./update_clang.sh

workdir/.update-libgo-stamp: workdir/.update-clang-stamp update_libgo.sh
	./update_libgo.sh

.SUFFIXES:
