include $(GOROOT)/src/Make.inc

TARG=llgo
GOFILES=llgo.go \
        expr.go \
        println.go \
        stmt.go \
        decl.go \
        literals.go \
        len.go \
        const.go \
        types.go \
        methods.go \
        goroutine.go \
        new.go \
        debug.go \
        version.go

include $(GOROOT)/src/Make.cmd

