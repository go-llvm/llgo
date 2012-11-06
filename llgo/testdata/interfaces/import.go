package main

import "syscall"

type Signal interface {
	Signal()
}

func main() {
	var s Signal = syscall.SIGINT
	println(s)
}
