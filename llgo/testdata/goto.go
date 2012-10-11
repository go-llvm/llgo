package main

func main() {
	i := 0
start:
	if i < 10 {
		println(i)
		i++
		goto start
	} else {
		goto end
	}
	return
end:
	println("done")
	return
}

