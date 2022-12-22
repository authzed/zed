package console

import (
	"fmt"
)

// Printf defines an (overridable) function for printing to the console.
var Printf = func(format string, a ...any) {
	fmt.Printf(format, a...)
}

// Println prints a line with optional values to the console.
func Println(values ...any) {
	for _, value := range values {
		Printf("%v\n", value)
	}
}
