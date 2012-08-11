package main

func main() {
	{
		var m map[int]int
		println(len(m)) // 0
		println(m[123]) // 0, despite map being nil
	}

	{
		m := make(map[int]int)
		m[123] = 456
		println(len(m)) // 1
		println(m[123])
		m[123] = 789
		println(len(m)) // 1
		println(m[123])
	}
}
