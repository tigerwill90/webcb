package command

import (
	"fmt"
	"github.com/jwalton/gchalk"
	"io"
	"time"
)

type ui struct {
	stdout io.Writer
	stderr io.Writer
}

func newUi(stdout, stderr io.Writer) *ui {
	return &ui{
		stdout: stdout,
		stderr: stderr,
	}
}

func (ui *ui) Errorf(format string, a ...interface{}) {
	if _, err := fmt.Fprint(ui.stderr, gchalk.WithHex("#c62828").Sprintf(format, a...)); err != nil {
		panic(err)
	}
}

func (ui *ui) Infof(format string, a ...interface{}) {
	if _, err := fmt.Fprintf(ui.stdout, format, a...); err != nil {
		panic(err)
	}
}

func (ui *ui) Warnf(format string, a ...interface{}) {
	if _, err := fmt.Fprint(ui.stdout, gchalk.WithHex("#fdd835").Sprintf(format, a...)); err != nil {
		panic(err)
	}
}

func (ui *ui) Successf(format string, a ...interface{}) {
	if _, err := fmt.Fprint(ui.stdout, gchalk.WithHex("#43a047").Sprintf(format, a...)); err != nil {
		panic(err)
	}
}

func formatDuration(d time.Duration) string {
	scale := 100 * time.Second
	// look for the max scale that is smaller than d
	for scale > d {
		scale = scale / 10
	}
	return fmt.Sprintf("%6s", d.Round(scale/100).String())
}
