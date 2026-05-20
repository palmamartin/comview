package main

import (
	"fmt"
	"os"

	"github.com/rockorager/comview/internal/terminal"
	"github.com/rockorager/comview/tui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "watch" {
		command, err := watchCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "comview: %v\n", err)
			os.Exit(1)
		}
		if err := tui.RunWatch(command); err != nil {
			fmt.Fprintf(os.Stderr, "comview: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

func watchCommand(args []string) ([]string, error) {
	if len(args) == 0 {
		return []string{"git", "diff"}, nil
	}
	if args[0] == "--" {
		if len(args) == 1 {
			return nil, fmt.Errorf("watch command after -- is required")
		}
		return append([]string{}, args[1:]...), nil
	}
	command := []string{"git", "diff"}
	command = append(command, args...)
	return command, nil
}

func readPipe() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	return terminal.PrintableANSIOutput(os.Stdin), nil
}
