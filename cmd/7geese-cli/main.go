// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"fmt"
	"os"

	"7geese-cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(cli.ExitCode(err))
	}
}
