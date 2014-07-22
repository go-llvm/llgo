// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func f1() {
	if true {
		println("f1")
		return
	}
	for {
	}
}

func f2() {
	defer func() { println("f2") }()
	if true {
		return
	}
	for {
	}
}

func f3() int {
	if true {
		println("f3")
		return 123
	}
	for {
	}
}

func f4() int {
	defer func() { println("f4") }()
	if true {
		return 123
	}
	for {
	}
}

func main() {
	f1()
	f2()
	f3()
	println(f4())
}
