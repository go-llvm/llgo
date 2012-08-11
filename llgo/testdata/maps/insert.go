package main

func main() {
	{
		var m map[int]int
		println(m[123]) // 0, despite map being nil
	}

	{
		m := make(map[int]int)
		m[123] = 456
		println(m[123])
	}
}
