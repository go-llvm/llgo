package runtime

func panic_(e interface{}) {
	print("panic(")
	printany(e)
	println(")")
	llvm_trap()
}
