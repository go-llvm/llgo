package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// A simple filter program which improves the readability of libgo's build
// output by filtering out everything except the package names.
func main() {
	stdin := bufio.NewReader(os.Stdin)
	pkgs := make(map[string]struct{})
	for {
		line, err := stdin.ReadString('\n')
		if err == io.EOF {
			return
		} else if err != nil {
			panic("read error: " + err.Error())
		}
		line = line[0 : len(line)-1]
		if strings.HasPrefix(line, "libtool: compile:") {
			words := strings.Split(line, " ")
			for _, word := range words {
				if strings.HasPrefix(word, "-fgo-pkgpath=") {
					pkg := word[13:]
					if _, ok := pkgs[pkg]; !ok {
						os.Stdout.WriteString(pkg + "\n")
						pkgs[pkg] = struct{}{}
					}
				}
			}
		}
	}
}
