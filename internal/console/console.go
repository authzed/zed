package console

import (
	"fmt"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/schollz/progressbar/v3"
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

func CreateProgressBar(description string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetVisibility(false),
	)
	if isatty.IsTerminal(os.Stderr.Fd()) {
		bar = progressbar.NewOptions64(-1,
			progressbar.OptionSetDescription(description),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("relationship"),
			progressbar.OptionOnCompletion(func() { _, _ = fmt.Fprint(os.Stderr, "\n") }),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
		)
	}

	return bar
}
