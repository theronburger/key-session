package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/theronburger/key-session/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "key-session: %v\n", err)
		var exitError cli.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.Code)
		}
		os.Exit(1)
	}
}
