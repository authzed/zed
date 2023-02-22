package console

import (
	"fmt"
	"os"
)

// Printf defines an (overridable) function for printing to the console via stdout.
var Printf = func(format string, a ...any) {
	fmt.Printf(format, a...)
}

// Errorf defines an (overridable) function for printing to the console via stderr.
var Errorf = func(format string, a ...any) {
	_, err := fmt.Fprintf(os.Stderr, format, a...)
	if err != nil {
		panic(err)
	}
}

// Println prints a line with optional values to the console.
func Println(values ...any) {
	for _, value := range values {
		Printf("%v\n", value)
	}
}
