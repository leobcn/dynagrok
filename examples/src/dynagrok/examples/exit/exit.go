package main

import (
	"os"
)

func main() {
	println(fac(5))
	x := 0
a:
	print("wizard\n")
	if x > 0 {
		goto c
	}
	goto b
b:
	{
		print("wooze\n")
		x++
		goto a
		goto b
		if x := true; x {
			println("never!")
			goto c
			for r := x; ; {
				println(r)
			}
			goto a
			switch {
			}
			select {}
			var f interface{} = 1
			switch f.(type) {
			}
		}
		println("what up")
	}
c:
	if true {
		os.Exit(5)
	}
}

func fac(x int) int {
	if x <= 1 {
		return 1
	}
	return x * fac(x-1)
}
