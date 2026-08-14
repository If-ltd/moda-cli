package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"

	"github.com/if-ltd/moda-cli/internal/cli"
	"github.com/if-ltd/moda-cli/internal/clientconn"
	"github.com/if-ltd/moda-cli/internal/clientmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const usage = `moda-cli exposes Moda capabilities to agents through MCP.

Usage:
  moda-cli --help
  moda-cli mcp --mode client [--connection-file FILE]

Modes:
  client  Bridge stdio MCP to the running signed-in Moda desktop client.
  direct  Reserved for the future standalone authenticated MCP client.

Environment:
  MODA_CLIENT_MCP_CONNECTION_FILE  Override the Electron connection descriptor.
`

func main() {
	if len(os.Args) == 1 || (len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h")) {
		fmt.Print(usage)
		return
	}

	command, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "moda-cli: %v\n", err)
		os.Exit(2)
	}
	if command.Mode == "direct" {
		fmt.Fprintln(os.Stderr, "moda-cli: direct mode is not implemented yet")
		os.Exit(2)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "moda-cli: resolve home directory: %v\n", err)
		os.Exit(1)
	}
	connectionFile, err := clientconn.ResolveFilePath(command.ConnectionFile, map[string]string{
		"MODA_CLIENT_MCP_CONNECTION_FILE": os.Getenv("MODA_CLIENT_MCP_CONNECTION_FILE"),
		"APPDATA":                         os.Getenv("APPDATA"),
		"XDG_CONFIG_HOME":                 os.Getenv("XDG_CONFIG_HOME"),
	}, homeDirectory, runtime.GOOS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "moda-cli: %v\n", err)
		os.Exit(1)
	}
	descriptor, err := clientconn.Read(connectionFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "moda-cli: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := clientmcp.RunBridge(ctx, descriptor, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "moda-cli: %v\n", err)
		os.Exit(1)
	}
}
