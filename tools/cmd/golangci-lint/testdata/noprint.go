package pkg

import "fmt"

func noprint_print() {
	print() // want `Use a logger instead of printing`
}

func noprint_println() {
	println() // want `Use a logger instead of printing`
}

func noprint_fmt_Print() {
	fmt.Print() // want `Use a logger instead of printing`
}

func noprint_fmt_Printf() {
	fmt.Printf("") // want `Use a logger instead of printing`
}

func noprint_fmt_Println() {
	fmt.Println() // want `Use a logger instead of printing`
}
