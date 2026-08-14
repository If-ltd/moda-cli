package main

import (
	"fmt"
	"os"
)

const usage = `moda-cli exposes Moda capabilities to agents through MCP.

Usage:
  moda-cli --help

Client and direct modes are under active development.
`

func main() {
	if len(os.Args) == 1 || (len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h")) {
		fmt.Print(usage)
		return
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
	os.Exit(2)
}
