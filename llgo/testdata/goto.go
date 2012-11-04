package main

func f1() {
	goto labeled
labeled:
	goto done
	return
done:
	println("!")
}

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
	f1()
	return
}
