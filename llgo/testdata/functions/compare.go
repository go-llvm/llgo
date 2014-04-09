package main

func main() {
	var f func()
	println(f == nil)
	println(f != nil)
	println(nil == f)
	println(nil != f)
	f = func() {}
	println(f == nil)
	println(f != nil)
	println(nil == f)
	println(nil != f)
}
