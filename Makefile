j = 1
prefix = /usr/local
bootstrap = quick

bootstrap: workdir/.bootstrap-stamp

install: bootstrap
	./install.sh $(prefix)

workdir/.bootstrap-stamp: workdir/.update-stamp bootstrap.sh *.go build/*.go cmd/gllgo/*.go debug/*.go
	./bootstrap.sh $(bootstrap) -j$(j)

workdir/.update-stamp: update_libgo.sh
	./update_libgo.sh

.SUFFIXES:
