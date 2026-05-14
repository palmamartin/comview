package main

import (
	"fmt"
	"io"
	"os"

	"github.com/rockorager/comview/tui"
)

func main() {
	input, err := readPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "comview: %v\n", err)
		os.Exit(1)
	}

	if err := tui.Run(input); err != nil {
		fmt.Fprintf(os.Stderr, "comview: %v\n", err)
		os.Exit(1)
	}
}

func readPipe() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(input), nil
}
